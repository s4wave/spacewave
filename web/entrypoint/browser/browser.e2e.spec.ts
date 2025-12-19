import { test, expect } from '@playwright/test'

test.describe('Web Release Build E2E', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    // Wait for app to initialize
    await page.waitForSelector('#bldr-root', { timeout: 10000 })
  })

  test('should have bldr-root element', async ({ page }) => {
    const root = page.locator('#bldr-root')
    await expect(root).toBeVisible()
  })

  test('should register service worker', async ({ page }) => {
    // Wait for service worker to register
    await expect(async () => {
      const swCount = await page.evaluate(async () => {
        const regs = await navigator.serviceWorker.getRegistrations()
        return regs.length
      })
      expect(swCount).toBeGreaterThan(0)
    }).toPass({ timeout: 10000 })
  })

  test('should render content', async ({ page }) => {
    const root = page.locator('#bldr-root')
    // Wait for root to have children
    await expect(async () => {
      const childCount = await root.evaluate((el) => el.children.length)
      expect(childCount).toBeGreaterThan(0)
    }).toPass({ timeout: 10000 })
  })

  test('should complete loading', async ({ page }) => {
    const root = page.locator('#bldr-root')
    // Wait for loading to complete (no "Loading" text and has content)
    await expect(async () => {
      const text = await root.textContent()
      expect(text).not.toContain('Loading')
      expect(text?.length).toBeGreaterThan(0)
    }).toPass({ timeout: 15000 })
  })
})
