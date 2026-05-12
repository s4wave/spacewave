import { afterEach, describe, expect, it, vi } from 'vitest'

import type { WebDocumentToClient } from '../runtime/runtime.js'
import { WebWorkerType } from '../document/document.pb.js'
import { WebDocument, registerUpdatedServiceWorker } from './web-document.js'
import { SabPairBroker } from './sab-pair-broker.js'
import { resetStartupMarksForTest, startupMarkPrefix } from './startup-marks.js'

type TestWebDocument = {
  closed?: true | Error
  hidden: boolean
  resumeReady: boolean
  resumeReadyPending: boolean
  resumeReadySequence: number
  runtimeConnected: boolean
  serviceWorkerPort?: MessagePort
  webWorkers: Record<string, Record<string, unknown> & { port: MessagePort }>
  sabPairBroker: SabPairBroker
  webrtcBridgeEndpoints: Map<string, unknown>
  firstWorkerReadyMarked: boolean
  notifyWebWorkerUpdated: ReturnType<typeof vi.fn>
  webStatusStream: {
    pushChangeEvent: ReturnType<typeof vi.fn>
    snapshot: Promise<null>
  }
  scheduleResumeReadySeed(): void
  onVisibilityChange(hidden: boolean): void
  onWebWorkerMessage(workerID: string, event: MessageEvent): void
  onWebDocumentClientMessage(event: MessageEvent): void
  openWebDocumentHostStream(): Promise<unknown>
  webRuntimeClient: { openStream: () => Promise<unknown> }
}

function buildTestWebDocument(hidden = false): TestWebDocument {
  const doc = Object.create(WebDocument.prototype) as TestWebDocument
  Object.assign(doc, {
    webDocumentUuid: 'document-1',
    webRuntimeId: 'runtime-1',
    closed: undefined,
    hidden,
    resumeReady: false,
    resumeReadyPending: false,
    resumeReadySequence: 0,
    runtimeConnected: true,
    serviceWorkerPort: undefined,
    webWorkers: {},
    sabPairBroker: new SabPairBroker(),
    webrtcBridgeEndpoints: new Map(),
    firstWorkerReadyMarked: false,
    notifyWebWorkerUpdated: vi.fn(),
    webStatusStream: {
      pushChangeEvent: vi.fn(),
      snapshot: Promise.resolve(null),
    },
    eventHandlers: {},
  })
  return doc
}

describe('registerUpdatedServiceWorker', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetStartupMarksForTest()
    globalThis.__swWebDocumentResumeReady = undefined
  })

  it('registers the manifest service worker URL when it differs', async () => {
    const register = vi.fn().mockResolvedValue({})
    const registration = {
      scope: 'https://example.test/',
    } as ServiceWorkerRegistration

    await registerUpdatedServiceWorker(
      '/sw-a.mjs',
      registration,
      register,
      '/sw-b.mjs',
    )

    expect(register).toHaveBeenCalledWith(
      new URL('/sw-b.mjs', location.href).toString(),
      {
        scope: registration.scope,
      },
    )
  })

  it('does not re-register when the URLs match', async () => {
    const register = vi.fn().mockResolvedValue({})

    const result = await registerUpdatedServiceWorker(
      '/sw-a.mjs',
      undefined,
      register,
      '/sw-a.mjs',
    )

    expect(result).toBeNull()
    expect(register).not.toHaveBeenCalled()
  })
})

