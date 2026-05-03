import { test, expect } from '@playwright/test'

test.describe('Web Release Build E2E', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/')
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
    await expect(async () => {
      const text = await root.textContent()
      expect(text?.trim().length).toBeGreaterThan(0)
    }).toPass({ timeout: 10000 })
  })

  test('should complete loading', async ({ page }) => {
    const root = page.locator('#bldr-root')
    await expect(async () => {
      const boot = await page.evaluate(() => {
        const g = globalThis as {
          __swBootStatus?: { state?: string }
          __swEntry?: string
        }
        return {
          bootState: g.__swBootStatus?.state ?? '',
          entrypoint: g.__swEntry ?? '',
        }
      })
      expect(boot.bootState).not.toBe('error')
      expect(boot.entrypoint).toContain('/entrypoint/')
      const text = (await root.textContent()) ?? ''
      expect(text.trim().length).toBeGreaterThan(0)
    }).toPass({ timeout: 15000 })
  })
})
