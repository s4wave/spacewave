import { WorldErrorCode, type AccessTypedObjectResponse } from './world.pb.js'

/** TypedObjectGrantRetiredError identifies acquisition prevented by retirement of its granting World mount. */
export class TypedObjectGrantRetiredError extends Error {
  override readonly name = 'TypedObjectGrantRetiredError'

  constructor() {
    super('typed object granting World mount retired')
  }
}

/** throwTypedObjectError restores acquisition failure and preserves terminal caller cancellation. */
export function throwTypedObjectError(
  response: AccessTypedObjectResponse,
  abortSignal?: AbortSignal,
): void {
  if (response.errorCode === WorldErrorCode.TYPED_OBJECT_GRANT_RETIRED) {
    abortSignal?.throwIfAborted()
    throw new TypedObjectGrantRetiredError()
  }
}
