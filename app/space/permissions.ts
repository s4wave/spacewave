import { SOParticipantRole } from '@s4wave/core/sobject/sobject.pb.js'

// canRenameSpace returns true when the current session can rename the space.
export function canRenameSpace(
  providerId: string | undefined,
  canManageSharing: boolean,
): boolean {
  if (providerId === 'local') {
    return true
  }
  if (providerId === 'spacewave') {
    return canManageSharing
  }
  return false
}

// canDeleteSpaceObject returns true when the current session can apply object mutations.
export function canDeleteSpaceObject(
  providerId: string | undefined,
  viewerRole: SOParticipantRole | undefined,
): boolean {
  if (providerId === 'local') {
    return true
  }
  if (providerId !== 'spacewave') {
    return false
  }
  return (
    viewerRole === SOParticipantRole.SOParticipantRole_WRITER ||
    viewerRole === SOParticipantRole.SOParticipantRole_VALIDATOR ||
    viewerRole === SOParticipantRole.SOParticipantRole_OWNER
  )
}
