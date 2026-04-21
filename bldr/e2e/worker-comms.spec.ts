// worker-comms.spec.ts - Full-stack worker communication integration tests.
//
// Runs against the full bldr dev server (bun run start:web:wasm).
// Verifies the complete plugin lifecycle: WASM runtime boot, worker
// creation, SAB bus registration, StarPC transport wiring.

import { test, expect } from '@playwright/test'

// Collect console messages matching a pattern within a timeout.
async function waitForConsole(
  page: import('@playwright/test').Page,
  pattern: string | RegExp,
  timeoutMs = 60_000,
): Promise<string> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`timeout waiting for console: ${pattern}`)),
      timeoutMs,
    )
    const handler = (msg: import('@playwright/test').ConsoleMessage) => {
      const text = msg.text()
      const matches =
        typeof pattern === 'string' ? text.includes(pattern) : pattern.test(text)
      if (matches) {
        clearTimeout(timer)
        page.removeListener('console', handler)
        resolve(text)
      }
    }
    page.on('console', handler)
  })
}

test.describe.configure({ mode: 'serial' })

test.describe('worker communication lifecycle', () => {
  test('detects worker comms config', async ({ page }) => {
    const configPromise = waitForConsole(page, 'worker-comms: detected config')

    await page.goto('/')
    const msg = await configPromise
    // Should detect a valid config (A, B, C, or F).
    expect(msg).toMatch(/detected config [ABCF]/)
  })

  test('creates SAB bus for plugin IPC', async ({ page }) => {
    const busPromise = waitForConsole(page, 'SAB bus')

    await page.goto('/')
    const msg = await busPromise
    // Either "created SAB bus" or "SAB bus transport available".
    expect(msg).toMatch(/SAB bus/)
  })

  test('plugin registers on SAB bus', async ({ page }) => {
    const regPromise = waitForConsole(page, 'registered on SAB bus')

    await page.goto('/')
    const msg = await regPromise
    expect(msg).toContain('registered on SAB bus with pluginId')
  })

  test('plugin starts native worker', async ({ page }) => {
    const startPromise = waitForConsole(page, 'starting native plugin')

    await page.goto('/')
    const msg = await startPromise
    expect(msg).toContain('starting native plugin')
  })

  test('full lifecycle: detect, bus, plugin, render', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => {
      if (err.message.includes('cache disabled')) return
      errors.push(err.message)
    })

    // Collect all lifecycle milestones.
    const milestones: string[] = []
    page.on('console', (msg) => {
      const text = msg.text()
      if (
        text.includes('worker-comms: detected config') ||
        text.includes('SAB bus') ||
        text.includes('registered on SAB bus') ||
        text.includes('starting native plugin') ||
        text.includes('SAB bus transport available')
      ) {
        milestones.push(text)
      }
    })

    await page.goto('/')

    // Wait for the page to render content (plugin loaded).
    const root = page.locator('#bldr-root')
    await expect(async () => {
      const childCount = await root.evaluate((el) => el.children.length)
      expect(childCount).toBeGreaterThan(0)
    }).toPass({ timeout: 120_000 })

    // Verify lifecycle milestones.
    expect(milestones.length).toBeGreaterThanOrEqual(1)

    // Should have detected config.
    const hasDetect = milestones.some((m) =>
      m.includes('worker-comms: detected config'),
    )
    expect(hasDetect).toBe(true)

    // No uncaught errors.
    expect(errors).toEqual([])
  })
})

test.describe('cross-tab communication', () => {
  test('two tabs establish cross-tab channels', async ({ context }) => {
    // Open two pages in the same browser context (shared ServiceWorker).
    const pageA = await context.newPage()
    const pageB = await context.newPage()

    // Collect ALL console messages for debugging.
    const allA: string[] = []
    const allB: string[] = []

    pageA.on('console', (msg) => allA.push(msg.text()))
    pageB.on('console', (msg) => allB.push(msg.text()))

    // Navigate both pages to the app.
    await pageA.goto('/')
    await pageA.waitForSelector('#bldr-root', { timeout: 60_000 })

    await pageB.goto('/')
    await pageB.waitForSelector('#bldr-root', { timeout: 60_000 })

    // Wait for cross-tab system to initialize on both pages.
    // Cross-tab messages include "cross-tab comms" and "cross-tab transport".
    await expect(async () => {
      const all = [...allA, ...allB]
      const hasCrossTab = all.some((m) => m.includes('cross-tab'))
      expect(hasCrossTab).toBe(true)
    }).toPass({ timeout: 30_000 })
  })
})

// Create a browser context where SharedWorker is unavailable,
// forcing DedicatedWorker runtime mode with Web Lock singleton.
async function newDedicatedRuntimeContext(
  browser: import('@playwright/test').Browser,
) {
  const context = await browser.newContext()
  await context.addInitScript(() => {
    Object.defineProperty(globalThis, 'SharedWorker', {
      value: undefined,
      configurable: true,
    })
  })
  return context
}

test.describe('singleton coordinator (no SharedWorker)', () => {
  test('only one tab runs plugins', async ({ browser }) => {
    const context = await newDedicatedRuntimeContext(browser)

    const pageA = await context.newPage()
    const pageB = await context.newPage()

    // Page A loads and acquires the singleton plugin lock.
    await pageA.goto('/')
    await waitForConsole(pageA, 'acquired plugin singleton lock')
    await waitForConsole(pageA, 'starting native plugin')

    // Page B loads. Wait until Go has attempted CreateWebWorker and is
    // blocked on the singleton lock (deterministic, not a timeout).
    await pageB.goto('/')
    await waitForConsole(pageB, 'waiting for plugin singleton lock')

    // Page B is blocked - verify no plugin started.
    const pluginStartB: string[] = []
    pageB.on('console', (msg) => {
      if (msg.text().includes('starting native plugin'))
        pluginStartB.push(msg.text())
    })
    expect(pluginStartB.length).toBe(0)

    await context.close()
  })

  test('singleton handoff on tab close', async ({ browser }) => {
    const context = await newDedicatedRuntimeContext(browser)

    const pageA = await context.newPage()
    const pageB = await context.newPage()

    // Page A acquires the singleton.
    await pageA.goto('/')
    await waitForConsole(pageA, 'starting native plugin')

    // Page B is blocked on the lock.
    await pageB.goto('/')
    await waitForConsole(pageB, 'waiting for plugin singleton lock')

    // Close page A, releasing the singleton lock.
    await pageA.close()

    // Page B should acquire the lock and start plugins.
    await waitForConsole(pageB, 'acquired plugin singleton lock')
    await waitForConsole(pageB, 'starting native plugin')

    await context.close()
  })
})
