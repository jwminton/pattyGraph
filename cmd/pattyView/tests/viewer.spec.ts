import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('opens and navigates a PattyLog snapshot', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Open a PattyLog' })).toBeVisible()

  await page.locator('input[type="file"]').setInputFiles(fixture)

  await expect(page.getByText('6 records')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:02 AM' })).toBeVisible()
  await expect(page.locator('.detail-heading p')).toHaveText('line 7 · interval 1')
  const runtimeSection = page.locator('.overview-section').filter({ has: page.getByRole('heading', { name: 'Runtime' }) })
  await expect(runtimeSection).toContainText('Schema version')
  await expect(runtimeSection).toContainText('4')
  await expect(page.getByText('1 malformed line preserved as diagnostics')).toBeVisible()
  await expect(page.getByText('1,450', { exact: true })).toBeVisible()
  await expect(
    page.locator('.metric').filter({ hasText: 'Unmarked' }).getByText('1,020', { exact: true }),
  ).toBeVisible()
  const factoidRibbon = page.getByRole('region', { name: 'Recorded factoids' })
  await expect(factoidRibbon).toContainText('Active Alerts:errs')
  const factoidToggle = factoidRibbon.locator('.factoid-toggle')
  await expect(factoidToggle).toHaveAccessibleName('Expand 1 factoid')
  await factoidToggle.click()
  await expect(factoidToggle).toHaveAttribute('aria-expanded', 'true')
  await expect(factoidToggle).toHaveAccessibleName('Collapse 1 factoid')
  await expect(factoidRibbon).toContainText('alerts.active')
  await factoidRibbon.getByRole('button', { name: 'Collapse 1 factoid' }).click()
  await expect(page.locator('.record-list-item').first()).toContainText('Interval 1')
  await expect(page.locator('.record-list-item').first().locator('strong')).toHaveText('Jul 18, 08:02 AM')
  await expect(page.locator('.record-list-item').first().locator('small')).toHaveText('Interval 1')
  await expect(page.locator('.record-list-item').last()).toContainText('Session start')

  const recordPosition = page.locator('.record-nav-buttons > span')
  await expect(recordPosition).toHaveText('6')
  await expect(recordPosition).not.toContainText('/')
  await expect(page.getByRole('button', { name: 'Newer record' })).toBeDisabled()
  await page.getByRole('button', { name: 'Older record' }).click()
  await expect(page.getByRole('heading', { name: 'future_event' })).toBeVisible()
  await page.getByRole('button', { name: 'Newer record' }).click()
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:02 AM' })).toBeVisible()

  const intervalMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  await expect(intervalMap).toBeVisible()
  const changeThreshold = page.getByRole('slider', { name: 'Change threshold' })
  await expect(changeThreshold).toHaveValue('40')
  await expect(changeThreshold).toHaveAttribute('step', '1')
  await expect(page.locator('.change-lane-label')).toHaveText('CHANGE')
  await expect(page.locator('.change-value')).toContainText(/Change \d+ ·/)
  await expect(page.locator('.change-value')).toHaveAttribute('title', /Lines|Bytes|Avg bytes\/line|Errs|Marked|B16|Peak balance|Word wave/)
  await changeThreshold.fill('0')
  await expect(page.locator('.interval-map-change-qualified')).not.toHaveAttribute('d', '')
  await expect(page.locator('.interval-map-change-context')).toHaveAttribute('d', '')
  await changeThreshold.fill('100')
  await expect(page.locator('.interval-map-change-qualified')).toHaveAttribute('d', '')
  await expect(page.locator('.interval-map-change-context')).not.toHaveAttribute('d', '')
  await changeThreshold.fill('37')
  await expect(changeThreshold).toHaveValue('37')
  await expect(page.locator('.header-change output')).toHaveText('≥ 37')
  const selectedRail = page.locator('.interval-map-selected-rail')
  const selectedCaps = page.locator('.interval-map-selected-cap')
  await expect(selectedRail).toHaveAttribute('x1', '0.5')
  await expect(selectedCaps).toHaveCount(2)
  await expect(selectedCaps.first()).toHaveAttribute('x1', '0.5')
  await expect(selectedCaps.last()).toHaveAttribute('x1', '0.5')
  const railPrecedesSignals = await page.evaluate(() => {
    const rail = document.querySelector('.interval-map-selected-rail')
    const lines = document.querySelector('.interval-map-lines')
    return Boolean(rail && lines && (rail.compareDocumentPosition(lines) & Node.DOCUMENT_POSITION_FOLLOWING))
  })
  expect(railPrecedesSignals).toBe(true)
  await expect(page.locator('.alert-lane-label')).toHaveText('ALERTS')
  await expect(page.locator('.interval-map-alert-triggered')).toHaveCount(1)
  await expect(intervalMap).toHaveAttribute('aria-valuetext', /1,450 lines, 4.2 MiB served, 18 errors/)
  await intervalMap.click({ position: { x: 1, y: 30 } })
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:02 AM' })).toBeVisible()
  await intervalMap.press('End')
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:01 AM' })).toBeVisible()
  await expect(selectedRail).toHaveAttribute('x1', '1.5')
  await expect(selectedCaps.first()).toHaveAttribute('x1', '1.5')
  await expect(selectedCaps.last()).toHaveAttribute('x1', '1.5')
  await expect(intervalMap).toHaveAttribute('aria-valuetext', /1 alert transition: errs triggered/)
  const alertStrip = page.getByRole('region', { name: 'Alerts for interval 0' })
  await expect(alertStrip).toContainText('TRIGGERED')
  await expect(alertStrip).toContainText('08:01:40 AM')
  await alertStrip.getByRole('button', { name: 'Open errs triggered transition' }).click()
  await expect(page.getByRole('heading', { name: 'errs triggered' })).toBeVisible()
  await expect(intervalMap).toHaveAttribute('aria-valuenow', '0')
  await expect(page.locator('.metric').filter({ hasText: 'Flux depth' }).getByText('3', { exact: true })).toBeVisible()
  await expect(page.locator('.metric').filter({ hasText: 'Cycle' }).getByText('40', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: /Interval 0/ }).click()
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:01 AM' })).toBeVisible()
  await expect(
    page.locator('.metric').filter({ hasText: 'Interval lines' }).getByText('1,200', { exact: true }),
  ).toBeVisible()
  await page.getByRole('tab', { name: 'Traffic' }).click()
  await expect(factoidRibbon).toBeVisible()
  await expect(page.locator('.matcher-table button.selected')).toContainText('errs')
  const bytesLane = page.getByRole('checkbox', { name: 'Remove bytes from interval map' })
  const errorsLane = page.getByRole('checkbox', { name: 'Remove errs from interval map' })
  await expect(bytesLane).toBeChecked()
  await expect(errorsLane).toBeChecked()
  await bytesLane.uncheck()
  await errorsLane.uncheck()
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '48px')
  await expect(page.locator('.core-lane-bytes')).toHaveCount(0)
  await expect(page.locator('.core-lane-errors')).toHaveCount(0)
  await expect(page.locator('.interval-map-alert-triggered')).toHaveAttribute('y1', '34')
  await expect(page.locator('.interval-map-alert-triggered')).toHaveAttribute('y2', '46')
  await expect(intervalMap).not.toHaveAttribute('aria-valuetext', /served|errors/)
  await expect(intervalMap).toHaveAttribute('aria-valuetext', /1 alert transition: errs triggered/)
  await page.getByRole('checkbox', { name: 'Add bytes to interval map' }).check()
  await page.getByRole('checkbox', { name: 'Add errs to interval map' }).check()
  await expect(page.locator('.interval-map-graph')).toHaveCSS('height', '80px')
  await expect(page.locator('.interval-map-alert-triggered')).toHaveAttribute('y1', '66')
  await expect(page.locator('.interval-map-alert-triggered')).toHaveAttribute('y2', '78')
  await page.getByRole('checkbox', { name: 'Add Googlebot to interval map' }).check()
  await page.locator('.matcher-table button').filter({ hasText: 'bytes' }).click()
  await page.getByRole('button', { name: /Interval 1/ }).click()
  await expect(page.locator('.matcher-table button.selected')).toContainText('bytes')
  await expect(page.locator('.tracked-label')).toHaveText(['Googlebot'])
  await expect(page.getByRole('region', { name: 'Tracked interval lanes' })
    .getByRole('checkbox', { name: 'Remove Googlebot from interval map' })).toBeChecked()
  await page.getByRole('button', { name: /Interval 0/ }).click()

  await page.getByRole('tab', { name: 'Record' }).click()
  await expect(factoidRibbon).toBeVisible()
  await expect(page.locator('.raw-pane')).toContainText('"interval": 0')

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
})

