import { describe, expect, it, vi } from 'vitest'

import type { Engine } from '../world/engine.js'
import { WorldErrorCode } from '../world/world.pb.js'
import type { AppDefinition } from './app.js'
import { createAppInstance } from './attachment.js'
import { SyncError } from './errors.js'

// attached stops createAppInstance after acceptance, where it attaches.
const attached = new Error('attached')

// engineReplying returns an Engine whose ExecuteWorldOp reply carries code.
function engineReplying(code: WorldErrorCode | undefined) {
  const sync = vi.fn(async () => true)
  const engine = {
    executeWorldOp: async () => ({ errorCode: code }),
    sync,
    getEngineInfo: async () => {
      throw attached
    },
  } as unknown as Engine
  return { engine, sync }
}

const app = { schema: { id: 'test/app' } } as unknown as AppDefinition<never>
const revision = { pluginId: 'plugin', manifestRoot: 'root' }

// create accepts a create request through engine.
function create(engine: Engine) {
  return createAppInstance(app, engine, revision, 'app/1', {
    requestId: 'req-1',
  })
}

describe('createAppInstance acceptance', () => {
  it.each([
    WorldErrorCode.TYPED_OBJECT_GRANT_RETIRED,
    WorldErrorCode.SUPERSEDED,
    (Math.max(
      ...Object.values(WorldErrorCode).filter(
        (v): v is number => typeof v === 'number',
      ),
    ) + 1) as WorldErrorCode,
  ])('code %i fails as uncertain without syncing', async (code) => {
    const { engine, sync } = engineReplying(code)
    const err = await create(engine).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(SyncError)
    expect(err).toMatchObject({ code: 'UNCERTAIN', requestId: 'req-1' })
    expect(sync).not.toHaveBeenCalled()
  })

  it.each([WorldErrorCode.UNSPECIFIED, undefined])(
    'code %s is accepted and synced',
    async (code) => {
      const { engine, sync } = engineReplying(code)
      await expect(create(engine)).rejects.toBe(attached)
      expect(sync).toHaveBeenCalledOnce()
    },
  )
})
