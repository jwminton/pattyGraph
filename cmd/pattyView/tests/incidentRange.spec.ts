import { expect, test } from '@playwright/test'
import { bundleFixture, fixture } from './testSupport'

test('selects an investigation range without replacing ordinary map navigation', async ({ context, page }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.addInitScript(() => {
    Object.defineProperty(window, 'showSaveFilePicker', { configurable: true, value: undefined })
  })
  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles(fixture)

  const intervalMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  const bounds = await intervalMap.boundingBox()
  expect(bounds).not.toBeNull()

  // A short pointer gesture remains the existing interval click.
  await intervalMap.click({ position: { x: Math.max(1, (bounds?.width ?? 1) - 2), y: 8 } })
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:01 AM' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Investigation range' })).toHaveCount(0)

  // Drag from newest on the left to oldest on the right. The anchor becomes
  // the ordinary selection while the exported range remains in file order.
  await page.mouse.move((bounds?.x ?? 0) + 2, (bounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move((bounds?.x ?? 0) + (bounds?.width ?? 0) - 2, (bounds?.y ?? 0) + 8, { steps: 4 })
  await page.mouse.up()

  const range = page.getByRole('region', { name: 'Investigation range' })
  await expect(range).toBeVisible()
  await expect(range).toContainText('2 intervals')
  const expected = "pattyView --bundle 'schema4.jsonl' --from '2026-07-18T08:01:00-07:00' " +
    "--through '2026-07-18T08:02:00-07:00' --session 'test-session'"
  await expect(range.locator('code')).toHaveText(expected)
  await expect(page.locator('.interval-map-range')).toHaveAttribute('x', '0')
  await expect(page.locator('.interval-map-range')).toHaveAttribute('width', '2')
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:02 AM' })).toBeVisible()

  await page.getByRole('tab', { name: 'Compare' }).click()
  await expect(page.locator('.comparison-side-label.comparison')).toContainText('interval 0')
  await expect(page.locator('.comparison-side-label.selected')).toContainText('interval 1')
  await page.getByRole('tab', { name: 'Overview' }).click()

  await page.getByRole('button', { name: 'Copy bundle command' }).click()
  await expect(page.getByRole('button', { name: 'Bundle command copied' })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(expected)

  await page.getByRole('tab', { name: 'Traffic' }).click()
  await expect(range).toBeVisible()
  await page.getByRole('button', { name: 'Clear investigation range' }).click()
  await expect(range).toHaveCount(0)
  await expect(page.locator('.interval-map-range')).toHaveCount(0)

  const trafficBounds = await intervalMap.boundingBox()
  expect(trafficBounds).not.toBeNull()
  await page.mouse.move((trafficBounds?.x ?? 0) + 2, (trafficBounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move(
    (trafficBounds?.x ?? 0) + (trafficBounds?.width ?? 0) - 2,
    (trafficBounds?.y ?? 0) + 8,
    { steps: 4 },
  )
  await page.mouse.up()
  await expect(range).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(range).toHaveCount(0)
  await expect(page.locator('.interval-map-range')).toHaveCount(0)

  await page.mouse.move((trafficBounds?.x ?? 0) + 2, (trafficBounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move(
    (trafficBounds?.x ?? 0) + (trafficBounds?.width ?? 0) - 2,
    (trafficBounds?.y ?? 0) + 8,
    { steps: 4 },
  )
  await page.mouse.up()
  await expect(range).toBeVisible()
  const downloadStarted = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download incident' }).click()
  const download = await downloadStarted
  expect(download.suggestedFilename()).toBe(
    'schema4_20260718_0801-0802.incident.zip',
  )
  const downloadedPath = await download.path()
  expect(downloadedPath).not.toBeNull()
  await page.locator('input[type="file"]').setInputFiles(downloadedPath ?? '')
  await expect(page.locator('.source-status')).toHaveText('Bundle')
  await expect(page.getByText('6 records')).toBeVisible()
  const runtime = page.locator('.overview-section').filter({
    has: page.getByRole('heading', { name: 'Runtime' }),
  })
  await expect(runtime).toContainText('Bundle representation')
  await expect(runtime).toContainText('semantic')
  await expect(page.getByRole('heading', { name: 'Jul 18, 08:02 AM' })).toBeVisible()
})

test('keeps Compare targeting exclusive and derives immutable incident bundles', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'showSaveFilePicker', { configurable: true, value: undefined })
  })
  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles(fixture)

  await page.getByRole('tab', { name: 'Compare' }).click()
  await page.getByRole('button', { name: 'Pick from map' }).click()
  const intervalMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  const bounds = await intervalMap.boundingBox()
  expect(bounds).not.toBeNull()
  await page.mouse.move((bounds?.x ?? 0) + 2, (bounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move((bounds?.x ?? 0) + (bounds?.width ?? 0) - 2, (bounds?.y ?? 0) + 8, { steps: 4 })
  await page.mouse.up()
  await expect(page.getByRole('region', { name: 'Investigation range' })).toHaveCount(0)

  await page.getByRole('tab', { name: 'Overview' }).click()
  await page.mouse.move((bounds?.x ?? 0) + 2, (bounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move((bounds?.x ?? 0) + (bounds?.width ?? 0) - 2, (bounds?.y ?? 0) + 8, { steps: 4 })
  await page.mouse.up()
  await expect(page.getByRole('region', { name: 'Investigation range' })).toBeVisible()

  await page.locator('input[type="file"]').setInputFiles(bundleFixture)
  await expect(page.locator('.source-status')).toHaveText('Bundle')
  await expect(page.getByRole('region', { name: 'Investigation range' })).toHaveCount(0)
  const bundleMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  await expect(bundleMap).toHaveAttribute('title', /investigation range/)
  const bundleBounds = await bundleMap.boundingBox()
  expect(bundleBounds).not.toBeNull()
  await page.mouse.move((bundleBounds?.x ?? 0) + 2, (bundleBounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move((bundleBounds?.x ?? 0) + (bundleBounds?.width ?? 0) - 2, (bundleBounds?.y ?? 0) + 8, { steps: 4 })
  await page.mouse.up()
  const bundleRange = page.getByRole('region', { name: 'Investigation range' })
  await expect(bundleRange).toBeVisible()
  await expect(bundleRange).toContainText('2 intervals')
  await expect(page.getByRole('button', { name: 'Copy bundle command' })).toHaveCount(0)

  const downloadStarted = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download incident' }).click()
  const download = await downloadStarted
  expect(download.suggestedFilename()).toBe('schema4_20260718_0801-0802.incident.zip')
  const downloadedPath = await download.path()
  expect(downloadedPath).not.toBeNull()
  await page.locator('input[type="file"]').setInputFiles(downloadedPath ?? '')
  await expect(page.locator('.source-status')).toHaveText('Bundle')
  const runtime = page.locator('.overview-section').filter({
    has: page.getByRole('heading', { name: 'Runtime' }),
  })
  await expect(runtime).toContainText('Bundle representation')
  await expect(runtime).toContainText('semantic')
  await expect(runtime).toContainText('schema4.jsonl')

  const childMap = page.getByRole('slider', { name: 'Select a PattyLog interval' })
  const childBounds = await childMap.boundingBox()
  expect(childBounds).not.toBeNull()
  await page.mouse.move((childBounds?.x ?? 0) + 2, (childBounds?.y ?? 0) + 8)
  await page.mouse.down()
  await page.mouse.move((childBounds?.x ?? 0) + (childBounds?.width ?? 0) - 2, (childBounds?.y ?? 0) + 8, { steps: 4 })
  await page.mouse.up()
  await expect(page.getByRole('region', { name: 'Investigation range' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Download incident' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Copy bundle command' })).toHaveCount(0)

  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
})
