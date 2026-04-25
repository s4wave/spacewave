import { usePath } from '@s4wave/web/router/router.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { UnixFSBrowser } from './UnixFSBrowser.js'

// UnixFSTypeID is the type identifier for UnixFS fs-node objects.
export const UnixFSTypeID = 'unixfs/fs-node'

// joinPath joins two path segments.
function joinPath(base: string, rel: string): string {
  if (!rel || rel === '/') return base
  if (base.endsWith('/')) return base + rel.replace(/^\//, '')
  return base + '/' + rel.replace(/^\//, '')
}

// UnixFSViewer renders a UnixFS filesystem object as an ObjectViewer.
export function UnixFSViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const routerPath = usePath()
  const unixfsId = getObjectKey(objectInfo)
  const unixfsInfo =
    objectInfo?.info?.case === 'unixfsObjectInfo' ? objectInfo.info.value : null
  const basePath = unixfsInfo?.path || '/'
  const currentPath = joinPath(basePath, routerPath || '/')
  return (
    <UnixFSBrowser
      unixfsId={unixfsId}
      basePath={basePath}
      currentPath={currentPath}
      mimeTypeOverride={unixfsInfo?.mimeType}
      worldState={worldState}
    />
  )
}
