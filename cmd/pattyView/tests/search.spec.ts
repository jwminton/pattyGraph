import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('uses a synthetic search lane to locate emitted traffic and factoid records', async ({ page }) => {
  const values = [
    {
      schema_version: 4,
      event_type: 'session_start',
      session_id: 'search-lane',
      timestamp: '1970-01-01T00:00:00Z',
      log_time: '1970-01-01T00:00:00Z',
      version: '0.1.8',
    },
    {
      schema_version: 4,
      event_type: 'interval',
      session_id: 'search-lane',
      timestamp: '2026-07-19T09:01:00-07:00',
      log_time: '2019-01-22T11:08:00-08:00',
      interval: 0,
      interval_lines: 120,
      matchers: [
        { name: 'bytes', interval_count: 4096 },
        { name: 'errs', interval_count: 2 },
      ],
      interesting: [
        {
          name: 'words',
          top: [{ key: 'ProductModel' }, { key: 'deployment' }, { key: 'undeployed' }, { key: 'catalog' }],
          peaks: [{ key: 'ProductModel' }, { key: 'deployed' }],
        },
        {
          name: 'refs',
          top: [{ key: 'https://deploy.example.test/release' }, { key: 'direct' }],
          peaks: [],
        },
        {
          name: 'ips',
          top: [{ key: '203.0.113.17' }],
          peaks: [],
          ip_groups: [{ prefix: '203.0.' }],
        },
      ],
      runtime: {},
    },
    {
      schema_version: 4,
      event_type: 'control_command',
      session_id: 'search-lane',
      timestamp: '2019-01-22T11:08:52-08:00',
      log_time: '2019-01-22T11:08:52-08:00',
      command_name: 'fact',
      command: '!!! fact print PattyView search marker: deployment rolled back after the weekend change',
      status: 'applied',
      result: {
        fact: 'print',
        text: 'Note: PattyView search marker: deployment rolled back after the weekend change',
      },
    },
    {
      schema_version: 4,
      event_type: 'interval',
      session_id: 'search-lane',
      timestamp: '2026-07-19T09:02:00-07:00',
      log_time: '2019-01-22T11:09:00-08:00',
      interval: 1,
      interval_lines: 140,
      matchers: [
        { name: 'bytes', interval_count: 8192 },
        { name: 'errs', interval_count: 1 },
      ],
      interesting: [],
      factoids: [{ name: 'print', text: 'Note: deployment rolled back after the weekend change' }],
      runtime: {},
    },
  ]

  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'search.jsonl',
    mimeType: 'application/x-ndjson',
    buffer: Buffer.from(values.map((value) => JSON.stringify(value)).join('\n')),
  })

  const search = page.getByRole('searchbox', { name: 'Search PattyLog' })
  const intervalMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  await search.fill('PattyView search marker')
  await expect(page.locator('.header-search')).toContainText('1 record')
  await expect(page.locator('.record-list-item.selected')).toHaveClass(/event-control_command/)
  await expect(page.getByRole('heading', { name: 'fact' })).toBeVisible()

  await search.fill('weekend')
  await expect(page.locator('.header-search')).toContainText('2 records')
  await expect(page.locator('.search-lane-label')).toHaveText('SEARCH')
  await expect(page.locator('.interval-map-search')).toHaveAttribute('d', 'M0.5 78V66')
  await intervalMap.click({ position: { x: 1, y: 72 } })
  await expect(page.getByRole('heading', { name: 'fact' })).toBeVisible()
  await expect(page.locator('.record-list-item.selected')).toHaveClass(/event-control_command/)
  await expect(page.getByRole('heading', { name: 'fact' }).locator('xpath=..')).toContainText('Jan 22, 11:08:52 AM')

  await search.fill('productmodel')
  await expect(page.locator('.interval-map-search')).toHaveAttribute('d', 'M1.5 78V66')
  const intervalMapBounds = await intervalMap.boundingBox()
  expect(intervalMapBounds).not.toBeNull()
  await intervalMap.click({ position: { x: Math.max(1, (intervalMapBounds?.width ?? 1) - 1), y: 72 } })
  await expect(page.getByRole('heading', { name: 'Jan 22, 11:08 AM' })).toBeVisible()
  await expect(page.locator('.record-list-item.selected')).toContainText('Interval 0')

  await page.getByRole('tab', { name: 'Traffic' }).click()
  await search.fill('deploy')
  const words = page.getByRole('table', { name: 'words top' })
  await expect(words.locator('.search-text-match')).toHaveText(['deployment', 'undeployed'])
  await expect(words.getByText('catalog', { exact: true })).not.toHaveClass(/search-text-match/)

  await page.getByRole('tab', { name: 'Peaks', exact: true }).click()
  await expect(page.getByRole('table', { name: 'words peaks' }).locator('.search-text-match')).toHaveText('deployed')
  await page.getByRole('tab', { name: 'Refs', exact: true }).click()
  await page.getByRole('tab', { name: 'Top', exact: true }).click()
  await expect(page.getByRole('table', { name: 'refs top' }).locator('.search-text-match'))
    .toHaveText('https://deploy.example.test/release')

  await search.fill('203.0')
  await page.getByRole('tab', { name: 'IPs', exact: true }).click()
  await expect(page.getByRole('table', { name: 'ips top' }).locator('.search-text-match')).toHaveText('203.0.113.17')
  await expect(page.getByRole('table', { name: 'IP groups' }).locator('.search-text-match')).toHaveText('203.0.')

  await search.fill('does-not-exist')
  await expect(page.locator('.header-search')).toContainText('0 records')
  await expect(page.locator('.search-lane-label')).toHaveText('SEARCH')
  await expect(page.locator('.interval-map-search')).toHaveAttribute('d', '')
  await expect(page.locator('.record-list-item')).toHaveCount(4)
  await expect(page.locator('.search-text-match')).toHaveCount(0)

  await page.getByRole('button', { name: 'Clear search' }).click()
  await expect(page.locator('.search-lane-label')).toHaveCount(0)
  await expect(page.locator('.interval-map-search')).toHaveCount(0)
})
