import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

// accountTitle returns the account label shown by selection surfaces.
export function accountTitle(
  metadata: SessionMetadata | null | undefined,
  sessionIndex: number,
): string {
  return (
    metadata?.displayName ||
    metadata?.cloudEntityId ||
    `Session ${sessionIndex}`
  )
}

// accountDescription returns the account identity detail shown by selection surfaces.
export function accountDescription(
  metadata: SessionMetadata | null | undefined,
  sessionIndex: number,
): string {
  const isCloudProvider = metadata?.providerId === 'spacewave'
  const providerLabel =
    metadata?.providerDisplayName || (isCloudProvider ? 'Cloud' : 'Local')
  const title = accountTitle(metadata, sessionIndex)

  if (
    isCloudProvider &&
    metadata?.cloudEntityId &&
    metadata.cloudEntityId !== title
  ) {
    return `${providerLabel} · ${metadata.cloudEntityId}`
  }
  if (!isCloudProvider && !metadata?.displayName) {
    return `${providerLabel} · Session ${sessionIndex}`
  }
  return providerLabel
}