describe('WebDocument resume-ready state', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetStartupMarksForTest()
    globalThis.__swWebDocumentResumeReady = undefined
  })

  it('seeds resume-ready only after two visible foreground frames', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((cb: FrameRequestCallback) => {
        frames.push(cb)
        return frames.length
      }),
    )
    const mark = vi.spyOn(performance, 'mark').mockImplementation(() => {
      return {} as PerformanceMark
    })
    const doc = buildTestWebDocument()

    doc.scheduleResumeReadySeed()

    expect(globalThis.__swWebDocumentResumeReady).toBeUndefined()
    expect(frames).toHaveLength(1)

    frames.shift()?.(1)
    expect(globalThis.__swWebDocumentResumeReady).toBeUndefined()
    expect(frames).toHaveLength(1)

    frames.shift()?.(2)

    expect(globalThis.__swWebDocumentResumeReady).toMatchObject({
      ready: true,
      documentId: 'document-1',
      runtimeId: 'runtime-1',
      hidden: false,
      sequence: 1,
    })
    expect(mark).toHaveBeenCalledWith(
      `${startupMarkPrefix}web-document.resume-ready`,
      expect.objectContaining({
        detail: expect.objectContaining({
          label: 'web-document.resume-ready',
          documentId: 'document-1',
          runtimeId: 'runtime-1',
          sequence: 1,
        }),
      }),
    )
  })

  it('notifies connected clients after resume-ready is seeded', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((cb: FrameRequestCallback) => {
        frames.push(cb)
        return frames.length
      }),
    )
    vi.spyOn(performance, 'mark').mockImplementation(() => {
      return {} as PerformanceMark
    })
    const serviceWorkerPostMessage = vi.fn()
    const workerPostMessage = vi.fn()
    const doc = buildTestWebDocument()
    doc.serviceWorkerPort = {
      postMessage: serviceWorkerPostMessage,
    } as unknown as MessagePort
    doc.webWorkers = {
      'worker-1': {
        port: {
          postMessage: workerPostMessage,
        } as unknown as MessagePort,
      },
    }

    doc.scheduleResumeReadySeed()
    frames.shift()?.(1)
    frames.shift()?.(2)

    expect(serviceWorkerPostMessage).toHaveBeenCalledWith({
      from: 'document-1',
      resumeReady: true,
    })
    expect(workerPostMessage).toHaveBeenCalledWith({
      from: 'document-1',
      resumeReady: true,
    })
  })

  it('does not seed resume-ready while hidden', () => {
    const raf = vi.fn()
    vi.stubGlobal('requestAnimationFrame', raf)
    const doc = buildTestWebDocument(true)

    doc.scheduleResumeReadySeed()

    expect(raf).not.toHaveBeenCalled()
    expect(globalThis.__swWebDocumentResumeReady).toBeUndefined()
  })

  it('clears and reseeds resume-ready across foreground resumes', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((cb: FrameRequestCallback) => {
        frames.push(cb)
        return frames.length
      }),
    )
    const mark = vi.spyOn(performance, 'mark').mockImplementation(() => {
      return {} as PerformanceMark
    })
    const workerPostMessage = vi.fn()
    const doc = buildTestWebDocument()
    doc.webWorkers = {
      'worker-1': {
        port: {
          postMessage: workerPostMessage,
        } as unknown as MessagePort,
      },
    }

    doc.scheduleResumeReadySeed()
    frames.shift()?.(1)
    frames.shift()?.(2)

    expect(globalThis.__swWebDocumentResumeReady?.sequence).toBe(1)
    workerPostMessage.mockClear()

    doc.onVisibilityChange(true)

    expect(doc.resumeReady).toBe(false)
    expect(globalThis.__swWebDocumentResumeReady).toBeUndefined()
    expect(workerPostMessage).toHaveBeenCalledWith({
      from: 'document-1',
      resumeReady: false,
    })
    expect(mark).toHaveBeenCalledWith(
      `${startupMarkPrefix}web-document.resume-not-ready`,
      expect.objectContaining({
        detail: expect.objectContaining({
          label: 'web-document.resume-not-ready',
          reason: 'hidden',
          sequence: expect.any(Number),
        }),
      }),
    )

    doc.onVisibilityChange(false)
    frames.shift()?.(3)
    frames.shift()?.(4)

    expect(globalThis.__swWebDocumentResumeReady).toMatchObject({
      ready: true,
      documentId: 'document-1',
      runtimeId: 'runtime-1',
      hidden: false,
      sequence: 2,
    })
  })

  it('keeps stream-open failures observable after resume-ready is seeded', async () => {
    const err = new Error('stream-open failed')
    const doc = buildTestWebDocument()
    doc.resumeReady = true
    globalThis.__swWebDocumentResumeReady = {
      ready: true,
      documentId: 'document-1',
      runtimeId: 'runtime-1',
      hidden: false,
      sequence: 1,
    }
    doc.webRuntimeClient = {
      openStream: vi.fn().mockRejectedValue(err),
    }

    await expect(doc.openWebDocumentHostStream()).rejects.toThrow(
      'stream-open failed',
    )
  })
})

