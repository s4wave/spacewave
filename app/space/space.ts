import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Session } from '@s4wave/sdk/session'
import type { CreateSpaceResponse } from '@s4wave/sdk/session/session.pb.js'
import { SharedObject, SharedObjectBody } from '@s4wave/sdk/sobject/sobject.js'
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
  // Phase optionally records the individual mount steps for callers with
  // user-facing progress diagnostics.
  phase?: <T>(name: string, cb: () => Promise<T>) => Promise<T>
}

/**
 * mountSpace mounts a Space resource from a CreateSpaceResponse.
 *
 * @param params - Parameters for mounting the space
 * @returns The mounted Space resource
 */
export async function mountSpace(params: MountSpaceParams): Promise<Space> {
  const { session, spaceResp, abortSignal, cleanup, phase } = params
  const runPhase: NonNullable<MountSpaceParams['phase']> =
    phase ?? ((_name, cb) => cb())

  const mountedSharedObject = spaceResp.mountedSharedObject
  const mountedSharedObjectID = mountedSharedObject?.resourceId ?? 0
  const mountedBodyID = spaceResp.sharedObjectBodyResourceId ?? 0
  if (mountedSharedObject && mountedSharedObjectID && mountedBodyID) {
    const spaceSo = cleanup(
      session.resourceRef.createResource(
        mountedSharedObjectID,
        SharedObject,
        mountedSharedObject,
      ),
    )
    const spaceSoBody = cleanup(
      spaceSo.resourceRef.createResource(mountedBodyID, SharedObjectBody),
    )
    return cleanup(new Space(spaceSoBody.resourceRef.createRef(spaceSoBody.id)))
  }

  // Mount the space as a shared object.
  const sharedObjectId = spaceResp.sharedObjectRef?.providerResourceRef?.id
  const spaceSo = cleanup(
    await runPhase('mount-space-shared-object', () =>
      session.mountSharedObject({ sharedObjectId }, abortSignal),
    ),
  )

  // Mount the shared object body to access space-specific functionality.
  const spaceSoBody = cleanup(
    await runPhase('mount-space-body', () =>
      spaceSo.mountSharedObjectBody({}, abortSignal),
    ),
  )

  // The Space wrapper has an independent lifetime from the SharedObjectBody
  // wrapper, even though both target the same server resource.
  return cleanup(new Space(spaceSoBody.resourceRef.createRef(spaceSoBody.id)))
}
