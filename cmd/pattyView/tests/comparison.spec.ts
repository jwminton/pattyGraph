import { readFileSync } from 'node:fs'
import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('compares two emitted interval snapshots and their recorded context', async ({ page }) => {
  const laterInterval = {
    schema_version: 4,
    event_type: 'interval',
    session_id: 'test-session',
    timestamp: '2026-07-18T08:03:00-07:00',
    log_time: '2026-07-18T08:03:00-07:00',
    phase: 'before_push',
    interval: 2,
    interval_lines: 900,
    matchers: [
      { name: 'Googlebot', interval_count: 72 },
      { name: 'Bots', interval_count: 21 },
      { name: 'bytes', interval_count: 2_800_000 },
      { name: 'errs', interval_count: 6 },
    ],
    interesting: [],
    factoids: [{ name: 'traffic.quiet', text: 'Traffic settled' }],
  }
  const fixtureContents = readFileSync(fixture, 'utf8').trim().split('\n').map((line) => {
    try {
      const value = JSON.parse(line)
      if (value.event_type === 'interval') {
        const words = value.interesting.find((stream: { name: string }) => stream.name === 'words')
        words.peaks = words.top.map((entry: Record<string, unknown>) => ({ ...entry, is_peak: true }))
        value.interesting.push({
          name: 'refs',
          total_keys: value.interval === 0 ? 80 : 92,
          top: [],
          peaks: [{
            key: 'direct',
            rank: 1,
            score: value.interval === 0 ? 90 : 130,
            count: value.interval === 0 ? 30 : 44,
            bytes: value.interval === 0 ? 2048 : 4096,
            is_peak: true,
          }],
        })
      }
      return JSON.stringify(value)
    } catch {
      return line
    }
  }).join('\n')
  const contents = `${fixtureContents}\n${JSON.stringify(laterInterval)}`

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'comparison.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(contents),
  })

  await page.getByRole('button', { name: /Interval 1/ }).click()
  await page.getByRole('tab', { name: 'Compare' }).click()
  const intervalMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  const selectedRail = page.locator('.interval-map-selected-rail')
  const originRail = page.locator('.interval-map-comparison-origin-rail')
  await expect(selectedRail).toHaveClass(/comparison/)
  await expect(selectedRail).toHaveAttribute('x1', '2.5')
  await expect(originRail).toHaveAttribute('x1', '1.5')

  const reference = page.getByRole('combobox', { name: 'Compare selected interval with' })
  await expect(reference.locator('option:checked')).toContainText('interval 0')
  await expect(page.locator('.comparison-side-label.comparison')).toContainText('interval 0')
  await expect(page.locator('.comparison-side-label.selected')).toContainText('interval 1')

  const signals = page.getByRole('table', { name: 'Interval signal comparison' })
  const linesRow = signals.getByRole('row').filter({ hasText: 'Lines' })
  await expect(linesRow).toContainText(/Lines1,4501,200\+250/)
  await expect(linesRow.getByLabel('Increased: +250')).toBeVisible()
  await expect(signals.getByRole('row').filter({ hasText: 'Bytes' })).toContainText(/Bytes4.2 MiB3.2 MiB\+977 KiB/)

  const matchers = page.getByRole('table', { name: 'Matcher comparison' })
  await expect(matchers.getByRole('row').filter({ hasText: 'Googlebot' })).toContainText(/Googlebot1102188\+14/)
  const botsRow = matchers.getByRole('row').filter({ hasText: 'Bots' })
  await expect(botsRow).toContainText(/Bots228232-4/)
  await expect(botsRow.getByLabel('Decreased: -4')).toBeVisible()

  const wordPeaks = page.getByRole('table', { name: 'Words peak comparison' })
  await expect(wordPeaks.getByRole('row').filter({ hasText: 'catalog' })).toContainText(/280.*catalog.*220/)
  await expect(wordPeaks.locator('.peak-bar span')).toHaveCount(2)
  const refPeaks = page.getByRole('table', { name: 'Refs peak comparison' })
  await expect(refPeaks.getByRole('row').filter({ hasText: 'direct' })).toContainText(/130.*direct.*90/)

  const context = page.locator('.context-columns .comparison-context-column')
  await expect(context.first()).toContainText('deployment started # edge pool')
  await expect(context.first()).toContainText('Active Alerts:errs')
  await expect(context.last()).toContainText('Pressure p:5 g:30 s:1.0 f:3')
  await expect(context.last()).toContainText('errs triggered')

  const interval2Value = await reference.locator('option').filter({ hasText: 'interval 2' }).getAttribute('value')
  expect(interval2Value).not.toBeNull()
  const pickFromMap = page.getByRole('button', { name: 'Pick from map' })
  await pickFromMap.click()
  await reference.selectOption(interval2Value ?? '')
  await expect(pickFromMap).toHaveAttribute('aria-pressed', 'false')
  await expect(page.locator('.comparison-side-label.comparison')).toContainText('interval 2')
  await expect(selectedRail).toHaveAttribute('x1', '0.5')
  const intervalMapBounds = await intervalMap.boundingBox()
  expect(intervalMapBounds).not.toBeNull()

  await pickFromMap.click()
  await expect(page.getByRole('button', { name: 'Choosing interval...' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByText('Select comparison interval')).toBeVisible()
  await intervalMap.click({ position: { x: (intervalMapBounds?.width ?? 2) / 2, y: 8 } })
  await expect(page.getByRole('button', { name: 'Choosing interval...' })).toHaveAttribute('aria-pressed', 'true')
  await expect(reference.locator('option:checked')).toContainText('interval 2')
  await page.keyboard.press('Escape')
  await expect(pickFromMap).toHaveAttribute('aria-pressed', 'false')
  await expect(reference.locator('option:checked')).toContainText('interval 2')

  await pickFromMap.click()
  await intervalMap.click({
    position: { x: Math.max(1, (intervalMapBounds?.width ?? 1) - 1), y: 8 },
  })
  await expect(page.getByRole('tab', { name: 'Compare' })).toHaveAttribute('aria-selected', 'true')
  await expect(reference.locator('option:checked')).toContainText('interval 0')
  await expect(page.locator('.comparison-side-label.comparison')).toContainText('interval 0')
  await expect(page.locator('.comparison-side-label.selected')).toContainText('interval 1')
  await expect(selectedRail).toHaveClass(/comparison/)
  await expect(selectedRail).toHaveAttribute('x1', '2.5')
  await expect(originRail).toHaveAttribute('x1', '1.5')
  await expect(pickFromMap).toHaveAttribute('aria-pressed', 'false')

  await intervalMap.click({ position: { x: 1, y: 8 } })
  await expect(page.locator('.comparison-side-label.comparison')).toContainText('interval 1')
  await expect(page.locator('.comparison-side-label.selected')).toContainText('interval 2')
  await expect(selectedRail).toHaveAttribute('x1', '1.5')
  await expect(originRail).toHaveAttribute('x1', '0.5')

  await page.getByRole('tab', { name: 'Overview' }).click()
  await expect(page.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:03 AM' })).toBeVisible()
  await expect(selectedRail).not.toHaveClass(/comparison/)
  await expect(selectedRail).toHaveAttribute('x1', '0.5')
  await expect(originRail).toHaveCount(0)

  await page.getByRole('tab', { name: 'Compare' }).click()
  await page.getByRole('button', { name: /Control command/ }).click()
  await expect(page.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:03 AM' })).toBeVisible()
  await expect(selectedRail).not.toHaveClass(/comparison/)
  await expect(originRail).toHaveCount(0)

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
})

