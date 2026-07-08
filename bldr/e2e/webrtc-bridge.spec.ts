// webrtc-bridge.spec.ts - WebRTC bridge browser verification.
//
// Runs against the full bldr dev server (bun run start:web:wasm). The routed
// harness keeps the app import map, instantiates the real WebDocument bridge
// owner, and drives ProxyRTCPeerConnection over the returned bridge port.

import { test, expect, chromium, firefox } from '@playwright/test'
import type {
  Browser,
  BrowserContext,
  ConsoleMessage,
  Page,
  Worker,
} from '@playwright/test'

const chromiumLoopbackArgs = [
  '--allow-loopback-in-peer-connection',
  '--disable-features=WebRtcHideLocalIpsWithMdns',
]

const bridgeActionTimeoutMs = 30_000
const chromiumPayloadAToB = Array.from({ length: 4099 }, (_, index) => {
  return (17 + index * 37 + (index >> 3)) & 0xff
})
const chromiumPayloadBToA = Array.from({ length: 4097 }, (_, index) => {
  return (191 + index * 29 + (index >> 2)) & 0xff
})
const firefoxPayloadChromiumToFirefox = Array.from(
  { length: 4093 },
  (_, index) => {
    return (43 + index * 31 + (index >> 1)) & 0xff
  },
)
const firefoxPayloadFirefoxToChromium = Array.from(
  { length: 4091 },
  (_, index) => {
    return (211 + index * 17 + (index >> 4)) & 0xff
  },
)

type BridgeHarness = {
  url: string
  html: string
}

type ProofSnapshot = {
  peerId: string
  connectionState: string
  iceConnectionState: string
  iceGatheringState: string
  signalingState: string
  dataChannelState: string
  endpointCount: number
  bridgeClosed: boolean
  events: string[]
  messages: number[][]
}

type ProofPage = {
  context: BrowserContext
  page: Page
}

async function waitForConsole(
  page: Page,
  pattern: string | RegExp,
  timeoutMs = 60_000,
  label = 'page',
): Promise<string> {
  const { promise, resolve, reject } = Promise.withResolvers<string>()
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
  const timer = setTimeout(() => {
    page.removeListener('console', handler)
    reject(new Error(`timeout waiting for ${label} console: ${pattern}`))
  }, timeoutMs)
  page.on('console', handler)
  return promise
}

