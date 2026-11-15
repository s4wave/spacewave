import { describe, expect, it } from 'vitest'

import {
  PluginExecutableMissingError,
  PluginOpTimeoutError,
  PluginUnavailableError,
  throwBatchError,
  throwCommitError,
  throwOperationError,
  WorldCommitError,
  WorldOperationError,
  WorldOperationRejection,
} from './errors.js'
import { ObjectState } from './object-state.js'
import { WorldStateResource } from './world-state.js'
import { WorldErrorCode } from './world.pb.js'

// errorFor is the error each code an operation reply carries throws. The
// cases come from the enum, so a code added without an entry fails.
const errorFor: Record<
  WorldErrorCode,
  (new (...args: never[]) => Error) | null
> = {
  [WorldErrorCode.UNSPECIFIED]: null,
  [WorldErrorCode.UNHANDLED_OP]: Error,
  [WorldErrorCode.TYPED_OBJECT_GRANT_RETIRED]: WorldOperationError,
  [WorldErrorCode.OBJECT_NOT_FOUND]: WorldOperationError,
  [WorldErrorCode.PLUGIN_OP_TIMEOUT]: PluginOpTimeoutError,
  [WorldErrorCode.PLUGIN_UNAVAILABLE]: PluginUnavailableError,
  [WorldErrorCode.PLUGIN_EXECUTABLE_MISSING]: PluginExecutableMissingError,
  [WorldErrorCode.SUPERSEDED]: WorldCommitError,
  [WorldErrorCode.STALE_GENERATION]: WorldCommitError,
  [WorldErrorCode.ROOT_NOT_HELD]: WorldCommitError,
}

// replyWith returns a World state and an object state whose services answer
// every operation with a reply carrying code.
function replyWith(code: WorldErrorCode | undefined) {
  const reply = () => Promise.resolve({ seqno: 7n, rev: 7n, errorCode: code })
  const ws = Object.create(WorldStateResource.prototype) as WorldStateResource
  const obj = Object.create(ObjectState.prototype) as ObjectState
  Object.assign(ws, { service: { ApplyWorldOp: reply } })
  Object.assign(obj, { service: { ApplyObjectOp: reply } })
  return {
    world: () => ws.applyWorldOp('test/empty', new Uint8Array(), ''),
    object: () => obj.applyObjectOp('test/empty', new Uint8Array(), ''),
  }
}

describe('operation replies', () => {
  const codes = Object.values(WorldErrorCode).filter(
    (v): v is WorldErrorCode => typeof v === 'number',
  )

  it('maps every enum code', () => {
    expect(Object.keys(errorFor).map(Number).sort()).toEqual(
      [...codes].sort((a, b) => a - b),
    )
  })

  it.each(codes.filter((c) => c !== WorldErrorCode.UNSPECIFIED))(
    'code %i throws its error',
    async (code) => {
      const want = errorFor[code]!
      const { world, object } = replyWith(code)
      await expect(world()).rejects.toBeInstanceOf(want)
      await expect(object()).rejects.toBeInstanceOf(want)
    },
  )

  it('throws on a code this build does not know', async () => {
    const code = (Math.max(...codes) + 1) as WorldErrorCode
    const { world, object } = replyWith(code)
    await expect(world()).rejects.toMatchObject({
      name: 'WorldOperationError',
      code,
    })
    await expect(object()).rejects.toBeInstanceOf(WorldOperationError)
  })

  it.each([WorldErrorCode.UNSPECIFIED, undefined])(
    'code %s is success',
    async (code) => {
      const { world, object } = replyWith(code)
      await expect(world()).resolves.toEqual({ seqno: 7n, sysErr: false })
      await expect(object()).resolves.toMatchObject({ rev: 7n })
    },
  )
})

