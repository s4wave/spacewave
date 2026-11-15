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

// PluginUnavailableError reports that the plugin handling an operation could
// not be reached: its load failed or did not finish in time. The host never
// sent the operation or opened a transaction, so nothing was written and
// sending it again is safe.
const pluginUnavailableBrand = Symbol.for('spacewave.PluginUnavailableError')

export class PluginUnavailableError extends Error {
  override readonly name = 'PluginUnavailableError'
  readonly [pluginUnavailableBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      pluginUnavailableBrand in value &&
      value[pluginUnavailableBrand] === true
    )
  }

  constructor() {
    super('the plugin could not be reached, so nothing was changed')
  }
}

// PluginExecutableMissingError reports that the plugin build an operation is
// pinned to has no executable for this device's platform. The host never sent
// the operation or opened a transaction, so nothing was written. Sending it
// again on this device fails the same way; a device the build supports can
// apply it.
const pluginExecutableMissingBrand = Symbol.for(
  'spacewave.PluginExecutableMissingError',
)

export class PluginExecutableMissingError extends Error {
  override readonly name = 'PluginExecutableMissingError'
  readonly [pluginExecutableMissingBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      pluginExecutableMissingBrand in value &&
      value[pluginExecutableMissingBrand] === true
    )
  }

  constructor() {
    super('the plugin has no build for this device, so nothing was changed')
  }
}

// ReplayWaitError reports that the World took no write because its replay is
// not caught up: it is opening, or waits at an operation for a handler this
// device lacks or a plugin's approval. Nothing was written and reads go on.
// The write may be sent again once SharedObjectHealth.replay is caught_up.
const replayWaitBrand = Symbol.for('spacewave.ReplayWaitError')

export class ReplayWaitError extends Error {
  override readonly name = 'ReplayWaitError'
  readonly [replayWaitBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      replayWaitBrand in value &&
      value[replayWaitBrand] === true
    )
  }

  constructor() {
    super('the Space is catching up and is read only until it does')
  }
}

// typedError returns the typed error for a plugin's built-in code or a
// replay wait, or undefined for any other code.
export function typedError(
  code: WorldErrorCode | undefined,
): Error | undefined {
  switch (code) {
    case WorldErrorCode.PLUGIN_OP_TIMEOUT:
      return new PluginOpTimeoutError()
    case WorldErrorCode.PLUGIN_UNAVAILABLE:
      return new PluginUnavailableError()
    case WorldErrorCode.PLUGIN_EXECUTABLE_MISSING:
      return new PluginExecutableMissingError()
    case WorldErrorCode.REPLAY_WAITS:
      return new ReplayWaitError()
  }
  return undefined
}

// WorldCommitError reports a commit the World refused with a typed result, so
// nothing was committed. superseded is true when another writer advanced the
// World first (SUPERSEDED or STALE_GENERATION): the caller may retry from a
// new transaction. ROOT_NOT_HELD means the transaction's tree names a block
// the Volume does not hold.
const worldCommitBrand = Symbol.for('spacewave.WorldCommitError')

export class WorldCommitError extends Error {
  override readonly name = 'WorldCommitError'
  readonly [worldCommitBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      worldCommitBrand in value &&
      value[worldCommitBrand] === true
    )
  }

  constructor(
    readonly code: WorldErrorCode,
    message: string,
  ) {
    super(message || `world commit refused: ${WorldErrorCode[code]}`)
  }

  // superseded reports whether another writer advanced the World first.
  get superseded(): boolean {
    return (
      this.code === WorldErrorCode.SUPERSEDED ||
      this.code === WorldErrorCode.STALE_GENERATION
    )
  }
}

// CommitErrorResponse is the error part of a World transaction's Commit
// response.
export interface CommitErrorResponse {
  errorCode?: WorldErrorCode
  errorMessage?: string
}

// throwCommitError throws the typed refusal a CommitTx response carries as
// WorldCommitError, and returns when the commit published.
export function throwCommitError(response: CommitErrorResponse): void {
  if (response.errorCode)
    throw new WorldCommitError(response.errorCode, response.errorMessage ?? '')
}

// OperationResponse is the error part of a World or object operation response.
export interface OperationResponse {
  errorCode?: WorldErrorCode
  rejectionCode?: string
  rejectionMessage?: string
}

// throwOperationError restores the typed rejection or error an operation
// response carries across the Resource RPC boundary. It returns only when the
// response carries no rejection and no error code, so a refused or failed
// operation never reads as success.
export function throwOperationError(response: OperationResponse): void {
  if (response.rejectionCode)
    throw new WorldOperationRejection(
      response.rejectionCode,
      response.rejectionMessage ?? '',
    )
  const code = response.errorCode
  if (!code) return
  if (code === WorldErrorCode.UNHANDLED_OP)
    throw new Error('remote world operation unhandled')
  const err = typedError(code)
  if (err) throw err
  switch (code) {
    case WorldErrorCode.SUPERSEDED:
    case WorldErrorCode.STALE_GENERATION:
    case WorldErrorCode.ROOT_NOT_HELD:
      throw new WorldCommitError(code, '')
  }
  throw new WorldOperationError(code)
}

// WorldOperationError reports an operation the World refused or failed with a
// built-in code that has no more specific error, including a code this build
// does not know. The operation did not succeed.
const worldOperationErrorBrand = Symbol.for('spacewave.WorldOperationError')

export class WorldOperationError extends Error {
  override readonly name = 'WorldOperationError'
  readonly [worldOperationErrorBrand] = true

  // Client and plugin builds recognize the same public error across bundles.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      worldOperationErrorBrand in value &&
      value[worldOperationErrorBrand] === true
    )
  }

  constructor(readonly code: WorldErrorCode) {
    super(`world operation failed: ${WorldErrorCode[code] ?? code}`)
  }
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
// rejection as WorldOperationRejection, a plugin's code or a replay wait as
// its typed error, and another built-in code as WorldBatchRefusal.
export function throwBatchError(response: BatchResponse): void {
  if (response.rejectionCode) throwOperationError(response)
  const err = typedError(response.errorCode)
  if (err) throw err
  if (response.errorCode)
    throw new WorldBatchRefusal(response.failedIndex ?? 0, response.errorCode)
}