test('presents Peak reset comparisons as context while retaining raw Change', async ({ page }) => {
  const interval = (number: number, lines: number) => ({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'peak-reset',
    timestamp: `2019-01-22T11:0${number}:00Z`,
    log_time: `2019-01-22T11:0${number}:00Z`,
    interval: number,
    interval_lines: lines,
    matchers: [
      { name: 'lines', interval_count: lines, top_keys: [{ key: 'marked', count: 0 }, { key: ' b16', count: 0 }] },
      { name: 'bytes', interval_count: lines * 1024 },
      { name: 'errs', interval_count: 0 },
    ],
    interesting: [],
  })
  const records = [
    interval(0, 1000),
    {
      schema_version: 4,
      event_type: 'control_command',
      session_id: 'peak-reset',
      timestamp: '2019-01-22T11:00:30Z',
      log_time: '2019-01-22T11:00:30Z',
      source: 'keyboard',
      command: '^p',
      command_name: 'purge',
      status: 'applied',
    },
    interval(1, 2000),
    interval(2, 1000),
    interval(3, 2000),
  ]

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'peak-reset.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(records.map((record) => JSON.stringify(record)).join('\n')),
  })

  await expect(page.locator('.interval-map-change-reset')).toHaveCount(2)
  await expect(page.locator('.interval-map-change-reset.reset')).toHaveCount(1)
  await expect(page.locator('.interval-map-change-reset.rebaseline')).toHaveCount(1)
  await expect(page.locator('.interval-map-change-qualified')).not.toHaveAttribute('d', /NaN/)

  await page.getByRole('tab', { name: 'Compare' }).click()
  await expect(page.getByRole('note')).toContainText('Peak memory was reset')
})
