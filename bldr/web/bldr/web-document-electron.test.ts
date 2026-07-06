import { afterEach, describe, expect, it, vi } from 'vitest'

import type {
  WebDocument as WebDocumentInstance,
  WebDocumentOptions,
} from './web-document.js'
import type { WebRuntimeClientInit } from '../runtime/runtime.pb.js'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { resetStartupMarksForTest } from './startup-marks.js'

type TestMessage = {
  data: unknown
  transfer?: readonly unknown[]
}

type TestMessageChannelRecord = {
  port1: TestMessagePort
  port2: TestMessagePort
}

type WebDocumentConstructor = new (
  opts?: WebDocumentOptions,
) => WebDocumentInstance

type OpenWebRuntimeClient = (
  this: WebDocumentInstance,
  init: WebRuntimeClientInit,
) => Promise<MessagePort>

class TestMessagePort {
  public onmessage: ((ev: MessageEvent) => void) | null = null
  public readonly messages: TestMessage[] = []
  public readonly postMessage = vi.fn(
    (data: unknown, transfer?: readonly unknown[]) => {
      this.messages.push({ data, transfer })
    },
  )
  public readonly start = vi.fn()
  public readonly close = vi.fn()
}

function installMessageChannel(): TestMessageChannelRecord[] {
  const channels: TestMessageChannelRecord[] = []

  class TestMessageChannel {
    public readonly port1 = new TestMessagePort()
    public readonly port2 = new TestMessagePort()

    public constructor() {
      channels.push({ port1: this.port1, port2: this.port2 })
    }
  }

  vi.stubGlobal('MessagePort', TestMessagePort)
  vi.stubGlobal('MessageChannel', TestMessageChannel)
  return channels
}

function installElectronBrowserGlobals() {
  const serviceWorkerRegister = vi.fn()
  const serviceWorkerAddEventListener = vi.fn()
  const locksRequest = vi.fn(() => new Promise<void>(() => {}))

  vi.stubGlobal('navigator', {
    userAgent: 'Mozilla/5.0 Electron/37.2.0',
    userAgentData: { platform: 'macOS' },
    storage: {},
    locks: { request: locksRequest },
    serviceWorker: {
      controller: null,
      register: serviceWorkerRegister,
      addEventListener: serviceWorkerAddEventListener,
    },
  })

  return {
    locksRequest,
    serviceWorkerAddEventListener,
    serviceWorkerRegister,
  }
}

async function loadElectronWebDocument() {
  vi.resetModules()

  const handleElectronWorkerPort = vi.fn()
  const workboxRegister = vi.fn()
  const workboxUpdate = vi.fn()
  const Workbox = vi.fn().mockImplementation((_swUrl: string) => ({
    controlling: Promise.resolve({} as ServiceWorker),
    register: workboxRegister,
    update: workboxUpdate,
  }))

  vi.doMock('../electron/electron.js', () => ({
    handleElectronWorkerPort,
    isDesktop: true,
    isElectron: true,
    isLinux: false,
    isMac: true,
    isSaucer: false,
    isWindows: false,
    openElectronDirectory: vi.fn(),
    quitDesktopRuntime: vi.fn(),
  }))
  vi.doMock('workbox-window', () => ({ Workbox }))

  // Test-only module-boundary exercise: WebDocument captures isElectron at
  // module evaluation, so this import must happen after the local Electron mock.
  const { WebDocument } = await import('./web-document.js')

  return {
    handleElectronWorkerPort,
    WebDocument: WebDocument as WebDocumentConstructor,
    Workbox,
    workboxRegister,
  }
}

function constructElectronDocument(WebDocument: WebDocumentConstructor) {
  return new WebDocument({
    serviceWorkerPath: '/sw-should-not-register.mjs',
    webDocumentId: 'electron-document-1',
    webRuntimeId: 'electron-runtime-1',
  })
}

describe('WebDocument Electron boot', () => {
  afterEach(() => {
    vi.doUnmock('../electron/electron.js')
    vi.doUnmock('workbox-window')
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.resetModules()
    resetStartupMarksForTest()
    globalThis.__swWebDocumentResumeReady = undefined
  })

  it('starts the Electron runtime branch without touching Workbox or ServiceWorker registration', async () => {
    const channels = installMessageChannel()
    const browser = installElectronBrowserGlobals()
    const { handleElectronWorkerPort, WebDocument, Workbox, workboxRegister } =
      await loadElectronWebDocument()

    const doc = constructElectronDocument(WebDocument)
    try {
      expect(Workbox).not.toHaveBeenCalled()
      expect(workboxRegister).not.toHaveBeenCalled()
      expect(browser.serviceWorkerRegister).not.toHaveBeenCalled()
      expect(browser.serviceWorkerAddEventListener).not.toHaveBeenCalled()
      expect(handleElectronWorkerPort).toHaveBeenCalledOnce()
      expect(handleElectronWorkerPort).toHaveBeenCalledWith(channels[0].port1)
      expect(channels[0].port2.start).toHaveBeenCalledOnce()
    } finally {
      doc.close()
    }
  })

  it('initializes the Electron runtime port before opening a WebRuntime client', async () => {
    const channels = installMessageChannel()
    installElectronBrowserGlobals()
    const { handleElectronWorkerPort, WebDocument } =
      await loadElectronWebDocument()
    const doc = constructElectronDocument(WebDocument)

    try {
      const openWebRuntimeClient = Reflect.get(
        doc,
        'openWebRuntimeClient',
      ) as OpenWebRuntimeClient
      const init: WebRuntimeClientInit = {
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
        clientUuid: 'electron-runtime-client-1',
        webRuntimeId: 'electron-runtime-1',
      }

      const clientPort = await openWebRuntimeClient.call(doc, init)

      expect(clientPort).toBe(channels[1].port1)
      expect(handleElectronWorkerPort).toHaveBeenCalledOnce()
      expect(channels[0].port2.messages).toHaveLength(1)
      expect(channels[0].port2.messages[0]).toMatchObject({
        data: {
          from: 'electron-document-1',
          connectWebRuntime: {
            init: expect.any(Uint8Array),
            port: channels[1].port2,
          },
        },
        transfer: [channels[1].port2],
      })
    } finally {
      doc.close()
    }
  })
})
