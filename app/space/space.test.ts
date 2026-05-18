import { describe, expect, it, vi } from 'vitest'

import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
import { Space } from '@s4wave/sdk/space/space.js'

import { mountSpace } from './space.js'

function buildResourceRef(id: number): ClientResourceRef {
  const ref = {
    resourceId: id,
    released: false,
    client: {},
    createRef: vi.fn((nextId: number) => buildResourceRef(nextId)),
    createResource: vi.fn(
      <T, Args extends unknown[]>(
        nextId: number,
        ResourceClass: new (ref: ClientResourceRef, ...args: Args) => T,
        ...args: Args
      ): T => new ResourceClass(buildResourceRef(nextId), ...args),
    ),
    release: vi.fn(),
    [Symbol.dispose]: vi.fn(),
  }
  return ref as unknown as ClientResourceRef
}

describe('mountSpace', () => {
  it('uses mounted CreateSpace resources without remounting by id', async () => {
    const cleanupMock = vi.fn(<T>(value: T): T => value)
    const cleanup = cleanupMock as unknown as RegisterCleanup
    const session = {
      resourceRef: buildResourceRef(10),
      mountSharedObject: vi.fn(),
    }

    const space = await mountSpace({
      session: session as never,
      spaceResp: {
        sharedObjectRef: {
          providerResourceRef: { id: 'space-1' },
          blockStoreId: 'sobject-space-1',
        },
        sharedObjectMeta: { bodyType: 'space' },
        mountedSharedObject: {
          resourceId: 11,
          sharedObjectMeta: { bodyType: 'space' },
          sharedObjectId: 'space-1',
          blockStoreId: 'sobject-space-1',
        },
        sharedObjectBodyResourceId: 12,
      },
      abortSignal: new AbortController().signal,
      cleanup,
    })

    expect(session.mountSharedObject).not.toHaveBeenCalled()
    expect(cleanupMock.mock.calls[0]?.[0]).toBeInstanceOf(SharedObject)
    expect(cleanupMock.mock.calls[1]?.[0]).toBeInstanceOf(SharedObjectBody)
    expect(cleanupMock.mock.calls[2]?.[0]).toBeInstanceOf(Space)
    expect(space.id).toBe(12)
  })

  it('falls back to mounting older CreateSpace responses by shared object id', async () => {
    const cleanupMock = vi.fn(<T>(value: T): T => value)
    const cleanup = cleanupMock as unknown as RegisterCleanup
    const body = new SharedObjectBody(buildResourceRef(22))
    const sharedObject = {
      mountSharedObjectBody: vi.fn().mockResolvedValue(body),
    }
    const session = {
      mountSharedObject: vi.fn().mockResolvedValue(sharedObject),
    }

    const space = await mountSpace({
      session: session as never,
      spaceResp: {
        sharedObjectRef: {
          providerResourceRef: { id: 'space-1' },
          blockStoreId: 'sobject-space-1',
        },
      },
      abortSignal: new AbortController().signal,
      cleanup,
    })

    expect(session.mountSharedObject).toHaveBeenCalledWith(
      { sharedObjectId: 'space-1' },
      expect.any(AbortSignal),
    )
    expect(sharedObject.mountSharedObjectBody).toHaveBeenCalledWith(
      {},
      expect.any(AbortSignal),
    )
    expect(space.id).toBe(22)
  })
})
