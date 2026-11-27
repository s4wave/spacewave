import { afterEach, expect, it, vi } from 'vitest'
import { pushable } from 'it-pushable'
import type { ResourceClientResponse } from '@aptre/bldr-sdk/resource/resource.pb.js'
import { Client } from '@aptre/bldr-sdk/resource/client.js'
import type { ResourceService } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
} from '@s4wave/core/sobject/sobject.pb.js'
import { Session } from '../session/session.js'
import { SessionResourceServiceClient } from '../session/session_srpc.pb.js'
import { SharedObjectResourceServiceClient } from './sobject_srpc.pb.js'
import {
  getSharedObjectHealthFromError,
  SharedObject,
  SharedObjectHealthError,
} from './sobject.js'

afterEach(() => vi.restoreAllMocks())

it.each([
  [
    SharedObjectHealthCommonReason.NOT_FOUND,
    SharedObjectHealthRemediationHint.CONTACT_OWNER,
  ],
  [
    SharedObjectHealthCommonReason.ACCESS_REVOKED,
    SharedObjectHealthRemediationHint.REQUEST_ACCESS,
  ],
  [
    SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
    SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
  ],
])(
  'preserves typed health through mount and body SDK calls: %s',
  async (commonReason, remediationHint) => {
    // Return typed health from both mount RPC adapters.
    const health = {
      status: SharedObjectHealthStatus.CLOSED,
      layer: SharedObjectHealthLayer.SHARED_OBJECT,
      commonReason,
      remediationHint,
      error: 'diagnostic wording unrelated to recovery',
    }
    vi.spyOn(
      SessionResourceServiceClient.prototype,
      'MountSharedObject',
    ).mockResolvedValue({ health })
    vi.spyOn(
      SharedObjectResourceServiceClient.prototype,
      'MountSharedObjectBody',
    ).mockResolvedValue({ result: { case: 'health', value: health } })

    // Keep the client generation open until the fixture disposes it.
    const responses = pushable<ResourceClientResponse>({ objectMode: true })
    responses.push({
      body: {
        case: 'init',
        value: { clientHandleId: 1, rootResourceId: 1 },
      },
    })
    const service: ResourceService = {
      ResourceClient: (_requests, signal) => {
        signal?.addEventListener('abort', () => responses.end(), { once: true })
        return responses
      },
      ResourceRpc: async function* () {},
      ResourceAttach: async function* () {},
    }

    // Acquire a live root before checking each SDK adapter.
    const client = new Client(service, new AbortController().signal)
    try {
      const ref = await client.accessRootResource()
      await expect(
        new Session(ref, {}).mountSharedObject({ sharedObjectId: 'example' }),
      ).rejects.toMatchObject({ health })
      await expect(
        new SharedObject(ref, {}).mountSharedObjectBody(),
      ).rejects.toMatchObject({ health })
      expect(ref.released).toBe(false)
    } finally {
      client.dispose()
    }
  },
)

it('recognizes health from another bundle without interpreting text', () => {
  // Recognize the shared error brand while leaving ordinary errors untyped.
  const health = { commonReason: SharedObjectHealthCommonReason.ACCESS_REVOKED }
  const foreign = Object.assign(new Error('opaque'), {
    [Symbol.for('spacewave.SharedObjectHealthError')]: true,
    health,
  })
  expect(foreign).toBeInstanceOf(SharedObjectHealthError)
  expect(getSharedObjectHealthFromError(foreign)).toBe(health)
  expect(
    getSharedObjectHealthFromError(new Error('not a participant')),
  ).toBeNull()
})
