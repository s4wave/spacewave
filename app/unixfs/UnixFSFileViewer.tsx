import { useCallback, useMemo } from 'react'
import {
  LuArrowRight,
  LuFile,
  LuFileText,
  LuImage,
  LuLink,
  LuMusic,
  LuVideo,
} from 'react-icons/lu'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { StatResult } from '@s4wave/web/hooks/useUnixFSHandle.js'
import {
  isTextMimeType,
  isImageMimeType,
  isAudioMimeType,
  isVideoMimeType,
  useUnixFSHandle,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
import { UnixFSAudioFileViewer } from './UnixFSAudioFileViewer.js'
import { UnixFSPdfFileViewer } from './UnixFSPdfFileViewer.js'
import { UnixFSVideoFileViewer } from './UnixFSVideoFileViewer.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

// UnixFSFileViewerProps are the props passed to the UnixFSFileViewer component.
export interface UnixFSFileViewerProps {
  // path is the file path being viewed.
  path: string
  // stat contains the file stat result with mime type.
  stat: StatResult
  // rootHandle is the root FSHandle resource for reading file content.
  rootHandle: Resource<FSHandle>
  // hideToolbar suppresses the built-in toolbar when an outer component
  // (e.g. GitToolbar) already provides navigation.
  hideToolbar?: boolean
  // inlineFileURL is the projected raw file URL for inline previews.
  inlineFileURL?: string
}

// FileIcon returns the appropriate icon for a mime type.
function FileIcon({ mimeType }: { mimeType: string }) {
  const className = 'h-4 w-4 text-foreground-alt'

  if (isTextMimeType(mimeType)) {
    return <LuFileText className={className} />
  }
  if (isImageMimeType(mimeType)) {
    return <LuImage className={className} />
  }
  if (isAudioMimeType(mimeType)) {
    return <LuMusic className={className} />
  }
  if (isVideoMimeType(mimeType)) {
    return <LuVideo className={className} />
  }
  return <LuFile className={className} />
}

// TextFileViewer displays text file content.
function TextFileViewer({
  rootHandle,
  path,
}: {
  rootHandle: Resource<FSHandle>
  path: string
}) {
  // Get a handle for this specific file path
  const fileHandle = useUnixFSHandle(rootHandle, path)
  const contentResource = useUnixFSHandleTextContent(fileHandle)

  if (contentResource.loading) {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading file',
              detail: 'Reading file content from UnixFS.',
            }}
          />
        </div>
      </div>
    )
  }

  if (contentResource.error) {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'error',
              title: 'Failed to load file',
              error: contentResource.error.message,
              onRetry: contentResource.retry,
            }}
          />
        </div>
      </div>
    )
  }

  if (contentResource.value === null) {
    return null
  }

  return (
    <pre className="text-foreground min-h-0 flex-1 overflow-auto p-4 font-mono text-xs whitespace-pre-wrap">
      {contentResource.value}
    </pre>
  )
}

// BinaryFileViewer displays a placeholder for binary files.
function BinaryFileViewer({ mimeType }: { mimeType: string }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center p-4">
      <FileIcon mimeType={mimeType} />
      <div className="text-foreground-alt mt-4 text-sm">
        Binary file preview not available
      </div>
      <div className="text-foreground-alt/70 mt-1 text-xs">{mimeType}</div>
    </div>
  )
}

function ImageFileViewer({
  alt,
  inlineFileURL,
}: {
  alt: string
  inlineFileURL?: string
}) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto p-4">
      <img
        alt={alt}
        className="max-h-full max-w-full object-contain"
        src={inlineFileURL}
      />
    </div>
  )
}

// GoModeSymlink is Go's os.ModeSymlink bit value.
const GoModeSymlink = 0x08000000

// SymlinkViewer displays the symlink target path with a navigate button.
function SymlinkViewer({
  target,
  loading,
  onNavigate,
}: {
  target: string
  loading: boolean
  onNavigate?: () => void
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center p-4">
      <LuLink className="text-foreground-alt h-5 w-5" />
      <div className="text-foreground-alt mt-4 text-sm">Symbolic link</div>
      {loading ?
        <div className="text-foreground-alt/70 mt-1 text-sm">
          Reading target...
        </div>
      : <>
          <div className="text-foreground mt-1 font-mono text-sm">{target}</div>
          {onNavigate && (
            <button
              className="text-brand mt-3 flex items-center gap-1 text-sm hover:underline"
              onClick={onNavigate}
            >
              Go to target
              <LuArrowRight className="h-3.5 w-3.5" />
            </button>
          )}
        </>
      }
    </div>
  )
}

