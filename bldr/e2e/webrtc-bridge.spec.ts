// webrtc-bridge.spec.ts - WebRTC bridge bootstrap verification.
//
// Runs against the full bldr dev server (bun run start:web:wasm).
// Verifies that:
//   1. The WebDocument creates a bridge endpoint for the worker.
//   2. No bridge-related errors during startup.
//
// The cross-browser test launches Firefox and Chromium simultaneously and
// verifies the WebDocument bridge owner without starting the app bundle.
//
// Note: verifying the worker-side shim installation or data-channel signaling
// through the bridge requires either worker console capture (unreliable in
// Playwright for WASM workers) or a signaling peer. The alpha e2e/wasm Go test
// harness is better suited for full-stack bridge integration tests.

import { test, expect, chromium, firefox } from '@playwright/test'
import type { Page, ConsoleMessage, Worker } from '@playwright/test'

async function waitForConsole(
  page: Page,
  pattern: string | RegExp,
  timeoutMs = 60_000,
  label = 'page',
): Promise<string> {
  const { promise, resolve, reject } = Promise.withResolvers<string>()
  const timer = setTimeout(
    () => reject(new Error(`timeout waiting for ${label} console: ${pattern}`)),
    timeoutMs,
  )
  const handler = (msg: ConsoleMessage) => {
    const text = msg.text()
    const matches =
      typeof pattern === 'string'
        ? text.includes(pattern)
        : pattern.test(text)
    if (matches) {
      clearTimeout(timer)
      page.removeListener('console', handler)
      resolve(text)
    }
  }
  page.on('console', handler)
  return promise
}

async function bridgeHarnessDocument(
  origin: string,
  browserIceServersEndpoint?: string,
) {
  const indexResponse = await fetch(origin)
  if (!indexResponse.ok) {
    throw new Error(`failed to fetch browser index: ${indexResponse.status}`)
  }

  const indexHtml = await indexResponse.text()
  const importMap = indexHtml.match(
    /<script type="importmap">[\s\S]*?<\/script>/,
  )?.[0]
  if (!importMap) {
    throw new Error('browser index missing importmap')
  }

  const html = `<!doctype html>
<html>
  <head><meta charset="utf-8"></head>
  <body>
    ${importMap}
    <script type="module">
      import { WebDocument } from '@aptre/bldr'

      const workerId = 'bridge-harness-worker'
      const requestId = 'bridge-harness-request'
      const { port1: workerPort, port2: ackPort } = new MessageChannel()
      const NativeRTCPeerConnection = globalThis.RTCPeerConnection
      globalThis.RTCPeerConnection = class extends NativeRTCPeerConnection {
        constructor(config) {
          super(config)
          globalThis.bridgeHarnessPeerConfiguration = this.getConfiguration()
        }
      }
      const webDocument = Object.create(WebDocument.prototype)
      webDocument.webDocumentUuid = 'bridge-harness-document'
      webDocument.webWorkers = { [workerId]: { port: workerPort } }
      webDocument.webrtcBridgeEndpoints = new Map()
      webDocument.browserIceServers = []
      webDocument.browserIceServersEndpoint = ${JSON.stringify(browserIceServersEndpoint)}

      ackPort.onmessage = (ev) => {
        const bridgePort = ev.data?.bridgePort
        if (!bridgePort) {
          console.error('WebRTC bridge harness missing bridge port')
          return
        }

        bridgePort.onmessage = (bridgeEvent) => {
          if (bridgeEvent.data?.type === 'createPC') {
            if (bridgeEvent.data.error) {
              console.error('WebRTC bridge harness createPC failed', bridgeEvent.data.error)
              return
            }
            console.log('WebRTC bridge harness createPC response', bridgeEvent.data.pcId)
            console.log(
              'WebRTC bridge harness peer configuration',
              JSON.stringify(globalThis.bridgeHarnessPeerConfiguration),
            )
            bridgePort.postMessage({
              type: 'close',
              cmdId: 2,
              pcId: bridgeEvent.data.pcId,
            })
            return
          }
          if (bridgeEvent.data?.type === 'close') {
            bridgePort.close()
            workerPort.close()
            ackPort.close()
            webDocument.webrtcBridgeEndpoints.get(workerId)?.close()
            console.log('WebRTC bridge harness closed')
          }
        }
        bridgePort.start()
        bridgePort.postMessage({ type: 'createPC', cmdId: 1, config: {} })
      }
      ackPort.start()
      workerPort.start()
      webDocument.handleConnectWebRtcBridge(workerId, requestId)
    </script>
  </body>
</html>`
  return {
    url: `${origin}/__bldr-webrtc-bridge-harness.html`,
    html,
  }
}

