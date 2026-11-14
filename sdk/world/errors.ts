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

// PluginOpTimeoutError reports that the plugin handling an operation ran past
// the host's deadline and the host discarded the transaction, so the
// operation wrote nothing and sending it again is safe. The host reports a
// timeout it cannot prove kept nothing as a plain error instead.
const pluginOpTimeoutBrand = Symbol.for('spacewave.PluginOpTimeoutError')

export class PluginOpTimeoutError extends Error {
  override readonly name = 'PluginOpTimeoutError'
  readonly [pluginOpTimeoutBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      pluginOpTimeoutBrand in value &&
      value[pluginOpTimeoutBrand] === true
    )
  }

  constructor() {
    super('the plugin did not finish in time, so nothing was changed')
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
  if (response.errorCode === WorldErrorCode.PLUGIN_OP_TIMEOUT)
    throw new PluginOpTimeoutError()
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
// rejection as WorldOperationRejection, a plugin timeout as
// PluginOpTimeoutError, and another built-in code as WorldBatchRefusal.
export function throwBatchError(response: BatchResponse): void {
  if (response.rejectionCode) throwOperationError(response)
  if (response.errorCode === WorldErrorCode.PLUGIN_OP_TIMEOUT)
    throw new PluginOpTimeoutError()
  if (response.errorCode)
    throw new WorldBatchRefusal(response.failedIndex ?? 0, response.errorCode)
}
