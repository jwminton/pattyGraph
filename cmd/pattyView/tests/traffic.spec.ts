import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('explores only emitted interval traffic detail', async ({ page }) => {
  const record = {
    schema_version: 4,
    event_type: 'interval',
    session_id: 'traffic-detail',
    timestamp: '2026-07-18T10:01:00-07:00',
    interval: 0,
    interval_lines: 1450,
    source_examples_enabled: true,
    source_lines: [
      '192.0.2.20 - latest catalog request page=4 mirrored PAGE=4',
      '192.0.2.30 - latest direct referrer request',
      '192.0.2.10 - latest retained IP request',
    ],
    matchers: [
      {
        name: 'lines',
        color_hex: '#4ecdc4',
        interval_count: 1450,
        last_interval_count: 1200,
        history_peak: 1600,
        history_total: 2650,
        top_keys: [{ key: 'marked', count: 435 }, { key: ' b16', count: 725 }],
      },
      { name: 'bytes', color_hex: '#e3ba72', interval_count: 4400000, last_interval_count: 3400000, history_peak: 4400000, history_total: 7800000 },
      {
        name: 'errs',
        color_hex: '#f18a8a',
        interval_count: 18,
        last_interval_count: 14,
        history_peak: 18,
        history_total: 32,
        top_keys: [{ key: '500', count: 11, rank: 1 }, { key: '404', count: 7, rank: 2 }],
      },
      { name: 'Googlebot', color_hex: '#ff00ff', interval_count: 102, last_interval_count: 88, history_peak: 120 },
      { name: 'Bots', color_hex: '#a0ffff', interval_count: 28, last_interval_count: 32, history_peak: 54 },
      { name: 'checkout', color_hex: '#ff6347', interval_count: 40, last_interval_count: 20, history_peak: 60 },
      { name: 'page=1', color_hex: '#7fff00', interval_count: 75, last_interval_count: 55, history_peak: 90 },
      { name: 'bingbot', color_hex: '#ffff00', interval_count: 12, last_interval_count: 9, history_peak: 14 },
    ],
    interesting: [
      {
        name: 'words',
        total_keys: 2,
        top: [
          { key: 'catalog', rank: 1, score: 80, count: 20, bytes: 4096, prime_flux: 60, history_depth: 4, last_status: '200', marked_state: 'unmarked', source_line_ref: 1 },
          { key: 'page=4', rank: 2, score: 30, count: 8, bytes: 1024, source_line_ref: 1 },
        ],
        peaks: [{ key: 'catalog', rank: 1, score: 80, count: 20, is_peak: true, source_line_ref: 1 }],
      },
      {
        name: 'refs',
        total_keys: 2,
        top: [{ key: 'direct', rank: 1, score: 42, count: 12, last_status: '200', marked_state: 'unmarked', source_line_ref: 2 }],
        peaks: [{ key: 'campaign', rank: 1, score: 31, count: 8, is_peak: true }],
      },
      {
        name: 'ips',
        total_keys: 1,
        top: [{ key: '192.0.2.10', rank: 1, score: 22, count: 9, bytes: 2048, last_status: '200', marked_state: 'marked', marked_by_matcher: 'Googlebot', source_line_ref: 3 }],
        peaks: [],
        ip_groups: [{ prefix: '192.0.', rank: 1, score: 22, count: 9, count_plus_first: 18, members: 2, bytes: 2048, history_depth: 3, marked_state: 'marked', marked_by_matcher: 'Googlebot' }],
      },
    ],
    factoids: [{ name: 'alerts.active', text: 'Active Alerts:errs', probability: 100 }],
    selected: {
      matcher: 'errs',
      graph_value: 18,
      first_source: { log_line: '192.0.2.10 - first retained request' },
    },
  }

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'traffic.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(JSON.stringify(record)),
  })

  await expect(page.getByRole('tab', { name: 'Compare' })).toBeDisabled()
  await expect(page.locator('.alert-lane-label')).toHaveCount(0)
  const intervalSelector = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  await expect(intervalSelector).not.toHaveAttribute('aria-valuetext', /marked|b16/)
  const overviewLanes = page.getByRole('region', { name: 'Tracked interval lanes' })
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '64px')
  await expect(overviewLanes).toContainText('Marked %')
  await expect(overviewLanes).toContainText('B16 %')
  await page.getByRole('checkbox', { name: 'Show Marked % in interval map' }).check()
  await page.getByRole('checkbox', { name: 'Show B16 % in interval map' }).check()
  await expect(intervalSelector).toHaveAttribute('aria-valuetext', /30.0% marked, 50.0% b16/)
  await page.getByRole('tab', { name: 'Traffic' }).click()
  const bytesLane = page.getByRole('checkbox', { name: 'Remove bytes from interval map' })
  const errorsLane = page.getByRole('checkbox', { name: 'Remove errs from interval map' })
  await expect(bytesLane).toBeChecked()
  await expect(errorsLane).toBeChecked()
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '96px')
  await bytesLane.uncheck()
  await expect(page.locator('.core-lane-bytes')).toHaveCount(0)
  await expect(page.locator('.interval-map-bytes')).toHaveCount(0)
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '80px')
  await expect(intervalSelector).not.toHaveAttribute('aria-valuetext', /served/)
  await expect(intervalSelector).toHaveAttribute('aria-valuetext', /18 errors/)
  await errorsLane.uncheck()
  await expect(page.locator('.core-lane-errors')).toHaveCount(0)
  await expect(page.locator('.interval-map-errors')).toHaveCount(0)
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '64px')
  await expect(intervalSelector).not.toHaveAttribute('aria-valuetext', /served|errors/)
  await expect(page.locator('.core-lane-lines')).toHaveCount(1)
  await page.getByRole('checkbox', { name: 'Add bytes to interval map' }).check()
  await page.getByRole('checkbox', { name: 'Add errs to interval map' }).check()
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '96px')
  const trafficLanes = page.getByRole('region', { name: 'Tracked interval lanes' })
  await expect(trafficLanes).not.toContainText('Marked %')
  await expect(trafficLanes).not.toContainText('B16 %')
  await expect(page.getByRole('region', { name: 'Core interval signals' })).toContainText('1,450')
  await expect(page.getByRole('region', { name: 'Core interval signals' })).toContainText('4.2 MiB')
  await expect(page.locator('.signal-lines .color-swatch')).toHaveCSS('background-color', 'rgb(78, 205, 196)')
  await expect(page.locator('.signal-bytes .color-swatch')).toHaveCSS('background-color', 'rgb(227, 186, 114)')
  await expect(page.locator('.signal-errs .color-swatch')).toHaveCSS('background-color', 'rgb(255, 107, 107)')
  await expect(page.locator('.matcher-table button.selected')).toContainText('errs')
  await expect(page.getByRole('table', { name: 'errs retained keys' })).toContainText('500')
  const catalogSource = page.locator('.source-examples pre').filter({ hasText: 'latest catalog request' })
  await expect(catalogSource).toContainText('192.0.2.20 - latest catalog request page=4 mirrored PAGE=4')
  const search = page.getByRole('searchbox', { name: 'Search PattyLog' })
  await search.fill('page=4')
  await expect(page.getByRole('table', { name: 'words top' }).locator('.search-text-match')).toHaveText('page=4')
  await expect(catalogSource.locator('mark')).toHaveText(['page=4', 'PAGE=4'])
  await page.getByRole('button', { name: 'Clear search' }).click()
  await expect(catalogSource.locator('mark')).toHaveCount(0)
  await expect(page.getByRole('table', { name: 'words top' }).getByRole('row').filter({ hasText: 'catalog' }))
    .toContainText(/catalog.*80.*20.*4\.0 KiB/)
  const pageFourRow = page.getByRole('table', { name: 'words top' }).getByRole('row').filter({ hasText: 'page=4' })
  await pageFourRow.getByRole('cell').filter({ hasText: '30' }).click()
  await expect(pageFourRow).toHaveClass(/selected/)
  await expect(page.locator('.entry-detail > h3')).toHaveText('page=4')

  for (const name of ['Googlebot', 'Bots', 'checkout', 'page=1', 'bingbot']) {
    await page.getByRole('checkbox', { name: `Add ${name} to interval map` }).check()
  }
  await page.getByRole('checkbox', { name: 'Add W catalog to interval map' }).check()
  await expect(page.locator('.entry-detail > h3')).toHaveText('page=4')
  await page.getByRole('tab', { name: 'Peaks' }).click()
  await expect(page.getByRole('table', { name: 'words peaks' })
    .getByRole('checkbox', { name: 'Remove W catalog from interval map' })).toBeChecked()

  await page.getByRole('tab', { name: 'Refs' }).click()
  await page.getByRole('tab', { name: 'Top' }).click()
  await expect(page.getByRole('table', { name: 'refs top' })).toContainText('direct')
  await expect(page.getByText('192.0.2.30 - latest direct referrer request', { exact: true })).toBeVisible()
  await page.getByRole('checkbox', { name: 'Add R direct to interval map' }).check()

  await page.getByRole('tab', { name: 'IPs' }).click()
  await page.getByRole('tab', { name: 'Top' }).click()
  await expect(page.getByRole('table', { name: 'ips top' })).toContainText('192.0.2.10')
  await expect(page.getByText('192.0.2.10 - latest retained IP request', { exact: true })).toBeVisible()
  await page.getByRole('checkbox', { name: 'Add IP 192.0.2.10 to interval map' }).check()
  await expect(page.locator('.tracked-label')).toHaveText([
    'Googlebot', 'Bots', 'checkout', 'page=1', 'bingbot', 'W catalog', 'R direct', 'IP 192.0.2.10',
  ])
  await expect(page.getByRole('region', { name: 'Tracked interval lanes' })).toContainText('8 / 8')
  await expect(page.getByRole('slider', { name: 'Select a PattyLog interval' })).toHaveAttribute(
    'aria-valuetext',
    /W catalog 80, R direct 42, IP 192.0.2.10 22/,
  )
  await expect(page.getByRole('radiogroup', { name: 'Interesting lane metric' })).toHaveCount(0)
  await expect(page.getByRole('table', { name: 'IP groups' })).toContainText('192.0.')
  await expect(page.getByText('192.0.2.10 - first retained request')).toBeVisible()

  await page.getByRole('tab', { name: 'Refs' }).click()
  await page.getByRole('tab', { name: 'Peaks' }).click()
  await expect(page.getByRole('table', { name: 'refs peaks' })).toContainText('campaign')
  await expect(page.getByRole('checkbox', { name: 'Add R campaign to interval map' })).toBeDisabled()
  await page.getByRole('region', { name: 'Tracked interval lanes' })
    .getByRole('checkbox', { name: 'Remove checkout from interval map' }).click()
  await expect(page.getByRole('checkbox', { name: 'Add R campaign to interval map' })).toBeEnabled()

  await expect(page.getByText('Active Alerts:errs')).toBeVisible()
  await page.getByRole('tab', { name: 'Overview' }).click()
  await expect(page.getByRole('region', { name: 'Tracked interval lanes' })).toContainText('Marked %')
  await expect(page.getByRole('region', { name: 'Tracked interval lanes' })).not.toContainText('Googlebot')
  await expect(page.getByRole('checkbox', { name: 'Hide Marked % in interval map' })).toBeEnabled()
  await expect(page.getByRole('checkbox', { name: 'Hide B16 % in interval map' })).toBeEnabled()

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
})

