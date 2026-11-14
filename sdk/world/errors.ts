import { WorldErrorCode } from './world.pb.js'

// WorldOperationRejection carries an operation's stable application-defined
// refusal, such as a validation failure or a conflicting request.
const rejectionBrand = Symbol.for('spacewave.WorldOperationRejection')

export class WorldOperationRejection extends Error {
  override readonly name = 'WorldOperationRejection'
  readonly [rejectionBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      rejectionBrand in value &&
      value[rejectionBrand] === true
    )
  }

  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message || code)
  }
}

// OperationResponse is the error part of a World or object operation response.
export interface OperationResponse {
  errorCode?: WorldErrorCode
  rejectionCode?: string
  rejectionMessage?: string
}

// throwOperationError restores the typed rejection or error an operation
// response carries across the Resource RPC boundary.
export function throwOperationError(response: OperationResponse): void {
  if (response.rejectionCode)
    throw new WorldOperationRejection(
      response.rejectionCode,
      response.rejectionMessage ?? '',
    )
  if (response.errorCode === WorldErrorCode.UNHANDLED_OP)
    throw new Error('remote world operation unhandled')
}

// WorldBatchRefusal refuses a World batch with a built-in code, such as an
// absent object a delete expected. failedIndex names the refused mutation.
export class WorldBatchRefusal extends Error {
  override readonly name = 'WorldBatchRefusal'

  constructor(
    readonly failedIndex: number,
    readonly code: WorldErrorCode,
  ) {
    super(
      `world batch mutation ${failedIndex} refused: ${WorldErrorCode[code]}`,
    )
  }
}

// BatchResponse is the error part of a World batch response.
export interface BatchResponse extends OperationResponse {
  failedIndex?: number
}

// throwBatchError throws the refusal a batch response carries: an op's
// rejection as WorldOperationRejection, and a built-in code as
// WorldBatchRefusal.
export function throwBatchError(response: BatchResponse): void {
  if (response.rejectionCode) throwOperationError(response)
  if (response.errorCode)
    throw new WorldBatchRefusal(response.failedIndex ?? 0, response.errorCode)
}
