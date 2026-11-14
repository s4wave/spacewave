import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { StatResult } from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { UnixFSFileViewer } from '@s4wave/app/unixfs/UnixFSFileViewer.js'

// FileViewerProps are props for the FileViewer component.
export interface FileViewerProps {
  path: string
  stat: StatResult
  rootHandle: Resource<FSHandle>
  // objectKey names the Git object and tree holding the file.
  objectKey: string
  inlineFileURL?: string
  // readOnly says why the file cannot be written, such as a commit's files.
  readOnly?: string
}

// commitReadOnly is why a file in a commit offers no Edit.
export const commitReadOnly = 'Read-only: a file in a commit'

// FileViewer delegates to UnixFSFileViewer for rendering file content.
// Hides the built-in toolbar since GitToolbar provides navigation.
export function FileViewer({
  path,
  stat,
  rootHandle,
  objectKey,
  inlineFileURL,
  readOnly,
}: FileViewerProps) {
  return (
    <UnixFSFileViewer
      path={path}
      stat={stat}
      rootHandle={rootHandle}
      objectKey={objectKey}
      hideToolbar
      inlineFileURL={inlineFileURL}
      readOnly={readOnly}
    />
  )
}
