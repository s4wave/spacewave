import { useCallback } from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { DriveHandle, DriveTypeID } from '@s4wave/sdk/space/drive/drive.js'
import type { Drive } from '@s4wave/sdk/space/drive/drive.pb.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { UnixFSTypeID } from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { usePath } from '@s4wave/web/router/router.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { UnixFSBrowser } from '../unixfs/UnixFSBrowser.js'

export { DriveTypeID }

function joinPath(base: string, rel: string): string {
  if (!rel || rel === '/') return base
  if (base.endsWith('/')) return base + rel.replace(/^\//, '')
  return base + '/' + rel.replace(/^\//, '')
}

function DriveStatusCard({
  title,
  detail,
  state,
}: {
  title: string
  detail: string
  state: 'active' | 'error'
}) {
  return (
    <div className="bg-background-primary flex h-full w-full items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard
          view={{
            state,
            title,
            detail,
          }}
        />
      </div>
    </div>
  )
}

// DriveViewer renders a Drive object as an app-scale file surface.
export function DriveViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const routerPath = usePath()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    DriveHandle,
    DriveTypeID,
  )
  const streamFactory = useCallback(
    (h: DriveHandle, signal: AbortSignal) => h.watchDriveState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Drive | undefined = stateResource.value ?? undefined
  const root = state?.roots?.[0]

  if (!state) {
    return (
      <DriveStatusCard
        state="active"
        title="Loading Drive"
        detail="Reading the Drive state stream."
      />
    )
  }
  if (!root) {
    return (
      <DriveStatusCard
        state="error"
        title="Drive has no storage root"
        detail="Open Space management to inspect or repair this Drive object."
      />
    )
  }
  if (root.rootType !== UnixFSTypeID) {
    return (
      <DriveStatusCard
        state="error"
        title="Unsupported Drive root"
        detail="Open Space management to inspect the backing storage object."
      />
    )
  }

  return (
    <UnixFSBrowser
      unixfsId={root.rootObjectKey ?? ''}
      basePath="/"
      currentPath={joinPath('/', routerPath || '/')}
      worldState={worldState}
    />
  )
}