describe('throwOperationError', () => {
  it('restores an operation rejection', () => {
    let caught: unknown
    try {
      throwOperationError({
        rejectionCode: 'request-conflict',
        rejectionMessage: 'The request ID names different work.',
      })
    } catch (err) {
      caught = err
    }
    expect(caught).toBeInstanceOf(WorldOperationRejection)
    expect(caught).toMatchObject({
      code: 'request-conflict',
      message: 'The request ID names different work.',
    })
  })

  it('reports an unhandled operation', () => {
    expect(() =>
      throwOperationError({
        errorCode: WorldErrorCode.UNHANDLED_OP,
      }),
    ).toThrow('remote world operation unhandled')
  })

  it('restores a plugin op timeout that wrote nothing', () => {
    expect(() =>
      throwOperationError({
        errorCode: WorldErrorCode.PLUGIN_OP_TIMEOUT,
      }),
    ).toThrow(PluginOpTimeoutError)
  })

  it('restores an unavailable plugin that was sent nothing', () => {
    expect(() =>
      throwOperationError({
        errorCode: WorldErrorCode.PLUGIN_UNAVAILABLE,
      }),
    ).toThrow(PluginUnavailableError)
  })

  it('restores a plugin with no build for this device', () => {
    expect(() =>
      throwOperationError({
        errorCode: WorldErrorCode.PLUGIN_EXECUTABLE_MISSING,
      }),
    ).toThrow(PluginExecutableMissingError)
  })

  it('accepts a successful response', () => {
    expect(() => throwOperationError({})).not.toThrow()
  })
})

describe('throwBatchError', () => {
  it('restores a plugin op timeout that wrote nothing', () => {
    expect(() =>
      throwBatchError({
        failedIndex: 2,
        errorCode: WorldErrorCode.PLUGIN_OP_TIMEOUT,
      }),
    ).toThrow(PluginOpTimeoutError)
  })

  it('restores an unavailable plugin that was sent nothing', () => {
    expect(() =>
      throwBatchError({
        failedIndex: 1,
        errorCode: WorldErrorCode.PLUGIN_UNAVAILABLE,
      }),
    ).toThrow(PluginUnavailableError)
  })
  it('restores a plugin with no build for this device', () => {
    expect(() =>
      throwBatchError({
        failedIndex: 1,
        errorCode: WorldErrorCode.PLUGIN_EXECUTABLE_MISSING,
      }),
    ).toThrow(PluginExecutableMissingError)
  })
})

describe('throwCommitError', () => {
  it('returns for a published commit', () => {
    expect(() => throwCommitError({})).not.toThrow()
  })

  it('restores a superseded commit with its message', () => {
    let caught: unknown
    try {
      throwCommitError({
        errorCode: WorldErrorCode.SUPERSEDED,
        errorMessage: 'head moved to seqno 3: world candidate superseded',
      })
    } catch (err) {
      caught = err
    }
    expect(caught).toBeInstanceOf(WorldCommitError)
    expect(caught).toMatchObject({
      code: WorldErrorCode.SUPERSEDED,
      superseded: true,
      message: 'head moved to seqno 3: world candidate superseded',
    })
  })

  it('restores a stale generation as superseded with its message', () => {
    let caught: unknown
    try {
      throwCommitError({
        errorCode: WorldErrorCode.STALE_GENERATION,
        errorMessage: 'base root is stale: coord: stale generation',
      })
    } catch (err) {
      caught = err
    }
    expect(caught).toBeInstanceOf(WorldCommitError)
    expect(caught).toMatchObject({
      code: WorldErrorCode.STALE_GENERATION,
      superseded: true,
      message: 'base root is stale: coord: stale generation',
    })
  })

  it('restores a not-held refusal as not superseded', () => {
    let caught: unknown
    try {
      throwCommitError({ errorCode: WorldErrorCode.ROOT_NOT_HELD })
    } catch (err) {
      caught = err
    }
    expect(caught).toBeInstanceOf(WorldCommitError)
    expect(caught).toMatchObject({
      code: WorldErrorCode.ROOT_NOT_HELD,
      superseded: false,
    })
  })
})
