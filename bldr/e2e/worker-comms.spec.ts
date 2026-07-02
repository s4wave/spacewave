// worker-comms.spec.ts - Full-stack worker communication integration tests.
//
// Runs against the full bldr dev server (bun run start:web:wasm).
// Verifies the complete plugin lifecycle: WASM runtime boot, worker
// creation, plugin startup, and absence of eager SAB bus registration.

import { test, expect } from '@playwright/test'
import type { Browser, ConsoleMessage, Page } from '@playwright/test'

import { startupMarkEvent } from '../web/bldr/startup-marks.js'

const workerCommsTimeoutMs = 300_000

interface StartupMark {
  name: string
  label: string
  sequence: number
  timestampMs?: number
  detail: Record<string, unknown>
}

interface StartupMarkGlobal {
  __swStartupMarks?: StartupMark[]
}

interface StartupMarkEventPayload {
  name?: string
  detail?: Record<string, unknown>
}

// Collect console messages matching a pattern within a timeout.
async function waitForConsole(
  page: Page,
  pattern: string | RegExp,
  timeoutMs = workerCommsTimeoutMs,
): Promise<string> {
  const { promise, resolve, reject } = Promise.withResolvers<string>()
  const timer = setTimeout(
    () => reject(new Error(`timeout waiting for console: ${pattern}`)),
    timeoutMs,
  )
  const handler = (msg: ConsoleMessage) => {
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
  return promise
}

interface StartupMarkWaiter {
  label: string
  resolve: (mark: StartupMark) => void
}

interface StartupMarkCollector {
  count(label: string): number
  dump(): string
  wait(label: string, timeoutMs?: number): Promise<StartupMark>
}

let startupMarkCollectorID = 0

function formatStartupMarkDump(marks: StartupMark[]): string {
  if (!marks.length) {
    return '(none)'
  }
  return marks
    .map((mark, index) => {
      const sequence =
        mark.sequence > 0 ? mark.sequence.toString() : (index + 1).toString()
      const timestamp =
        typeof mark.timestampMs === 'number'
          ? `${mark.timestampMs.toFixed(3)}ms`
          : 'unknown-ms'
      return `${sequence}. ${mark.label} @ ${timestamp}`
    })
    .join('\n')
}

async function createStartupMarkCollector(
  page: Page,
): Promise<StartupMarkCollector> {
  const marks: StartupMark[] = []
  const waiters: StartupMarkWaiter[] = []
  const callbackName = `__bldrE2EStartupMark${++startupMarkCollectorID}`

  const removeWaiter = (waiter: StartupMarkWaiter) => {
    const index = waiters.indexOf(waiter)
    if (index >= 0) {
      waiters.splice(index, 1)
    }
  }
  const recordMark = (mark: StartupMark) => {
    marks.push(mark)
    for (const waiter of [...waiters]) {
      if (waiter.label === mark.label) {
        waiter.resolve(mark)
      }
    }
  }

  await page.exposeFunction(callbackName, recordMark)
  await page.addInitScript(
    ({ callbackName, startupMarkEvent }) => {
      const startupGlobal = globalThis as typeof globalThis &
        StartupMarkGlobal &
        Record<string, unknown>
      const pushMark = (mark: StartupMark) => {
        const callback = startupGlobal[callbackName]
        if (typeof callback !== 'function') {
          return
        }
        if (typeof performance === 'undefined') {
          void (callback as (mark: StartupMark) => void)(mark)
          return
        }
        void (callback as (mark: StartupMark) => void)({
          ...mark,
          timestampMs: performance.now(),
        })
      }
      for (const mark of startupGlobal.__swStartupMarks ?? []) {
        pushMark(mark)
      }
      globalThis.addEventListener(startupMarkEvent, (event: Event) => {
        const payload = (event as CustomEvent<StartupMarkEventPayload>).detail
        const detail = payload?.detail ?? {}
        const label = detail.label
        if (typeof label !== 'string') {
          return
        }
        pushMark({
          name: payload?.name ?? '',
          label,
          sequence: Number(detail.sequence ?? 0),
          detail,
        })
      })
    },
    { callbackName, startupMarkEvent },
  )

  return {
    count(label: string) {
      return marks.filter((mark) => mark.label === label).length
    },
    dump() {
      return formatStartupMarkDump(marks)
    },
    wait(label: string, timeoutMs = workerCommsTimeoutMs) {
      const existing = marks.find((mark) => mark.label === label)
      if (existing) {
        return Promise.resolve(existing)
      }

      const { promise, resolve, reject } = Promise.withResolvers<StartupMark>()
      const waiter: StartupMarkWaiter = {
        label,
        resolve: (mark: StartupMark) => {
          clearTimeout(timer)
          removeWaiter(waiter)
          resolve(mark)
        },
      }
      const timer = setTimeout(() => {
        removeWaiter(waiter)
        reject(
          new Error(
            `timeout waiting for startup mark: ${label}\nCollected startup marks:\n${formatStartupMarkDump(marks)}`,
          ),
        )
      }, timeoutMs)
      waiters.push(waiter)
      return promise
    },
  }
}

test.describe.configure({ mode: 'serial', timeout: 360_000 })

// TIER: pr
test.describe('worker communication lifecycle', () => {
  test('detects worker comms config', async ({ page }) => {
    const configPromise = waitForConsole(page, 'worker-comms: detected config')

    await page.goto('/#/')
    const msg = await configPromise
    // Should detect a valid config (A, B, C, or F).
    expect(msg).toMatch(/detected config [ABCF]/)
  })

  test('does not eagerly register plugins on SAB bus', async ({ page }) => {
    const busMessages: string[] = []
    page.on('console', (msg) => {
      const text = msg.text()
      if (
        text.includes('SAB bus') ||
        text.includes('registered on SAB bus') ||
        text.includes('SabBus: max readers')
      ) {
        busMessages.push(text)
      }
    })

    await page.goto('/#/')
    await expect(page.locator('#bldr-root')).toBeVisible()
    expect(busMessages).toEqual([])
  })

  test('plugin starts native worker', async ({ page }) => {
    const startupMarks = await createStartupMarkCollector(page)

    await page.goto('/#/')
    const startMark = await startupMarks.wait('plugin.script-import-start')
    expect(startMark.detail).toMatchObject({ plugin: true })
  })

  test('full lifecycle: detect, bus, plugin, render', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => {
      if (err.message.includes('cache disabled')) return
      errors.push(err.message)
    })

    // Collect all lifecycle milestones.
    const milestones: string[] = []
    const forbiddenBusMessages: string[] = []
    page.on('console', (msg) => {
      const text = msg.text()
      if (
        text.includes('worker-comms: detected config') ||
        text.includes('starting native plugin')
      ) {
        milestones.push(text)
      }
      if (
        text.includes('SAB bus') ||
        text.includes('registered on SAB bus') ||
        text.includes('SabBus: max readers')
      ) {
        forbiddenBusMessages.push(text)
      }
    })

    await page.goto('/#/')

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
    expect(forbiddenBusMessages).toEqual([])

    // No uncaught errors.
    expect(errors).toEqual([])
  })
})