test('handles source enrichment changing between intervals', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const interval = (id: number, enriched: boolean) => ({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'mixed-sources',
    timestamp: `2026-07-18T10:0${id}:00-07:00`,
    log_time: `2026-07-18T10:0${id}:00-07:00`,
    interval: id,
    interval_lines: 100 + id,
    file_path: './access.log',
    machine: 'Frontend_1',
    source_examples_enabled: enriched,
    ...(enriched ? { source_lines: ['192.0.2.40 - retained catalog request'] } : {}),
    matchers: [
      { name: 'lines', interval_count: 100 + id },
      { name: 'bytes', interval_count: 4096 },
      { name: 'errs', interval_count: 0 },
    ],
    interesting: [{
      name: 'words',
      top: [{ key: 'catalog', count: 10, ...(enriched ? { source_line_ref: 1 } : {}) }],
      peaks: [],
    }],
  })

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'mixed-sources.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(`${JSON.stringify(interval(0, true))}\n${JSON.stringify(interval(1, false))}`),
  })

  await page.getByRole('tab', { name: 'Traffic' }).click()
  await expect(page.getByText('Source examples were not recorded for this interval.')).toBeVisible()
  await expect(page.getByText('192.0.2.40 - retained catalog request', { exact: true })).toHaveCount(0)
  const lookup = page.getByRole('region', { name: 'Find likely source lines' })
  const command = "LC_ALL=C grep -nF -- 'catalog' './access.log'"
  await expect(lookup).toContainText(command)
  await expect(lookup).toContainText('Recorded on Frontend_1')
  await expect(lookup).toContainText('2026-07-18T10:01:00-07:00')
  await expect(lookup).toContainText("run from PattyGraph's original working directory")
  await lookup.getByRole('button', { name: 'Copy command' }).click()
  await expect(lookup.getByText('Copied', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(command)

  await page.getByRole('button', { name: 'Older record' }).click()
  await expect(page.getByText('192.0.2.40 - retained catalog request', { exact: true })).toBeVisible()
  await expect(page.getByText('Source examples were not recorded for this interval.')).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Find likely source lines' })).toHaveCount(0)

  await page.getByRole('button', { name: 'Newer record' }).click()
  await expect(page.getByText('Source examples were not recorded for this interval.')).toBeVisible()
  await expect(page.getByText('192.0.2.40 - retained catalog request', { exact: true })).toHaveCount(0)
  await expect(lookup).toContainText(command)
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: () => Promise.reject(new Error('clipboard denied')) },
    })
  })
  await lookup.getByRole('button', { name: 'Copy command' }).click()
  await expect(lookup.getByText('Copy failed; select the command manually.')).toBeVisible()
})

