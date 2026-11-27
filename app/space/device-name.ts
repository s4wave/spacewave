import type { SpaceSharingState } from '@s4wave/sdk/space/space.pb.js'

// deviceName identifies the member whose device holds peerID, or returns
// fallback when no participant lists that peer.
export function deviceName(
  state: SpaceSharingState,
  peerID: string,
  fallback: string,
): string {
  const info = state.participantInfo?.find((p) => p.peerIds?.includes(peerID))
  if (!info) return fallback
  if (info.isSelf) return 'Your other device'
  return `${info.entityId || info.accountId || 'A member'}'s device`
}
