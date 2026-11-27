import { SessionRef } from './session.pb.js'

// buildSessionComponentID builds the component ID for a session.
// The function returns null if there is missing information or sessionRef is
// empty. The function corresponds to BuildSessionComponentID in Go.
export function buildSessionComponentID(
  sessionRef?: SessionRef,
): string | null {
  const providerResourceRef = sessionRef?.providerResourceRef
  const providerId = providerResourceRef?.providerId
  const providerAccountId = providerResourceRef?.providerAccountId
  const sessionId = providerResourceRef?.id

  if (!providerId || !providerAccountId || !sessionId) {
    return null
  }

  return `session/${providerId}/${providerAccountId}/${sessionId}`
}