describe('WebDocument SAB pair broker', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('allocates pair buffers only for explicit pair-open requests', () => {
    const doc = buildTestWebDocument()
    const sourcePostMessage = vi.fn()
    const targetPostMessage = vi.fn()
    doc.webWorkers = {
      'worker-a': {
        port: {
          postMessage: sourcePostMessage,
        } as unknown as MessagePort,
      },
      'worker-b': {
        port: {
          postMessage: targetPostMessage,
        } as unknown as MessagePort,
      },
    }

    doc.onWebDocumentClientMessage({
      data: {
        from: 'worker-a',
        openSabPair: {
          requestId: 'request-1',
          targetWorkerId: 'worker-b',
        },
      },
    } as MessageEvent)

    expect(targetPostMessage).toHaveBeenCalledOnce()
    expect(sourcePostMessage).toHaveBeenCalledOnce()

    const targetMessage = targetPostMessage.mock
      .calls[0][0] as WebDocumentToClient
    const sourceMessage = sourcePostMessage.mock
      .calls[0][0] as WebDocumentToClient
    const targetEndpoint = targetMessage.sabPairEndpoint
    const sourceEndpoint = sourceMessage.openSabPairAck?.endpoint

    expect(sourceMessage.openSabPairAck).toMatchObject({
      from: 'document-1',
      requestId: 'request-1',
    })
    expect(sourceEndpoint).toMatchObject({
      pairId: 'sab-pair-1',
      localWorkerId: 'worker-a',
      remoteWorkerId: 'worker-b',
      mtuBytes: 32 * 1024,
    })
    expect(targetEndpoint).toMatchObject({
      pairId: 'sab-pair-1',
      localWorkerId: 'worker-b',
      remoteWorkerId: 'worker-a',
      mtuBytes: 32 * 1024,
    })
    expect(sourceEndpoint?.txSab).toBe(targetEndpoint?.rxSab)
    expect(sourceEndpoint?.rxSab).toBe(targetEndpoint?.txSab)

    const brokerSnapshot = doc.sabPairBroker.snapshot()
    expect(brokerSnapshot).toEqual([
      {
        pairId: 'sab-pair-1',
        key: JSON.stringify(['worker-a', 'worker-b', 'sab-pair-1']),
        workerAId: 'worker-a',
        workerBId: 'worker-b',
        state: 'open',
      },
    ])
    expect(Object.keys(brokerSnapshot[0])).not.toContain('txSab')
    expect(Object.keys(brokerSnapshot[0])).not.toContain('rxSab')
  })

  it('does not allocate pair metadata for worker startup readiness', () => {
    const doc = buildTestWebDocument()
    const close = vi.fn()
    doc.webWorkers = {
      'worker-a': {
        ready: false,
        isShared: false,
        workerType: WebWorkerType.NATIVE,
        plugin: true,
        port: {
          close,
        } as unknown as MessagePort,
      },
    }

    doc.onWebWorkerMessage('worker-a', {
      data: {
        from: 'worker-a',
        ready: true,
      },
    } as MessageEvent)

    expect(doc.webWorkers['worker-a']).toMatchObject({ ready: true })
    expect(doc.sabPairBroker.snapshot()).toEqual([])
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-a',
      false,
      false,
      true,
    )
  })

  it('returns pair-open errors without retaining metadata', () => {
    const doc = buildTestWebDocument()
    const sourcePostMessage = vi.fn()
    doc.webWorkers = {
      'worker-a': {
        port: {
          postMessage: sourcePostMessage,
        } as unknown as MessagePort,
      },
    }

    doc.onWebDocumentClientMessage({
      data: {
        from: 'worker-a',
        openSabPair: {
          requestId: 'request-1',
          targetWorkerId: 'worker-missing',
        },
      },
    } as MessageEvent)

    expect(sourcePostMessage).toHaveBeenCalledWith({
      from: 'document-1',
      openSabPairAck: {
        from: 'document-1',
        requestId: 'request-1',
        error: 'target worker not found: worker-missing',
      },
    })
    expect(doc.sabPairBroker.snapshot()).toEqual([])
  })

  it('cleans pair metadata when endpoint delivery fails or closes', () => {
    const doc = buildTestWebDocument()
    const sourcePostMessage = vi.fn()
    const targetPostMessage = vi.fn()
    targetPostMessage.mockImplementationOnce(() => {
      throw new Error('target closed')
    })
    doc.webWorkers = {
      'worker-a': {
        port: {
          postMessage: sourcePostMessage,
        } as unknown as MessagePort,
      },
      'worker-b': {
        port: {
          postMessage: targetPostMessage,
        } as unknown as MessagePort,
      },
    }

    doc.onWebDocumentClientMessage({
      data: {
        from: 'worker-a',
        openSabPair: {
          requestId: 'request-1',
          targetWorkerId: 'worker-b',
        },
      },
    } as MessageEvent)

    expect(sourcePostMessage).toHaveBeenCalledWith({
      from: 'document-1',
      openSabPairAck: {
        from: 'document-1',
        requestId: 'request-1',
        error: 'target closed',
      },
    })
    expect(doc.sabPairBroker.snapshot()).toEqual([])

    doc.onWebDocumentClientMessage({
      data: {
        from: 'worker-a',
        openSabPair: {
          requestId: 'request-2',
          targetWorkerId: 'worker-b',
        },
      },
    } as MessageEvent)
    expect(doc.sabPairBroker.snapshot()).toHaveLength(1)

    doc.onWebDocumentClientMessage({
      data: {
        from: 'worker-b',
        closeSabPair: {
          pairId: 'sab-pair-2',
        },
      },
    } as MessageEvent)
    expect(doc.sabPairBroker.snapshot()).toEqual([])
    expect(sourcePostMessage).toHaveBeenLastCalledWith({
      from: 'document-1',
      sabPairClosed: {
        pairId: 'sab-pair-2',
        reason: 'stream closed',
      },
    })
  })

  it('notifies peer workers when closing pairs for a worker lifecycle event', () => {
    const doc = buildTestWebDocument()
    const sourcePostMessage = vi.fn()
    const targetPostMessage = vi.fn()
    doc.webWorkers = {
      'worker-a': {
        port: {
          postMessage: sourcePostMessage,
          close: vi.fn(),
        } as unknown as MessagePort,
      },
      'worker-b': {
        isShared: false,
        ready: true,
        workerType: WebWorkerType.NATIVE,
        port: {
          postMessage: targetPostMessage,
          close: vi.fn(),
        } as unknown as MessagePort,
      },
    }

    doc.onWebDocumentClientMessage({
      data: {
        from: 'worker-a',
        openSabPair: {
          requestId: 'request-1',
          targetWorkerId: 'worker-b',
        },
      },
    } as MessageEvent)
    expect(doc.sabPairBroker.snapshot()).toHaveLength(1)

    doc.onWebWorkerMessage('worker-b', {
      data: {
        from: 'worker-b',
        close: true,
      },
    } as MessageEvent)

    expect(doc.sabPairBroker.snapshot()).toEqual([])
    expect(sourcePostMessage).toHaveBeenLastCalledWith({
      from: 'document-1',
      sabPairClosed: {
        pairId: 'sab-pair-1',
        reason: 'worker closed',
      },
    })
  })
})