test('shows every alert transition attached to an interval', async ({ page }) => {
  const values = [
    {
      schema_version: 4,
      event_type: 'session_start',
      session_id: 'alert-timeline',
      timestamp: '2026-07-18T10:00:00-07:00',
      version: '0.1.8',
    },
    {
      schema_version: 4,
      event_type: 'interval',
      session_id: 'alert-timeline',
      timestamp: '2026-07-18T10:01:00-07:00',
      interval: 0,
      interval_lines: 100,
      matchers: [
        { name: 'bytes', interval_count: 4096 },
        { name: 'errs', interval_count: 18 },
      ],
      runtime: {},
    },
    {
      schema_version: 4,
      event_type: 'alert',
      session_id: 'alert-timeline',
      timestamp: '2026-07-18T10:01:20-07:00',
      interval: 0,
      status: 'triggered',
      matcher: 'errs',
      direction: 'above',
      threshold: 15,
      value: 18,
      flux_depth: 3,
      streak: 3,
      current_cycle: 20,
    },
    {
      schema_version: 4,
      event_type: 'alert',
      session_id: 'alert-timeline',
      timestamp: '2026-07-18T10:01:50-07:00',
      interval: 0,
      status: 'recovered',
      matcher: 'errs',
      direction: 'above',
      threshold: 15,
      value: 8,
      flux_depth: 3,
      streak: 3,
      current_cycle: 50,
    },
  ]

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'alerts.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(values.map((value) => JSON.stringify(value)).join('\n')),
  })

  const strip = page.getByRole('region', { name: 'Alerts for interval 0' })
  await expect(strip.getByRole('button')).toHaveCount(2)
  await expect(strip).toContainText('TRIGGERED')
  await expect(strip).toContainText('RECOVERED')
  await expect(page.locator('.interval-map-alert-triggered')).toHaveCount(1)
  await expect(page.locator('.interval-map-alert-recovered')).toHaveCount(1)

  await strip.getByRole('button', { name: 'Open errs triggered transition' }).click()
  await expect(page.getByRole('heading', { name: 'errs triggered' })).toBeVisible()
  await expect(strip.getByRole('button', { name: 'Open errs triggered transition' })).toHaveAttribute('aria-current', 'true')
  await expect(page.getByRole('slider', { name: 'Select a PattyLog interval' })).toHaveAttribute('aria-valuenow', '0')

  await page.getByRole('slider', { name: 'Select a PattyLog interval' }).click({ position: { x: 1, y: 8 } })
  await expect(page.getByRole('heading', { name: 'Jul 18, 10:01 AM' })).toBeVisible()
})

