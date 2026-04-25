import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Session } from '@s4wave/sdk/session'
import type { CreateSpaceResponse } from '@s4wave/sdk/session/session.pb.js'
import { Space } from '@s4wave/sdk/space/space.js'

// MountSpaceParams contains the parameters for mounting a Space resource.
export interface MountSpaceParams {
  // Session is the session to mount the space from.
  session: Session
  // SpaceResp is the CreateSpaceResponse containing the shared object reference.
  spaceResp: CreateSpaceResponse
  // AbortSignal is the signal to abort the operation.
  abortSignal: AbortSignal
  // Cleanup is the function to register cleanup for mounted resources.
  cleanup: RegisterCleanup
}

/**
 * mountSpace mounts a Space resource from a CreateSpaceResponse.
 *
 * @param params - Parameters for mounting the space
 * @returns The mounted Space resource
 */
export async function mountSpace(params: MountSpaceParams): Promise<Space> {
  const { session, spaceResp, abortSignal, cleanup } = params

  // Mount the space as a shared object.
  const sharedObjectId = spaceResp.sharedObjectRef?.providerResourceRef?.id
  const spaceSo = cleanup(
    await session.mountSharedObject({ sharedObjectId }, abortSignal),
  )

  // Mount the shared object body to access space-specific functionality.
  const spaceSoBody = cleanup(
    await spaceSo.mountSharedObjectBody({}, abortSignal),
  )

  // Cast the shared object body to a Space resource.
  return new Space(spaceSoBody.resourceRef)
}
