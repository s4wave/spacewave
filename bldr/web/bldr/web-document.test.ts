import { afterEach, describe, expect, it, vi } from 'vitest'

import type { WebDocumentToClient } from '../runtime/runtime.js'
import {
  WebWorkerGenerationState,
  WebWorkerType,
} from '../document/document.pb.js'
import {
  WebDocument,
  registerUpdatedServiceWorker,
  shouldForceDedicatedWorkers,
} from './web-document.js'
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
  webViews: Record<string, unknown>
  webWorkers: Record<string, Record<string, unknown> & { port: MessagePort }>
  pluginSingletonReady: Promise<void>
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
  buildWebDocumentStatusSnapshot(): Promise<unknown>
  openWebDocumentHostStream(): Promise<unknown>
  removeWebWorker(request: { id: string }): Promise<unknown>
  taskEnsureWebRuntimeConn(): void
  webRuntimeClient: {
    openStream: () => Promise<unknown>
    waitConn?: () => Promise<unknown>
  }
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
    webViews: {},
    webWorkers: {},
    pluginSingletonReady: Promise.resolve(),
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

function buildTestWorker(port: MessagePort = {} as MessagePort): Record<
  string,
  unknown
> & {
  port: MessagePort
  isShared: boolean
  ready: boolean
  plugin: boolean
  workerType: WebWorkerType
  generationState: WebWorkerGenerationState
  failureReason?: string
  setGenerationState(
    generationState: WebWorkerGenerationState,
    failureReason?: string,
  ): void
  close: ReturnType<typeof vi.fn>
} {
  return {
    port,
    isShared: false,
    ready: false,
    plugin: true,
    workerType: WebWorkerType.NATIVE,
    generationState: WebWorkerGenerationState.STARTUP_RUNNING,
    setGenerationState(
      generationState: WebWorkerGenerationState,
      failureReason?: string,
    ) {
      this.generationState = generationState
      if (failureReason) {
        this.failureReason = failureReason
      }
    },
    close: vi.fn().mockResolvedValue(undefined),
  }
}

describe('registerUpdatedServiceWorker', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
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

describe('shouldForceDedicatedWorkers', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('uses dedicated runtime when explicitly forced', () => {
    vi.stubGlobal('SharedWorker', class {})

    expect(shouldForceDedicatedWorkers(true)).toBe(true)
  })

  it('uses dedicated runtime when SharedWorker is unavailable', () => {
    vi.stubGlobal('SharedWorker', undefined)

    expect(shouldForceDedicatedWorkers()).toBe(true)
  })

  it('uses dedicated runtime for Firefox', () => {
    vi.stubGlobal('SharedWorker', class {})
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(
      'Mozilla/5.0 Firefox/147.0',
    )

    expect(shouldForceDedicatedWorkers()).toBe(true)
  })

  it('keeps SharedWorker runtime for non-Firefox browsers with SharedWorker', () => {
    vi.stubGlobal('SharedWorker', class {})
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(
      'Mozilla/5.0 Chrome/143.0.0.0 Safari/537.36',
    )

    expect(shouldForceDedicatedWorkers()).toBe(false)
  })
})

describe('WebDocument resume-ready state', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
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

  it('does not schedule a timer retry when runtime connection fails', async () => {
    vi.useFakeTimers()
    const doc = buildTestWebDocument()
    const waitConn = vi.fn().mockRejectedValue(new Error('runtime unavailable'))
    doc.webRuntimeClient = {
      openStream: vi.fn(),
      waitConn,
    }
    const setTimeout = vi.spyOn(globalThis, 'setTimeout')

    doc.taskEnsureWebRuntimeConn()
    await Promise.resolve()
    await Promise.resolve()

    expect(waitConn).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1000)

    expect(waitConn).toHaveBeenCalledTimes(1)
    expect(setTimeout).not.toHaveBeenCalledWith(expect.any(Function), 100)
  })
})