test('keeps an absent interesting lane removable without inventing interval values', async ({ page }) => {
  const interval = (number: number, interesting: unknown[]) => ({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'tracked-gap',
    timestamp: `2026-07-18T10:0${number}:00-07:00`,
    interval: number,
    interval_lines: 100 + number,
    matchers: [],
    interesting,
  })
  const records = [
    interval(0, [{
      name: 'words',
      total_keys: 1,
      top: [{ key: 'catalog', rank: 1, score: 18, color: '#4ecdc4' }],
      peaks: [],
    }]),
    interval(1, [{ name: 'words', total_keys: 0, top: [], peaks: [] }]),
  ]

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'tracked-gap.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(records.map((record) => JSON.stringify(record)).join('\n')),
  })
  await page.getByRole('button', { name: /Interval 0/ }).click()
  await page.getByRole('tab', { name: 'Traffic' }).click()
  await page.getByRole('checkbox', { name: 'Add W catalog to interval map' }).check()
  await page.getByRole('button', { name: /Interval 1/ }).click()

  await expect(page.locator('.tracked-label')).toHaveText(['W catalog'])
  await expect(page.getByRole('slider', { name: 'Select a PattyLog interval' })).toHaveAttribute(
    'aria-valuetext',
    /W catalog unavailable/,
  )
  const tracked = page.getByRole('region', { name: 'Tracked interval lanes' })
  await expect(tracked.getByText('W catalog')).toBeVisible()
  await tracked.getByRole('checkbox', { name: 'Remove W catalog from interval map' }).click()
  await expect(page.locator('.tracked-label')).toHaveCount(0)
})
