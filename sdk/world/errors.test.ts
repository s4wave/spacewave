import { describe, expect, it } from 'vitest'

import {
  PluginOpTimeoutError,
  throwBatchError,
  throwOperationError,
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
})