// TIER: pr
test.describe('WebRTC bridge bootstrap', () => {
  test('WebDocument opens bridge for worker', async ({ page }) => {
    const bridgePromise = waitForConsole(
      page,
      'WebDocument: WebRTC bridge opened for',
    )

    await page.goto('/#/')

    const msg = await bridgePromise
    expect(msg).toContain('WebRTC bridge opened for')
  })

  test('loads trusted same-origin ICE credentials before creating a real peer connection', async ({
    browser,
  }) => {
    const port = Number.parseInt(process.env.E2E_PORT ?? '', 10) || 5593
    const origin = `http://localhost:${port}`
    const credentialPath = '/api/test-turn-credentials'
    const harness = await bridgeHarnessDocument(origin, credentialPath)
    const context = await browser.newContext()
    let credentialRequests = 0
    await context.route(`${origin}${credentialPath}`, (route) => {
      credentialRequests++
      return route.fulfill({
        contentType: 'application/json',
        headers: { 'cache-control': 'private, no-store' },
        body: JSON.stringify({
          iceServers: [
            {
              urls: ['turns:turn.cloudflare.com:443?transport=tcp'],
              username: 'short-lived-user',
              credential: 'short-lived-credential',
            },
          ],
          expiresAt: Date.now() + 6 * 60 * 60 * 1000,
        }),
      })
    })
    await context.route(harness.url, (route) =>
      route.fulfill({ contentType: 'text/html', body: harness.html }),
    )
    const page = await context.newPage()
    const configurationPromise = waitForConsole(
      page,
      'WebRTC bridge harness peer configuration',
    )

    await page.goto(harness.url, { waitUntil: 'domcontentloaded' })
    const configurationMessage = await configurationPromise

    expect(credentialRequests).toBe(1)
    expect(configurationMessage).toContain(
      'turns:turn.cloudflare.com:443?transport=tcp',
    )
    expect(configurationMessage).not.toContain('turn:worker.example')
    await context.close()
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

    await page.goto('/#/')
    await waitForConsole(page, 'WebDocument: WebRTC bridge opened for')

    // Allow time for transport initialization after bridge setup.
    await page.waitForTimeout(5_000)

    expect(
      pageErrors.filter((e) => e.includes('WebRTC') || e.includes('bridge')),
    ).toEqual([])
    expect(consoleErrors).toEqual([])
  })
})

test.describe('cross-browser bridge bootstrap', () => {
  test('Chromium and Firefox both bootstrap bridge', async () => {
    const port = Number.parseInt(process.env.E2E_PORT ?? '', 10) || 5593
    const origin = `http://localhost:${port}`
    const harness = await bridgeHarnessDocument(origin)
    // Launch two separate browser instances.
    const [chrBrowser, ffBrowser] = await Promise.all([
      chromium.launch(),
      firefox.launch(),
    ])

    try {
      const [chrContext, ffContext] = await Promise.all([
        chrBrowser.newContext(),
        ffBrowser.newContext(),
      ])
      await Promise.all([
        chrContext.route(harness.url, (route) =>
          route.fulfill({ contentType: 'text/html', body: harness.html }),
        ),
        ffContext.route(harness.url, (route) =>
          route.fulfill({ contentType: 'text/html', body: harness.html }),
        ),
      ])

      const [chrPage, ffPage] = await Promise.all([
        chrContext.newPage(),
        ffContext.newPage(),
      ])

      const chrBridgePromise = waitForConsole(
        chrPage,
        'WebDocument: WebRTC bridge opened for',
        undefined,
        'chromium',
      )
      const ffBridgePromise = waitForConsole(
        ffPage,
        'WebDocument: WebRTC bridge opened for',
        undefined,
        'firefox',
      )
      const chrCreatePCPromise = waitForConsole(
        chrPage,
        'WebRTC bridge harness createPC response',
        undefined,
        'chromium createPC',
      )
      const ffCreatePCPromise = waitForConsole(
        ffPage,
        'WebRTC bridge harness createPC response',
        undefined,
        'firefox createPC',
      )

      const chrErrors: string[] = []
      const ffErrors: string[] = []
      chrPage.on('pageerror', (err) => chrErrors.push(err.message))
      ffPage.on('pageerror', (err) => ffErrors.push(err.message))

      await Promise.all([
        chrPage.goto(harness.url, { waitUntil: 'domcontentloaded' }),
        ffPage.goto(harness.url, { waitUntil: 'domcontentloaded' }),
      ])

      const [chrMsg, ffMsg, chrCreatePCMsg, ffCreatePCMsg] = await Promise.all([
        chrBridgePromise,
        ffBridgePromise,
        chrCreatePCPromise,
        ffCreatePCPromise,
      ])

      expect(chrMsg).toContain('WebRTC bridge opened for')
      expect(ffMsg).toContain('WebRTC bridge opened for')
      expect(chrCreatePCMsg).toContain('createPC response')
      expect(ffCreatePCMsg).toContain('createPC response')

      expect(
        chrErrors.filter(
          (e) =>
            e.includes('WebRTC') ||
            e.includes('bridge') ||
            e.includes('RTCPeerConnection'),
        ),
      ).toEqual([])
      expect(
        ffErrors.filter(
          (e) =>
            e.includes('WebRTC') ||
            e.includes('bridge') ||
            e.includes('RTCPeerConnection'),
        ),
      ).toEqual([])

      await Promise.all([chrContext.close(), ffContext.close()])
    } finally {
      await Promise.all([chrBrowser.close(), ffBrowser.close()])
    }
  })
})