// UnixFSFileViewer displays file content.
export function UnixFSFileViewer({
  path,
  stat,
  rootHandle,
  hideToolbar,
  inlineFileURL,
}: UnixFSFileViewerProps) {
  const navigate = useNavigate()
  const history = useHistory()

  const handleBack = useCallback(() => {
    history?.goBack()
  }, [history])

  const handleForward = useCallback(() => {
    history?.goForward()
  }, [history])

  const handleUp = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handlePathChange = useCallback(
    (newPath: string) => {
      navigate({ path: newPath })
    },
    [navigate],
  )

  const isSymlink = ((stat.info.mode ?? 0) & GoModeSymlink) !== 0
  const isText = !isSymlink && isTextMimeType(stat.mimeType)
  const isImage = !isSymlink && isImageMimeType(stat.mimeType)
  const isPdf = !isSymlink && stat.mimeType === 'application/pdf'
  const isAudio = !isSymlink && isAudioMimeType(stat.mimeType)
  const isVideo = !isSymlink && isVideoMimeType(stat.mimeType)

  // Read symlink target when viewing a symlink.
  const symlinkHandle = useUnixFSHandle(rootHandle, isSymlink ? path : '')
  const symlinkTargetResource = useResource(
    symlinkHandle,
    async (h: { readlink: () => Promise<string> }) => {
      if (!h || !isSymlink) return null
      return h.readlink()
    },
    [isSymlink],
  )

  // Resolve the symlink target to an absolute path for navigation.
  const resolvedTarget = useMemo(() => {
    const target = symlinkTargetResource.value
    if (!target) return null
    // Resolve relative target against the symlink's parent directory.
    const parent = path.replace(/\/[^/]*$/, '') || '/'
    const parts = (
      parent === '/' ?
        []
      : parent.split('/').filter(Boolean)).concat(target.split('/'))
    const resolved: string[] = []
    for (const part of parts) {
      if (part === '..') {
        resolved.pop()
      } else if (part !== '.') {
        resolved.push(part)
      }
    }
    return '/' + resolved.join('/')
  }, [symlinkTargetResource.value, path])

  const handleNavigateSymlink = useCallback(() => {
    if (resolvedTarget) {
      navigate({ path: resolvedTarget })
    }
  }, [resolvedTarget, navigate])

  return (
    <div
      data-testid="unixfs-browser"
      className="flex h-full w-full flex-col overflow-hidden"
    >
      {!hideToolbar && (
        <Toolbar
          currentPath={path}
          onPathChange={handlePathChange}
          onNavigate={handlePathChange}
          onBack={handleBack}
          onForward={handleForward}
          onUp={handleUp}
          canGoBack={history?.canGoBack ?? false}
          canGoForward={history?.canGoForward ?? false}
          canGoUp={path !== '/'}
        />
      )}

      {/* File content */}
      <div className="bg-file-back flex min-h-0 flex-1 flex-col overflow-hidden">
        {isSymlink ?
          <SymlinkViewer
            target={symlinkTargetResource.value ?? ''}
            loading={symlinkTargetResource.loading}
            onNavigate={resolvedTarget ? handleNavigateSymlink : undefined}
          />
        : isImage && inlineFileURL ?
          <ImageFileViewer
            alt={path.split('/').filter(Boolean).at(-1) ?? 'image'}
            inlineFileURL={inlineFileURL}
          />
        : isPdf && inlineFileURL ?
          <UnixFSPdfFileViewer
            title={path.split('/').filter(Boolean).at(-1) ?? 'pdf'}
            inlineFileURL={inlineFileURL}
          />
        : isAudio && inlineFileURL ?
          <UnixFSAudioFileViewer
            title={path.split('/').filter(Boolean).at(-1) ?? 'audio'}
            inlineFileURL={inlineFileURL}
          />
        : isVideo && inlineFileURL ?
          <UnixFSVideoFileViewer
            title={path.split('/').filter(Boolean).at(-1) ?? 'video'}
            inlineFileURL={inlineFileURL}
          />
        : isText ?
          <TextFileViewer rootHandle={rootHandle} path={path} />
        : <BinaryFileViewer mimeType={stat.mimeType} />}
      </div>
    </div>
  )
}
