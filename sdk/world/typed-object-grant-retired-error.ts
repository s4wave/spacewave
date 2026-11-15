import { WorldErrorCode, type AccessTypedObjectResponse } from './world.pb.js'
import { WorldOperationError } from './errors.js'

/** TypedObjectGrantRetiredError identifies acquisition prevented by retirement of its granting World mount. */
export class TypedObjectGrantRetiredError extends Error {
  override readonly name = 'TypedObjectGrantRetiredError'

  constructor() {
    super('typed object granting World mount retired')
  }
}

/** throwTypedObjectError restores acquisition failure and preserves terminal caller cancellation. It returns only for a response with no error code. */
export function throwTypedObjectError(
  response: AccessTypedObjectResponse,
  abortSignal?: AbortSignal,
): void {
  if (response.errorCode === WorldErrorCode.TYPED_OBJECT_GRANT_RETIRED) {
    abortSignal?.throwIfAborted()
    throw new TypedObjectGrantRetiredError()
  }
  if (response.errorCode === WorldErrorCode.OBJECT_NOT_FOUND) {
    throw new Error('object not found')
  }
  // Any other code is a failure this build cannot name; access never
  // succeeds on one.
  if (response.errorCode) throw new WorldOperationError(response.errorCode)
}
