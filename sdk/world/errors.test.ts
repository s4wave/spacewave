import { describe, expect, it } from 'vitest'

import {
  PluginExecutableMissingError,
  PluginOpTimeoutError,
  PluginUnavailableError,
  throwBatchError,
  throwCommitError,
  throwOperationError,
  WorldCommitError,
  WorldOperationRejection,
} from './errors.js'
import { WorldErrorCode } from './world.pb.js'

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
