import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { applySpaceIndexPath } from '../space/space-settings.js'

// WizardRef names a wizard object in a Space World with the Space index the
// caller last read. A useWizardState result satisfies it, and creation code
// that runs outside React builds one from its request.
export interface WizardRef {
  spaceWorld: IWorldState
  objectKey: string
  sessionPeerId: string
  spaceSettings?: { indexPath?: string }
}

// deleteWizard removes a finished or cancelled wizard. A Space index that
// names the wizard first moves to indexPath, or is cleared when indexPath is
// empty, so the index never names a deleted object: a failure between the two
// writes leaves the wizard in place. The index write carries the index the
// caller read as its expected path, so the World refuses it, without error,
// when another writer changed the index since. A wizard already deleted by
// another participant counts as deleted.
export async function deleteWizard(
  wizard: WizardRef,
  indexPath: string,
  signal?: AbortSignal,
) {
  const current = wizard.spaceSettings?.indexPath ?? ''
  if (parseObjectUri(current).objectKey === wizard.objectKey) {
    await applySpaceIndexPath(wizard.spaceWorld, indexPath, {
      sender: wizard.sessionPeerId,
      expectedIndexPath: current,
      signal,
    })
  }
  try {
    await (signal
      ? wizard.spaceWorld.deleteObject(wizard.objectKey, signal)
      : wizard.spaceWorld.deleteObject(wizard.objectKey))
  } catch (err) {
    if (!isObjectNotFoundError(err)) throw err
  }
}

// isObjectNotFoundError matches the world store's object-not-found error
// (db/world/errors.go) both as a local Error and as a starpc RemoteRPCError
// crossing the RPC boundary, where only the message string survives.
function isObjectNotFoundError(err: unknown) {
  if (!(err instanceof Error)) return false
  if ('rpcError' in err && err.rpcError === 'object not found') return true
  return err.message === 'object not found'
}