describe('WebDocument plugin generation state', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    resetStartupMarksForTest()
  })

  it('publishes frontend, capability, and running states from the worker ready marker', () => {
    vi.spyOn(performance, 'mark').mockImplementation(() => {
      return {} as PerformanceMark
    })
    const doc = buildTestWebDocument()
    const worker = buildTestWorker()
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage('worker-1', {
      data: {
        from: 'worker-1',
        ready: true,
      },
    } as MessageEvent)

    expect(worker.ready).toBe(true)
    expect(worker.generationState).toBe(WebWorkerGenerationState.RUNNING)
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      false,
      undefined,
      WebWorkerGenerationState.FRONTEND_READY,
    )
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      false,
      undefined,
      WebWorkerGenerationState.CAPABILITY_READY,
    )
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      true,
      undefined,
      WebWorkerGenerationState.RUNNING,
    )
  })

  it('publishes frontend and capability states before the final ready marker', () => {
    const doc = buildTestWebDocument()
    const worker = buildTestWorker()
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage(
      'worker-1',
      new MessageEvent('message', {
        data: {
          from: 'worker-1',
          frontendReady: true,
        },
      }),
    )

    expect(worker.ready).toBe(false)
    expect(worker.generationState).toBe(WebWorkerGenerationState.FRONTEND_READY)
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      false,
      undefined,
      WebWorkerGenerationState.FRONTEND_READY,
    )

    doc.onWebWorkerMessage(
      'worker-1',
      new MessageEvent('message', {
        data: {
          from: 'worker-1',
          capabilityReady: true,
        },
      }),
    )

    expect(worker.ready).toBe(false)
    expect(worker.generationState).toBe(
      WebWorkerGenerationState.CAPABILITY_READY,
    )
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      false,
      undefined,
      WebWorkerGenerationState.CAPABILITY_READY,
    )

    doc.onWebWorkerMessage(
      'worker-1',
      new MessageEvent('message', {
        data: {
          from: 'worker-1',
          ready: true,
        },
      }),
    )

    expect(worker.ready).toBe(true)
    expect(worker.generationState).toBe(WebWorkerGenerationState.RUNNING)
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      true,
      undefined,
      WebWorkerGenerationState.RUNNING,
    )
  })

  it('publishes capability state after synthesizing missing frontend state', () => {
    const doc = buildTestWebDocument()
    const worker = buildTestWorker()
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage(
      'worker-1',
      new MessageEvent('message', {
        data: {
          from: 'worker-1',
          capabilityReady: true,
        },
      }),
    )

    expect(worker.ready).toBe(false)
    expect(worker.generationState).toBe(
      WebWorkerGenerationState.CAPABILITY_READY,
    )
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      false,
      undefined,
      WebWorkerGenerationState.FRONTEND_READY,
    )
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      false,
      undefined,
      WebWorkerGenerationState.CAPABILITY_READY,
    )
  })

  it('classifies fatal worker close as terminal failure', () => {
    const doc = buildTestWebDocument()
    const port = { close: vi.fn() } as unknown as MessagePort
    const worker = buildTestWorker(port)
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage('worker-1', {
      data: {
        from: 'worker-1',
        close: true,
        failureReason: 'fatal wasm exit',
      },
    } as MessageEvent)

    expect(port.close).toHaveBeenCalled()
    expect(doc.webWorkers['worker-1']).toBeUndefined()
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      true,
      false,
      false,
      'fatal wasm exit',
      WebWorkerGenerationState.TERMINAL_FAILURE,
    )
  })

  it('classifies controlled stream reset separately from terminal failure', () => {
    const doc = buildTestWebDocument()
    const port = { close: vi.fn() } as unknown as MessagePort
    const worker = buildTestWorker(port)
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage('worker-1', {
      data: {
        from: 'worker-1',
        close: true,
        failureReason: 'StreamResetError: stream reset',
      },
    } as MessageEvent)

    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      true,
      false,
      false,
      'StreamResetError: stream reset',
      WebWorkerGenerationState.CONTROLLED_STREAM_RESET,
    )
  })

  it('classifies startup timeout separately from terminal failure', () => {
    const doc = buildTestWebDocument()
    const port = { close: vi.fn() } as unknown as MessagePort
    const worker = buildTestWorker(port)
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage('worker-1', {
      data: {
        from: 'worker-1',
        close: true,
        failureReason: 'startup timeout waiting for capability',
      },
    } as MessageEvent)

    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      true,
      false,
      false,
      'startup timeout waiting for capability',
      WebWorkerGenerationState.STARTUP_TIMEOUT,
    )
  })

  it('publishes QuickJS ready markers as a running generation', () => {
    const doc = buildTestWebDocument()
    const worker = buildTestWorker()
    worker.workerType = WebWorkerType.QUICKJS
    doc.webWorkers = {
      'worker-1': worker,
    }

    doc.onWebWorkerMessage('worker-1', {
      data: {
        from: 'worker-1',
        ready: true,
      },
    } as MessageEvent)

    expect(worker.generationState).toBe(WebWorkerGenerationState.RUNNING)
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      false,
      false,
      true,
      undefined,
      WebWorkerGenerationState.RUNNING,
    )
  })

  it('does not create plugin workers when singleton ownership is unavailable', async () => {
    const doc = buildTestWebDocument()
    doc.pluginSingletonReady = Promise.reject(
      new Error('Web Locks unavailable'),
    )
    doc.pluginSingletonReady.catch(() => {})

    await expect(
      WebDocument.prototype.createWebWorker.call(doc, {
        id: 'worker-1',
        path: '/worker.js',
        initData: new Uint8Array([1]),
      }),
    ).resolves.toEqual({ created: false, shared: false })

    expect(globalThis.__swStartupMarks?.map((mark) => mark.label)).toEqual([
      'worker.create-request-received',
      'singleton-lock.wait-start',
    ])
  })

  it('publishes normal stop for explicit worker removal', async () => {
    const doc = buildTestWebDocument()
    const worker = buildTestWorker()
    worker.ready = true
    doc.webWorkers = {
      'worker-1': worker,
    }

    await doc.removeWebWorker({ id: 'worker-1' })

    expect(worker.close).toHaveBeenCalledOnce()
    expect(doc.webWorkers['worker-1']).toBeUndefined()
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-1',
      true,
      false,
      true,
      undefined,
      WebWorkerGenerationState.NORMAL_STOP,
    )
  })

  it('includes generation state and failure classification in status snapshots', async () => {
    const doc = buildTestWebDocument()
    const worker = buildTestWorker()
    worker.setGenerationState(
      WebWorkerGenerationState.TERMINAL_FAILURE,
      'fatal wasm exit',
    )
    doc.webWorkers = {
      'worker-1': worker,
    }

    await expect(doc.buildWebDocumentStatusSnapshot()).resolves.toMatchObject({
      webWorkers: [
        {
          id: 'worker-1',
          failed: true,
          failureReason: 'fatal wasm exit',
          generationState: WebWorkerGenerationState.TERMINAL_FAILURE,
        },
      ],
    })
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
      'worker-a': buildTestWorker({
        close,
      } as unknown as MessagePort),
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
      undefined,
      WebWorkerGenerationState.RUNNING,
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
      'worker-b': buildTestWorker({
        postMessage: targetPostMessage,
        close: vi.fn(),
      } as unknown as MessagePort),
    }
    doc.webWorkers['worker-b'].ready = true

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

  it('marks worker close with a failure reason as failed', () => {
    const doc = buildTestWebDocument()
    const close = vi.fn()
    doc.webWorkers = {
      'worker-a': buildTestWorker({
        close,
      } as unknown as MessagePort),
    }
    doc.webWorkers['worker-a'].ready = true

    doc.onWebWorkerMessage('worker-a', {
      data: {
        from: 'worker-a',
        close: true,
        failureReason: 'fatal wasm exit',
      },
    } as MessageEvent)

    expect(doc.webWorkers['worker-a']).toBeUndefined()
    expect(close).toHaveBeenCalledOnce()
    expect(doc.notifyWebWorkerUpdated).toHaveBeenCalledWith(
      'worker-a',
      true,
      false,
      true,
      'fatal wasm exit',
      WebWorkerGenerationState.TERMINAL_FAILURE,
    )
  })
})