async function bridgeHarnessDocument(origin: string): Promise<BridgeHarness> {
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
      import {
        ProxyRTCPeerConnection,
        WebDocument,
        setBridgePort,
      } from '@aptre/bldr'

      const defaultTimeoutMs = ${bridgeActionTimeoutMs}

      function byteArray(value) {
        if (!Array.isArray(value)) {
          throw new Error('expected byte array')
        }
        return value.map((entry) => {
          if (!Number.isInteger(entry) || entry < 0 || entry > 255) {
            throw new Error('expected bytes in the 0..255 range')
          }
          return entry
        })
      }

      function bytesEqual(left, right) {
        if (left.length !== right.length) return false
        for (let index = 0; index < left.length; index += 1) {
          if (left[index] !== right[index]) return false
        }
        return true
      }

      function normalizeDataChannelPayload(data) {
        if (data instanceof ArrayBuffer) {
          return Array.from(new Uint8Array(data))
        }
        if (ArrayBuffer.isView(data)) {
          return Array.from(
            new Uint8Array(data.buffer, data.byteOffset, data.byteLength),
          )
        }
        throw new Error('expected binary data channel payload')
      }

      function descriptionInit(value) {
        if (!value || typeof value !== 'object') {
          throw new Error('expected RTCSessionDescriptionInit object')
        }
        if (typeof value.type !== 'string' || typeof value.sdp !== 'string') {
          throw new Error('description missing type or sdp')
        }
        return { type: value.type, sdp: value.sdp }
      }

      function installWebDocumentCloseStubs(webDocument, workerId, workerPort) {
        webDocument.sabPairBroker = { closeAll() {} }
        webDocument.opfsWorkers = new Map()
        webDocument.runtimeOpfsBrokerPort = undefined
        webDocument.crossTabManager = { close() {} }
        webDocument.client = { setOpenStreamFn() {} }
        webDocument.webRuntimeClient = { close() {} }
        webDocument.webViews = {}
        webDocument.worker = undefined
        webDocument.runtimeWorker = undefined
        webDocument.serviceWorkerPort = undefined
        webDocument.serviceWorker = undefined
        webDocument.releaseShutdownCallback = undefined
        webDocument.releaseVisibilityCallback = undefined
        webDocument.closedCallback = undefined
        webDocument.pushChangeEvent = () => {}
        webDocument.emit = () => {}
        webDocument.releasePluginSingletonLock = () => {}
        webDocument.dedicatedRuntimeHost = undefined
        webDocument.webDocumentLivenessAbort = undefined
        webDocument.webDocumentLivenessLockState = 'idle'
        webDocument.webWorkers = {
          [workerId]: {
            port: workerPort,
            close() {
              workerPort.close()
            },
          },
        }
      }

      function snapshot(state) {
        return {
          peerId: state.peerId,
          connectionState: state.pc.connectionState,
          iceConnectionState: state.pc.iceConnectionState,
          iceGatheringState: state.pc.iceGatheringState,
          signalingState: state.pc.signalingState,
          dataChannelState: state.dc.readyState,
          endpointCount: state.webDocument.webrtcBridgeEndpoints.size,
          bridgeClosed: state.bridgeClosed,
          events: [...state.events],
          messages: state.messages.map((bytes) => [...bytes]),
        }
      }

      function record(state, event) {
        state.events.push(
          event +
            ':pc=' +
            state.pc.connectionState +
            ':ice=' +
            state.pc.iceConnectionState +
            ':dc=' +
            state.dc.readyState,
        )
        for (const notify of state.waiters) notify()
      }

      function waitForState(state, label, predicate, timeoutMs = defaultTimeoutMs) {
        const current = predicate()
        if (current) return Promise.resolve(current)

        const { promise, resolve, reject } = Promise.withResolvers()
        const notify = () => {
          try {
            const value = predicate()
            if (!value) return
            clearTimeout(timer)
            state.waiters.delete(notify)
            resolve(value)
          } catch (err) {
            clearTimeout(timer)
            state.waiters.delete(notify)
            reject(err)
          }
        }
        const timer = setTimeout(() => {
          state.waiters.delete(notify)
          reject(new Error(label + ' timed out after ' + timeoutMs + 'ms'))
        }, timeoutMs)
        state.waiters.add(notify)
        return promise
      }

      function currentState() {
        const state = window.__webrtcBridgeProofState
        if (!state) {
          throw new Error('WebRTC bridge proof is not initialized')
        }
        return state
      }

      async function init(peerId) {
        window.__webrtcBridgeProofState?.webDocument.close()

        const workerId = 'bridge-proof-worker-' + peerId
        const requestId = 'bridge-proof-request-' + peerId
        const { port1: workerPort, port2: ackPort } = new MessageChannel()
        const { promise: bridgePortPromise, resolve, reject } =
          Promise.withResolvers()
        const webDocument = Object.create(WebDocument.prototype)
        webDocument.webDocumentUuid = 'bridge-proof-document-' + peerId
        webDocument.webrtcBridgeEndpoints = new Map()
        installWebDocumentCloseStubs(webDocument, workerId, workerPort)

        ackPort.onmessage = (ev) => {
          const bridgePort = ev.data?.bridgePort
          if (!bridgePort) {
            reject(new Error('WebRTC bridge harness missing bridge port'))
            return
          }
          resolve(bridgePort)
        }
        ackPort.onmessageerror = () =>
          reject(new Error('WebRTC bridge harness ack port messageerror'))
        ackPort.start()
        workerPort.start()
        webDocument.handleConnectWebRtcBridge(workerId, requestId)

        const bridgePort = await bridgePortPromise
        setBridgePort(bridgePort)
        const pc = new ProxyRTCPeerConnection({
          bundlePolicy: 'max-bundle',
          iceServers: [],
        })
        const dc = pc.createDataChannel('spacewave-bridge-proof', {
          negotiated: true,
          id: 0,
          ordered: true,
          protocol: 'spacewave-proof',
        })
        dc.binaryType = 'arraybuffer'

        const state = {
          peerId,
          webDocument,
          workerPort,
          ackPort,
          pc,
          dc,
          messages: [],
          events: [],
          waiters: new Set(),
          bridgeClosed: false,
        }
        bridgePort.addEventListener('message', (event) => {
          if (event.data?.type === 'event:bridgeclose') {
            state.bridgeClosed = true
            record(state, 'bridge:close')
          }
        })
        pc.onconnectionstatechange = () => record(state, 'pc:connectionstatechange')
        pc.oniceconnectionstatechange = () =>
          record(state, 'pc:iceconnectionstatechange')
        pc.onicegatheringstatechange = () =>
          record(state, 'pc:icegatheringstatechange')
        pc.onsignalingstatechange = () => record(state, 'pc:signalingstatechange')
        dc.onopen = () => record(state, 'dc:open')
        dc.onclosing = () => record(state, 'dc:closing')
        dc.onclose = () => record(state, 'dc:close')
        dc.onerror = () => record(state, 'dc:error')
        dc.onmessage = (event) => {
          state.messages.push(normalizeDataChannelPayload(event.data))
          record(state, 'dc:message')
        }

        window.__webrtcBridgeProofState = state
        record(state, 'init')
        return snapshot(state)
      }

      async function waitIceGatheringComplete(state) {
        await waitForState(state, 'ICE gathering complete', () =>
          state.pc.iceGatheringState === 'complete' ? true : undefined,
        )
      }

      async function createOffer() {
        const state = currentState()
        const offer = await state.pc.createOffer()
        await state.pc.setLocalDescription(offer)
        await waitIceGatheringComplete(state)
        return {
          type: state.pc.localDescription.type,
          sdp: state.pc.localDescription.sdp,
        }
      }

      async function acceptOffer(description) {
        const state = currentState()
        await state.pc.setRemoteDescription(descriptionInit(description))
        const answer = await state.pc.createAnswer()
        await state.pc.setLocalDescription(answer)
        await waitIceGatheringComplete(state)
        return {
          type: state.pc.localDescription.type,
          sdp: state.pc.localDescription.sdp,
        }
      }

      async function acceptAnswer(description) {
        const state = currentState()
        await state.pc.setRemoteDescription(descriptionInit(description))
        return snapshot(state)
      }

      async function waitOpen() {
        const state = currentState()
        return waitForState(state, 'data channel open', () =>
          state.pc.connectionState === 'connected' && state.dc.readyState === 'open'
            ? snapshot(state)
            : undefined,
        )
      }

      async function sendBytes(bytes) {
        const state = currentState()
        const normalized = byteArray(bytes)
        if (state.dc.readyState !== 'open') {
          throw new Error('data channel is ' + state.dc.readyState + ', not open')
        }
        state.dc.send(Uint8Array.from(normalized))
        record(state, 'dc:send')
        return snapshot(state)
      }

      async function waitBytes(bytes) {
        const state = currentState()
        const expected = byteArray(bytes)
        return waitForState(state, 'bridged byte payload', () => {
          const found = state.messages.find((message) =>
            bytesEqual(message, expected),
          )
          return found ? { ...snapshot(state), receivedBytes: [...found] } : undefined
        })
      }

      async function closeLocalDocument() {
        const state = currentState()
        state.webDocument.close()
        return waitForState(state, 'local bridge close', () =>
          state.bridgeClosed &&
          state.pc.connectionState === 'closed' &&
          state.pc.signalingState === 'closed' &&
          state.webDocument.webrtcBridgeEndpoints.size === 0
            ? snapshot(state)
            : undefined,
        )
      }

      async function waitRemoteTeardown() {
        const state = currentState()
        return waitForState(state, 'remote teardown', () => {
          if (
            state.dc.readyState === 'closed' ||
            state.pc.connectionState === 'closed' ||
            state.pc.connectionState === 'failed' ||
            state.pc.connectionState === 'disconnected'
          ) {
            return snapshot(state)
          }
          return undefined
        })
      }

      window.__webrtcBridgeProof = {
        init,
        createOffer,
        acceptOffer,
        acceptAnswer,
        waitOpen,
        sendBytes,
        waitBytes,
        closeLocalDocument,
        waitRemoteTeardown,
        snapshot() {
          return snapshot(currentState())
        },
      }
      console.log('WebRTC bridge proof harness ready')
    </script>
  </body>
</html>`
  return {
    url: `${origin}/__bldr-webrtc-bridge-harness.html`,
    html,
  }
}

async function newProofPage(
  browser: Browser,
  harness: BridgeHarness,
  peerId: string,
): Promise<ProofPage> {
  const context = await browser.newContext()
  await context.route(harness.url, (route) =>
    route.fulfill({ contentType: 'text/html', body: harness.html }),
  )
  const page = await context.newPage()
  const readyPromise = waitForConsole(page, 'WebRTC bridge proof harness ready')
  await page.goto(harness.url, { waitUntil: 'domcontentloaded' })
  await readyPromise
  await proofAction<ProofSnapshot>(page, 'init', peerId)
  return { context, page }
}

async function proofAction<T>(
  page: Page,
  action: string,
  arg?: unknown,
): Promise<T> {
  return (await page.evaluate(
    async ({ action: actionName, arg: actionArg }) => {
      const proof = (
        window as typeof window & {
          __webrtcBridgeProof?: Record<
            string,
            (arg?: unknown) => Promise<unknown>
          >
        }
      ).__webrtcBridgeProof
      if (!proof) {
        throw new Error('WebRTC bridge proof harness is not ready')
      }
      const fn = proof[actionName]
      if (!fn) {
        throw new Error(`unknown WebRTC bridge proof action ${actionName}`)
      }
      return fn(actionArg)
    },
    { action, arg },
  )) as T
}

async function connectProofPages(left: Page, right: Page): Promise<void> {
  const offer = await proofAction<RTCSessionDescriptionInit>(
    left,
    'createOffer',
  )
  const answer = await proofAction<RTCSessionDescriptionInit>(
    right,
    'acceptOffer',
    offer,
  )
  await proofAction<ProofSnapshot>(left, 'acceptAnswer', answer)
  await Promise.all([
    proofAction<ProofSnapshot>(left, 'waitOpen'),
    proofAction<ProofSnapshot>(right, 'waitOpen'),
  ])
}

async function expectBidirectionalBytes(
  left: Page,
  right: Page,
  leftToRight: number[],
  rightToLeft: number[],
): Promise<void> {
  await Promise.all([
    proofAction<ProofSnapshot>(left, 'sendBytes', leftToRight),
    proofAction<ProofSnapshot>(right, 'sendBytes', rightToLeft),
  ])

  const [rightReceived, leftReceived] = await Promise.all([
    proofAction<ProofSnapshot & { receivedBytes: number[] }>(
      right,
      'waitBytes',
      leftToRight,
    ),
    proofAction<ProofSnapshot & { receivedBytes: number[] }>(
      left,
      'waitBytes',
      rightToLeft,
    ),
  ])
  expect(rightReceived.receivedBytes).toEqual(leftToRight)
  expect(leftReceived.receivedBytes).toEqual(rightToLeft)
}

async function launchFirefoxOrSkip(): Promise<Browser> {
  try {
    return await firefox.launch()
  } catch (err) {
    test.skip(
      true,
      `firefox launch unavailable for WebRTC bridge proof: ${err instanceof Error ? err.message : String(err)}`,
    )
    throw err
  }
}

function bridgeHarnessOrigin(): string {
  const port = Number.parseInt(process.env.E2E_PORT ?? '', 10) || 5593
  return `http://localhost:${port}`
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
    page.on('worker', (w: Worker) => {
      w.on('console', errorHandler)
    })

    const bridgePromise = waitForConsole(
      page,
      'WebDocument: WebRTC bridge opened for',
    )
    await page.goto('/#/')
    await bridgePromise

    expect(
      pageErrors.filter((e) => e.includes('WebRTC') || e.includes('bridge')),
    ).toEqual([])
    expect(consoleErrors).toEqual([])
  })
})

