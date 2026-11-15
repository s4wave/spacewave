import { describe, expect, it } from 'vitest'

import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Engine } from './engine.js'
import { WorldOperationError } from './errors.js'
import { TypedObjectGrantRetiredError } from './typed-object-grant-retired-error.js'
import { AccessTypedObjectResponse, WorldErrorCode } from './world.pb.js'

// engineReplying returns an Engine whose AccessTypedObject reply carries code,
// encoded as the server sends it.
function engineReplying(code: WorldErrorCode | undefined): Engine {
  const client = {
    request: async () =>
      AccessTypedObjectResponse.toBinary({
        resourceId: 7,
        typeId: 'type/doc',
        errorCode: code,
      }),
  }
  return new Engine({ client } as unknown as ClientResourceRef)
}

const codes = Object.values(WorldErrorCode).filter(
  (v): v is WorldErrorCode => typeof v === 'number',
)

describe('accessTypedObject replies', () => {
  it('restores a retired grant', async () => {
    await expect(
      engineReplying(
        WorldErrorCode.TYPED_OBJECT_GRANT_RETIRED,
      ).accessTypedObject('doc'),
    ).rejects.toBeInstanceOf(TypedObjectGrantRetiredError)
  })

  it.each([
    WorldErrorCode.UNHANDLED_OP,
    WorldErrorCode.PLUGIN_UNAVAILABLE,
    (Math.max(...codes) + 1) as WorldErrorCode,
  ])('code %i fails', async (code) => {
    await expect(
      engineReplying(code).accessTypedObject('doc'),
    ).rejects.toBeInstanceOf(WorldOperationError)
  })

  it.each([WorldErrorCode.UNSPECIFIED, undefined])(
    'code %s grants access',
    async (code) => {
      await expect(
        engineReplying(code).accessTypedObject('doc'),
      ).resolves.toEqual({ resourceId: 7, typeId: 'type/doc' })
    },
  )
})
