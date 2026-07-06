import { createContext, use, useEffect, type ReactNode } from 'react'

import { useUploadManager } from '@s4wave/app/unixfs/useUploadManager.js'
import type { UploadManager } from '@s4wave/app/unixfs/useUploadManager.js'
import { UploadProgressBottomBar } from '@s4wave/app/unixfs/UploadProgressBottomBar.js'

// UPLOAD_CONCURRENCY caps concurrent upload groups per session.
const UPLOAD_CONCURRENCY = 5

// SessionUploadManagerContext holds the one upload manager for a session UI
// tree. It is null outside a provider so presentation-only surfaces (display,
// debug harnesses) that mount a UnixFS browser without a session simply have no
// upload owner rather than crashing.
const SessionUploadManagerContext = createContext<UploadManager | null>(null)

// SessionUploadManagerProvider owns file uploads for a session, above the object
// viewers that start them. Uploads and their feedback survive navigating between
// objects or finishing a wizard because this owner outlives any single viewer;
// in-flight uploads are aborted only on session teardown, when this provider
// unmounts.
export function SessionUploadManagerProvider({
  children,
}: {
  children: ReactNode
}) {
  const manager = useUploadManager(UPLOAD_CONCURRENCY)

  // Abort every in-flight upload on session teardown (provider unmount). Viewer
  // unmount never reaches here, which is the whole point: navigating away must
  // not kill an upload.
  const { cancelAll } = manager
  useEffect(() => cancelAll, [cancelAll])

  return (
    <SessionUploadManagerContext.Provider value={manager}>
      {children}
    </SessionUploadManagerContext.Provider>
  )
}

// useSessionUploadManager returns the session upload manager, or null when
// mounted outside a SessionUploadManagerProvider.
export function useSessionUploadManager(): UploadManager | null {
  return use(SessionUploadManagerContext)
}

// SessionUploadIndicator renders the session's upload progress indicator in the
// bottom bar. It self-hides when no uploads exist, so it can sit permanently in
// the session chrome and show regardless of which object is open.
export function SessionUploadIndicator() {
  const manager = useSessionUploadManager()
  if (!manager) return null
  return <UploadProgressBottomBar uploadManager={manager} />
}
