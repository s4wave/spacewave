import { describe, expect, it, vi } from 'vitest'

import type { ResourceService } from './resource_srpc.pb.js'
import { Client } from './client.js'

type TestAttachSession = {
  controller: AbortController
  outgoing: { end: () => void; push: () => void }
  attachIdCtr: number
  muxes: Map<number, unknown>
  pending: Map<
    number,
    {
      resolve: (resourceId: number) => void
      reject: (err: Error) => void
    }
  >
}

type TestClient = Client & {
  attachSession: TestAttachSession | null
  releaseAllResources: (reason: 'connection-lost') => void
}

describe('ResourceClient', () => {
  it('clears stale attachSession state on reconnect cleanup', () => {
    const client = new Client(
      {} as ResourceService,
      new AbortController().signal,
    ) as unknown as TestClient
    const controller = new AbortController()
    const end = vi.fn()
    const reject = vi.fn()

    client.attachSession = {
      controller,
      outgoing: { end, push: vi.fn() },
      attachIdCtr: 1,
      muxes: new Map([[1, vi.fn()]]),
      pending: new Map([[1, { resolve: vi.fn(), reject }]]),
    }

    client.releaseAllResources('connection-lost')

    expect(client.attachSession).toBe(null)
    expect(controller.signal.aborted).toBe(true)
    expect(end).toHaveBeenCalledOnce()
    expect(reject).toHaveBeenCalledWith(expect.any(Error))
  })

  it('clears stale attachSession state on dispose', () => {
    const client = new Client(
      {} as ResourceService,
      new AbortController().signal,
    ) as unknown as TestClient
    const controller = new AbortController()
    const end = vi.fn()

    client.attachSession = {
      controller,
      outgoing: { end, push: vi.fn() },
      attachIdCtr: 1,
      muxes: new Map(),
      pending: new Map(),
    }

    client.dispose()

    expect(client.attachSession).toBe(null)
    expect(controller.signal.aborted).toBe(true)
    expect(end).toHaveBeenCalledOnce()
  })
})
