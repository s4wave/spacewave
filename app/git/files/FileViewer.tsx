import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { StatResult } from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { UnixFSFileViewer } from '@s4wave/app/unixfs/UnixFSFileViewer.js'

// FileViewerProps are props for the FileViewer component.
export interface FileViewerProps {
  path: string
  stat: StatResult
  rootHandle: Resource<FSHandle>
  // objectKey identifies the Git object and tree holding the file.
  objectKey: string
  inlineFileURL?: string
  // readOnly says why the file cannot be written, such as a commit's files.
  readOnly?: string
  // line is the 1-based line to scroll to and highlight.
  line?: number
}

// commitReadOnly is why a file in a commit offers no Edit action.
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
  line,
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
      line={line}
    />
  )
}
