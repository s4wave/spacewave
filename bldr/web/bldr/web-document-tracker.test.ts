import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebDocumentTracker } from './web-document-tracker.js'

function buildTracker(): WebDocumentTracker {
  return new WebDocumentTracker(
    'tracker-client',
    WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    vi.fn().mockResolvedValue(undefined),
    null,
  )
}

function waitForActiveWebDocumentResumeReady(
  tracker: WebDocumentTracker,
): Promise<void> {
  const waitForResumeReady = Reflect.get(
    tracker,
    'waitForActiveWebDocumentResumeReady',
  ) as (this: WebDocumentTracker) => Promise<void>
  return waitForResumeReady.call(tracker)
}

describe('WebDocumentTracker resume-ready gate', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('resolves the active document resume-ready gate from the WebDocument port', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    const readyPromise = waitForActiveWebDocumentResumeReady(tracker)
    let resolved = false
    readyPromise.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)

    port2.postMessage({
      from: 'document-1',
      resumeReady: true,
    })

    await expect(readyPromise).resolves.toBeUndefined()
    expect(resolved).toBe(true)

    tracker.close()
    port2.close()
  })

  it('rejects the resume-ready gate when the active WebDocument closes', async () => {
    const tracker = buildTracker()
    const { port1, port2 } = new MessageChannel()
    tracker.handleWebDocumentMessage({
      from: 'document-1',
      initPort: port1,
    })
    Reflect.set(tracker, 'lastWebDocumentId', 'document-1')

    const readyPromise = waitForActiveWebDocumentResumeReady(tracker)

    port2.postMessage({
      from: 'document-1',
      close: true,
    })

    await expect(readyPromise).rejects.toThrow(
      'WebDocument document-1 closed before resume-ready',
    )

    tracker.close()
    port2.close()
  })
})
