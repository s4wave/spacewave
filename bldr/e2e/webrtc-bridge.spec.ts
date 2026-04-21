// webrtc-bridge.spec.ts - WebRTC bridge bootstrap verification.
//
// Runs against the full bldr dev server (bun run start:web:wasm).
// Verifies that:
//   1. The WebDocument creates a bridge endpoint for the worker.
//   2. No bridge-related errors during startup.
//
// The cross-browser test launches Firefox and Chromium simultaneously
// and verifies both browsers bootstrap the bridge independently.
//
// Note: verifying the worker-side shim installation or actual PC creation
// through the bridge requires either worker console capture (unreliable in
// Playwright for WASM workers) or a signaling peer. The alpha e2e/wasm
// Go test harness is better suited for full-stack bridge integration tests.

import { test, expect, chromium, firefox } from '@playwright/test'
import type { Page, ConsoleMessage, Worker } from '@playwright/test'

async function waitForConsole(
  page: Page,
  pattern: string | RegExp,
  timeoutMs = 60_000,
): Promise<string> {
  return new Promise((resolve, reject) => {
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
  })
}

test.describe('WebRTC bridge bootstrap', () => {
  test('WebDocument opens bridge for worker', async ({ page }) => {
    const bridgePromise = waitForConsole(
      page,
      'WebDocument: WebRTC bridge opened for',
    )

    await page.goto('/')

    const msg = await bridgePromise
    expect(msg).toContain('WebRTC bridge opened for')
  })

  test('no bridge-related errors during startup', async ({ page }) => {
    const pageErrors: string[] = []
    page.on('pageerror', (err) => {
      if (err.message.includes('cache disabled')) return
      pageErrors.push(err.message)
    })

    const consoleErrors: string[] = []
    const errorHandler = (msg: ConsoleMessage) => {
      if (msg.type() === 'error' || msg.type() === 'warning') {
        const text = msg.text()
        if (
          text.includes('WebRTC') ||
          text.includes('bridge') ||
          text.includes('RTCPeerConnection')
        ) {
          consoleErrors.push(text)
        }
      }
    }
    page.on('console', errorHandler)
    // Also capture worker-side errors.
    page.on('worker', (w: Worker) => {
      w.on('console', errorHandler)
    })

    await page.goto('/')
    await waitForConsole(page, 'WebDocument: WebRTC bridge opened for')

    // Allow time for transport initialization after bridge setup.
    await page.waitForTimeout(5_000)

    expect(
      pageErrors.filter(
        (e) => e.includes('WebRTC') || e.includes('bridge'),
      ),
    ).toEqual([])
    expect(consoleErrors).toEqual([])
  })
})

test.describe('cross-browser bridge bootstrap', () => {
  test('Chromium and Firefox both bootstrap bridge', async () => {
    const port = Number.parseInt(process.env.E2E_PORT ?? '', 10) || 8080
    const url = `http://localhost:${port}`

    // Launch two separate browser instances.
    const chrBrowser = await chromium.launch()
    const ffBrowser = await firefox.launch()

    try {
      const chrContext = await chrBrowser.newContext()
      const ffContext = await ffBrowser.newContext()

      const chrPage = await chrContext.newPage()
      const ffPage = await ffContext.newPage()

      // Set up console listeners for bridge bootstrap on both pages.
      const chrBridgePromise = waitForConsole(
        chrPage,
        'WebDocument: WebRTC bridge opened for',
      )
      const ffBridgePromise = waitForConsole(
        ffPage,
        'WebDocument: WebRTC bridge opened for',
      )

      // Collect errors from both browsers.
      const chrErrors: string[] = []
      const ffErrors: string[] = []
      chrPage.on('pageerror', (err) => chrErrors.push(err.message))
      ffPage.on('pageerror', (err) => ffErrors.push(err.message))

      // Navigate both browsers to the app.
      await Promise.all([chrPage.goto(url), ffPage.goto(url)])

      // Both should bootstrap the bridge.
      const [chrMsg, ffMsg] = await Promise.all([
        chrBridgePromise,
        ffBridgePromise,
      ])

      expect(chrMsg).toContain('WebRTC bridge opened for')
      expect(ffMsg).toContain('WebRTC bridge opened for')

      // Allow time for transport init, check no bridge errors.
      await Promise.all([
        chrPage.waitForTimeout(5_000),
        ffPage.waitForTimeout(5_000),
      ])

      const bridgeErrors = (errs: string[]) =>
        errs.filter(
          (e) =>
            e.includes('WebRTC') ||
            e.includes('bridge') ||
            e.includes('RTCPeerConnection'),
        )
      expect(bridgeErrors(chrErrors)).toEqual([])
      expect(bridgeErrors(ffErrors)).toEqual([])

      await chrContext.close()
      await ffContext.close()
    } finally {
      await chrBrowser.close()
      await ffBrowser.close()
    }
  })
})
