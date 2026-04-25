import type { Root } from '@s4wave/sdk/root/root.js'

type SessionLookupRoot = Pick<Root, 'listSessions' | 'getSessionMetadata'>

// findExistingSessionIndexByUsername returns the earliest mounted session whose
// cloud username matches the requested launch username.
export async function findExistingSessionIndexByUsername(
  root: SessionLookupRoot,
  username: string,
  abortSignal?: AbortSignal,
): Promise<number | null> {
  const resp = await root.listSessions(abortSignal)
  let existingSessionIndex: number | null = null
  for (const session of resp.sessions ?? []) {
    const sessionIndex = session.sessionIndex ?? 0
    if (!sessionIndex) continue
    const metadataResp = await root.getSessionMetadata(
      sessionIndex,
      abortSignal,
    )
    if ((metadataResp.metadata?.cloudEntityId ?? '') !== username) {
      continue
    }
    if (existingSessionIndex == null || sessionIndex < existingSessionIndex) {
      existingSessionIndex = sessionIndex
    }
  }
  return existingSessionIndex
}
