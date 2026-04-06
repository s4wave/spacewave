import { test, expect } from '@playwright/test'

test.describe('bldr demo startup', () => {
  test('page loads with bldr-root', async ({ page }) => {
    await page.goto('/')
    await page.waitForSelector('#bldr-root', { timeout: 60_000 })
    const root = page.locator('#bldr-root')
    await expect(root).toBeVisible()
  })

  test('renders content after wasm load', async ({ page }) => {
    await page.goto('/')
    const root = page.locator('#bldr-root')
    await expect(async () => {
      const childCount = await root.evaluate((el) => el.children.length)
      expect(childCount).toBeGreaterThan(0)
    }).toPass({ timeout: 120_000 })
  })

  test('no uncaught errors', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => {
      // Ignore benign cache-reload message from service worker.
      if (err.message.includes('cache disabled')) return
      errors.push(err.message)
    })
    await page.goto('/')
    await page.waitForSelector('#bldr-root', { timeout: 60_000 })
    // Allow time for wasm to load and initialize.
    await page.waitForTimeout(5_000)
    expect(errors).toEqual([])
  })
})