test.describe('cross-tab communication', () => {
  test('two tabs establish cross-tab channels', async ({ context }) => {
    // Open two pages in the same browser context (shared ServiceWorker).
    const [pageA, pageB] = await Promise.all([
      context.newPage(),
      context.newPage(),
    ])

    // Collect ALL console messages for debugging.
    const allA: string[] = []
    const allB: string[] = []

    pageA.on('console', (msg) => allA.push(msg.text()))
    pageB.on('console', (msg) => allB.push(msg.text()))

    // Navigate both pages to the app.
    await pageA.goto('/#/')
    await pageA.waitForSelector('#bldr-root', { timeout: 60_000 })

    await pageB.goto('/#/')
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
async function newDedicatedRuntimeContext(browser: Browser) {
  const context = await browser.newContext()
  await context.addInitScript(() => {
    Object.defineProperty(globalThis, 'SharedWorker', {
      value: undefined,
      configurable: true,
    })
  })
  return context
}

async function waitForBldrRootRender(page: Page) {
  const root = page.locator('#bldr-root')
  await expect(root).toBeVisible()
  await expect(async () => {
    const text = await root.evaluate((el) => el.textContent ?? '')
    expect(text).not.toContain('Downloading the app bundle')
  }).toPass({ timeout: 120_000 })
}

test.describe('singleton coordinator (no SharedWorker)', () => {
  test('only one tab runs plugins', async ({ browser }) => {
    const context = await newDedicatedRuntimeContext(browser)

    const [pageA, pageB] = await Promise.all([
      context.newPage(),
      context.newPage(),
    ])

    const pageAMarks = await createStartupMarkCollector(pageA)
    const pageBMarks = await createStartupMarkCollector(pageB)

    // Page A loads and acquires the singleton plugin lock.
    await pageA.goto('/#/')
    await pageAMarks.wait('singleton-lock.acquired')
    await pageAMarks.wait('plugin.script-import-start')

    // Page B loads without becoming another plugin host.
    await pageB.goto('/#/')
    await pageBMarks.wait('worker-comms.detected')
    await waitForBldrRootRender(pageB)

    expect(pageBMarks.count('plugin.script-import-start')).toBe(0)

    await context.close()
  })

  test('singleton handoff on tab close', async ({ browser }) => {
    const context = await newDedicatedRuntimeContext(browser)

    const [pageA, pageB] = await Promise.all([
      context.newPage(),
      context.newPage(),
    ])

    const pageAMarks = await createStartupMarkCollector(pageA)
    const pageBMarks = await createStartupMarkCollector(pageB)

    // Page A acquires the singleton.
    await pageA.goto('/#/')
    await pageAMarks.wait('plugin.script-import-start')

    // Page B attaches to the existing runtime without starting plugins.
    await pageB.goto('/#/')
    await pageBMarks.wait('worker-comms.detected')
    await waitForBldrRootRender(pageB)
    expect(pageBMarks.count('plugin.script-import-start')).toBe(0)

    const pageBLock = pageBMarks.wait('singleton-lock.acquired')
    const pageBStart = pageBMarks.wait('plugin.script-import-start')

    // Close page A, releasing the singleton lock.
    await pageA.close()

    try {
      // Page B should acquire the lock and start plugins.
      await pageBLock
      const startMark = await pageBStart
      expect(startMark.detail).toMatchObject({ plugin: true })
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      throw new Error(
        `${message}\nPage A startup marks:\n${pageAMarks.dump()}\nPage B startup marks:\n${pageBMarks.dump()}`,
        { cause: err },
      )
    }

    await context.close()
  })
})
