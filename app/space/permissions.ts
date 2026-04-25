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
