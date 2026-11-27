export type SyncErrorCode =
  | 'VALIDATION'
  | 'AUTHENTICATION'
  | 'DENIED'
  | 'MISSING_COLLECTION'
  | 'SCHEMA_MISMATCH'
  | 'QUERY_LIMIT'
  | 'CONFLICT'
  | 'UNAVAILABLE'
  | 'UNCERTAIN'
  | 'STORAGE'
  | 'CLOSED'
  | 'EXECUTABLE_MISSING'

// errorBrand identifies SyncError, which carries a stable code and the
// recovery identity of an uncertain write.
const errorBrand = Symbol.for('spacewave.SyncError')

export class SyncError extends Error {
  override readonly name = 'SyncError'
  private readonly [errorBrand] = true

  // Client and server builds recognize the same public error across entry points.
  static [Symbol.hasInstance](value: unknown): boolean {
    return (
      value instanceof Error &&
      errorBrand in value &&
      value[errorBrand] === true
    )
  }

  constructor(
    readonly code: SyncErrorCode,
    message: string,
    readonly requestId?: string,
  ) {
    super(message)
  }
}

// publicError keeps internal failures out of application and transport responses.
export function publicError(error: unknown): SyncError {
  if (error instanceof SyncError) return error
  return new SyncError('STORAGE', 'The data operation could not complete')
}
