import { describe, expect, it } from 'vitest'

import { throwOperationError, WorldOperationRejection } from './errors.js'
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

  it('accepts a successful response', () => {
    expect(() => throwOperationError({})).not.toThrow()
  })
})
