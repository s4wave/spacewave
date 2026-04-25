import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { StatResult } from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { UnixFSFileViewer } from '@s4wave/app/unixfs/UnixFSFileViewer.js'

// FileViewerProps are props for the FileViewer component.
export interface FileViewerProps {
  path: string
  stat: StatResult
  rootHandle: Resource<FSHandle>
  inlineFileURL?: string
}

// FileViewer delegates to UnixFSFileViewer for rendering file content.
// Hides the built-in toolbar since GitToolbar provides navigation.
export function FileViewer({
  path,
  stat,
  rootHandle,
  inlineFileURL,
}: FileViewerProps) {
  return (
    <UnixFSFileViewer
      path={path}
      stat={stat}
      rootHandle={rootHandle}
      hideToolbar
      inlineFileURL={inlineFileURL}
    />
  )
}
