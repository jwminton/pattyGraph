import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('virtualizes a large record navigator while preserving off-screen selection', async ({ page }) => {
  const sessionStart = {
    schema_version: 4,
    event_type: 'session_start',
    session_id: 'virtual-session',
    timestamp: '2026-07-18T08:00:00-07:00',
    version: 'virtual-test',
  }
  const intervals = Array.from({ length: 600 }, (_, interval) => ({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'virtual-session',
    timestamp: new Date(Date.UTC(2026, 6, 18, 15, interval + 1)).toISOString(),
    interval,
    interval_lines: 100 + interval,
    total_lines: 100 + interval,
    total_bytes: 1000 + interval,
    summary: {},
    runtime: {},
    matchers: [],
    interesting: [],
  }))
  const contents = [sessionStart, ...intervals].map((record) => JSON.stringify(record)).join('\n')

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'virtual.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(`${contents}\n`),
  })

  await expect(page.getByText('601 records')).toBeVisible()
  const renderedRecordCount = await page.locator('.record-list-item').count()
  expect(renderedRecordCount).toBeGreaterThan(0)
  expect(renderedRecordCount).toBeLessThan(50)

  const intervalMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  await intervalMap.press('End')
  await expect(page.locator('.record-list-item.selected')).toContainText('Interval 0')
  await expect(page.locator('.record-list-item.selected')).toBeInViewport()

  const recordList = page.locator('.record-list')
  await recordList.evaluate((element) => {
    element.scrollTop = element.scrollHeight
    element.dispatchEvent(new Event('scroll'))
  })
  const sessionButton = page.getByRole('button', { name: /Session start/ })
  await expect(sessionButton).toBeVisible()
  await sessionButton.click()
  await expect(page.getByRole('heading', { name: 'virtual-test' })).toBeVisible()
})

