import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

// accountCreatedLabel formats a creation time to the minute, so two accounts
// created on the same day still read apart.
function accountCreatedLabel(createdAt: bigint | number): string {
  return new Date(Number(createdAt)).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

// accountName returns the name a person gave the account, or its cloud
// username, or an empty string for an unnamed local account.
export function accountName(
  metadata: SessionMetadata | null | undefined,
): string {
  return metadata?.displayName || metadata?.cloudEntityId || ''
}

// accountTitle returns the account label shown by selection surfaces. An
// unnamed local account is labeled by when it was created.
export function accountTitle(
  metadata: SessionMetadata | null | undefined,
  sessionIndex: number,
): string {
  const name = accountName(metadata)
  if (name) return name
  if (metadata?.createdAt) {
    return `Local account, created ${accountCreatedLabel(metadata.createdAt)}`
  }
  return `Session ${sessionIndex}`
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
  if (!isCloudProvider && !accountName(metadata)) {
    const accountId = metadata?.providerAccountId ?? ''
    return [providerLabel, `Session ${sessionIndex}`, accountId.slice(-6)]
      .filter(Boolean)
      .join(' · ')
  }
  return providerLabel
}
