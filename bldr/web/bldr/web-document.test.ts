import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebDocument, registerUpdatedServiceWorker } from './web-document.js'
import { resetStartupMarksForTest, startupMarkPrefix } from './startup-marks.js'

type TestWebDocument = {
  closed?: true | Error
  hidden: boolean
  resumeReady: boolean
  resumeReadyPending: boolean
  runtimeConnected: boolean
  scheduleResumeReadySeed(): void
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
    runtimeConnected: true,
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
    })
    expect(mark).toHaveBeenCalledWith(
      `${startupMarkPrefix}web-document.resume-ready`,
      expect.objectContaining({
        detail: expect.objectContaining({
          label: 'web-document.resume-ready',
          documentId: 'document-1',
          runtimeId: 'runtime-1',
        }),
      }),
    )
  })

  it('does not seed resume-ready while hidden', () => {
    const raf = vi.fn()
    vi.stubGlobal('requestAnimationFrame', raf)
    const doc = buildTestWebDocument(true)

    doc.scheduleResumeReadySeed()

    expect(raf).not.toHaveBeenCalled()
    expect(globalThis.__swWebDocumentResumeReady).toBeUndefined()
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
    }
    doc.webRuntimeClient = {
      openStream: vi.fn().mockRejectedValue(err),
    }

    await expect(doc.openWebDocumentHostStream()).rejects.toThrow(
      'stream-open failed',
    )
  })
})
