import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('follows appends, preserves older selection, and resets on replacement', async ({ page }) => {
  const sessionStart = JSON.stringify({
    schema_version: 4,
    event_type: 'session_start',
    session_id: 'live-a',
    timestamp: '2026-07-18T09:00:00-07:00',
    version: '0.1.8',
  })
  const interval0 = JSON.stringify({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'live-a',
    timestamp: '2026-07-18T09:01:00-07:00',
    interval: 0,
    interval_lines: 100,
    total_lines: 100,
    total_bytes: 1000,
    summary: {},
    runtime: {},
    matchers: [],
    interesting: [],
  })
  await page.addInitScript(({ initial }) => {
    const liveWindow = window as unknown as {
      __pattyContent: string
      __pattyModified: number
      showOpenFilePicker: () => Promise<Array<{ name: string; getFile: () => Promise<File> }>>
    }
    liveWindow.__pattyContent = initial
    liveWindow.__pattyModified = Date.now()
    Object.defineProperty(window, 'showOpenFilePicker', {
      configurable: true,
      value: async () => [{
        name: 'live.jsonl',
        getFile: async () => new File([liveWindow.__pattyContent], 'live.jsonl', {
          lastModified: liveWindow.__pattyModified,
          type: 'application/x-ndjson',
        }),
      }],
    })
  }, { initial: `${sessionStart}\n${interval0}\n` })

  await page.goto('/')
  await page.getByRole('button', { name: 'Open and follow' }).click()
  await expect(page.getByText('2 records')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Jul 18, 09:01 AM' })).toBeVisible()

  await page.getByRole('button', { name: /Session start/ }).click()
  await expect(page.getByRole('heading', { name: '0.1.8' })).toBeVisible()

  const interval1 = JSON.stringify({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'live-a',
    timestamp: '2026-07-18T09:02:00-07:00',
    interval: 1,
    interval_lines: 140,
    total_lines: 240,
    total_bytes: 2400,
    summary: {},
    runtime: {},
    matchers: [],
    interesting: [],
  })
  await page.evaluate((line) => {
    const liveWindow = window as unknown as { __pattyContent: string; __pattyModified: number }
    liveWindow.__pattyContent += `${line}\n`
    liveWindow.__pattyModified += 1
  }, interval1)

  await expect(page.getByText('3 records')).toBeVisible({ timeout: 4000 })
  await expect(page.getByRole('heading', { name: '0.1.8' })).toBeVisible()
  await page.getByRole('button', { name: 'Resume live' }).click()
  await expect(page.getByRole('heading', { name: 'Jul 18, 09:02 AM' })).toBeVisible()

  const replacementStart = JSON.stringify({
    schema_version: 4,
    event_type: 'session_start',
    session_id: 'live-b',
    timestamp: '2026-07-18T10:00:00-07:00',
    version: '0.1.9-dev',
  })
  const replacementInterval = JSON.stringify({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'live-b',
    timestamp: '2026-07-18T10:01:00-07:00',
    interval: 0,
    interval_lines: 12,
    total_lines: 12,
    total_bytes: 120,
    summary: {},
    runtime: {},
    matchers: [],
    interesting: [],
  })
  await page.evaluate(({ start, interval }) => {
    const liveWindow = window as unknown as { __pattyContent: string; __pattyModified: number }
    liveWindow.__pattyContent = `${start}\n${interval}\n`
    liveWindow.__pattyModified += 1
  }, { start: replacementStart, interval: replacementInterval })

  await expect(page.getByText('2 records')).toBeVisible({ timeout: 4000 })
  await expect(page.getByRole('heading', { name: 'Jul 18, 10:01 AM' })).toBeVisible()
  await expect(page.getByText('12', { exact: true }).first()).toBeVisible()
})
