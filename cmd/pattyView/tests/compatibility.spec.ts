import { expect, test } from '@playwright/test'
import { fixture } from './testSupport'

test('preserves unknown event types for inspection', async ({ page }) => {
  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles(fixture)

  await page.getByRole('button', { name: /future_event/ }).click()
  await expect(page.getByText('Unknown event type.')).toBeVisible()
  await expect(page.getByRole('tab', { name: 'Traffic' })).toBeDisabled()
  await page.getByRole('tab', { name: 'Record' }).click()
  await expect(page.locator('.raw-pane')).toContainText('future_value')
})

test('falls back to snapshot opening without the file handle API', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'showOpenFilePicker', {
      configurable: true,
      value: undefined,
    })
  })
  await page.goto('/')

  await expect(page.getByRole('main').getByRole('button', { name: 'Open snapshot / bundle' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open a snapshot or incident bundle' })).toHaveCount(0)
  await page.locator('input[type="file"]').setInputFiles(fixture)
  await expect(page.getByText('6 records')).toBeVisible()
  await expect(page.getByText('Snapshot', { exact: true })).toBeVisible()
})