test.describe('WebRTC bridge endpoint proofs', () => {
  test('Chromium to Chromium preserves bidirectional bridged bytes', async () => {
    const harness = await bridgeHarnessDocument(bridgeHarnessOrigin())
    const browser = await chromium.launch({ args: chromiumLoopbackArgs })
    try {
      const [left, right] = await Promise.all([
        newProofPage(browser, harness, 'chromium-a'),
        newProofPage(browser, harness, 'chromium-b'),
      ])
      try {
        await connectProofPages(left.page, right.page)
        await expectBidirectionalBytes(
          left.page,
          right.page,
          chromiumPayloadAToB,
          chromiumPayloadBToA,
        )
      } finally {
        await Promise.all([left.context.close(), right.context.close()])
      }
    } finally {
      await browser.close()
    }
  })

  test('Chromium survivor observes bridge teardown when owner context closes', async () => {
    const harness = await bridgeHarnessDocument(bridgeHarnessOrigin())
    const browser = await chromium.launch({ args: chromiumLoopbackArgs })
    try {
      const [owner, survivor] = await Promise.all([
        newProofPage(browser, harness, 'owner'),
        newProofPage(browser, harness, 'survivor'),
      ])
      try {
        await connectProofPages(owner.page, survivor.page)
        const ownerClosed = await proofAction<ProofSnapshot>(
          owner.page,
          'closeLocalDocument',
        )
        expect(ownerClosed.connectionState).toBe('closed')
        expect(ownerClosed.signalingState).toBe('closed')
        expect(ownerClosed.endpointCount).toBe(0)

        await owner.context.close()
        const survivorTeardown = await proofAction<ProofSnapshot>(
          survivor.page,
          'waitRemoteTeardown',
        )
        expect(
          ['closed', 'failed', 'disconnected', 'connected'].includes(
            survivorTeardown.connectionState,
          ),
        ).toBe(true)
        expect(
          survivorTeardown.dataChannelState === 'closed' ||
            ['closed', 'failed', 'disconnected'].includes(
              survivorTeardown.connectionState,
            ),
        ).toBe(true)
      } finally {
        await survivor.context.close()
      }
    } finally {
      await browser.close()
    }
  })

  test('Chromium to Firefox preserves bridged bytes when Firefox launches', async () => {
    const harness = await bridgeHarnessDocument(bridgeHarnessOrigin())
    const chromiumBrowser = await chromium.launch({
      args: chromiumLoopbackArgs,
    })
    let firefoxBrowser: Browser | undefined
    try {
      firefoxBrowser = await launchFirefoxOrSkip()
      const [chr, ff] = await Promise.all([
        newProofPage(chromiumBrowser, harness, 'chromium'),
        newProofPage(firefoxBrowser, harness, 'firefox'),
      ])
      try {
        await connectProofPages(chr.page, ff.page)
        await expectBidirectionalBytes(
          chr.page,
          ff.page,
          firefoxPayloadChromiumToFirefox,
          firefoxPayloadFirefoxToChromium,
        )
      } finally {
        await Promise.all([chr.context.close(), ff.context.close()])
      }
    } finally {
      await Promise.all([
        chromiumBrowser.close(),
        firefoxBrowser ? firefoxBrowser.close() : Promise.resolve(),
      ])
    }
  })
})
