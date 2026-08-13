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
import { getUnixFSFileInfoKind } from '@s4wave/sdk/unixfs/file-kind.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import {
  localNavigation,
  useHistory,
} from '@s4wave/web/router/HistoryRouter.js'
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
function FileIcon({
  mimeType,
  className,
}: {
  mimeType: string
  className?: string
}) {
  const cls = className ?? 'text-foreground-alt size-4'

  if (isTextMimeType(mimeType)) {
    return <LuFileText className={cls} />
  }
  if (isImageMimeType(mimeType)) {
    return <LuImage className={cls} />
  }
  if (isAudioMimeType(mimeType)) {
    return <LuMusic className={cls} />
  }
  if (isVideoMimeType(mimeType)) {
    return <LuVideo className={cls} />
  }
  return <LuFile className={cls} />
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
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="border-foreground/6 bg-background-card/30 w-full max-w-xs rounded-lg border p-4 backdrop-blur-sm">
        <div className="flex items-start gap-2.5">
          <span className="bg-foreground/5 flex size-8 shrink-0 items-center justify-center rounded-md">
            <FileIcon
              mimeType={mimeType}
              className="text-foreground-alt/70 size-4"
            />
          </span>
          <div className="min-w-0">
            <p className="text-foreground text-xs font-medium select-none">
              Preview not available
            </p>
            <p className="text-foreground-alt/60 mt-0.5 text-xs leading-relaxed">
              This file type can't be rendered inline. Download it to open in
              another app.
            </p>
            <p className="text-foreground-alt/40 mt-1 font-mono text-xs">
              {mimeType}
            </p>
          </div>
        </div>
      </div>
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
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="border-foreground/6 bg-background-card/30 w-full max-w-xs rounded-lg border p-4 backdrop-blur-sm">
        <div className="flex items-start gap-2.5">
          <span className="bg-foreground/5 flex size-8 shrink-0 items-center justify-center rounded-md">
            <LuLink className="text-foreground-alt/70 size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="text-foreground text-xs font-medium select-none">
              Symbolic link
            </p>
            {loading ? (
              <p className="text-foreground-alt/60 mt-0.5 text-xs">
                Reading target…
              </p>
            ) : (
              <p className="text-foreground-alt/70 mt-1 truncate font-mono text-xs">
                {target}
              </p>
            )}
          </div>
        </div>
        {!loading && onNavigate && (
          <div className="mt-3 flex justify-end">
            <button
              type="button"
              onClick={onNavigate}
              className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition duration-150"
            >
              Go to target
              <LuArrowRight className="size-3" />
            </button>
          </div>
        )}
      </div>
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
      navigate(localNavigation({ path: newPath }))
    },
    [navigate],
  )

  const fileKind = stat.fileKind ?? getUnixFSFileInfoKind(stat.info)
  const isSymlink = fileKind === 'symlink'
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
      target.startsWith('/') || parent === '/'
        ? []
        : parent.split('/').filter(Boolean)
    ).concat(target.split('/').filter(Boolean))
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
      navigate(localNavigation({ path: resolvedTarget }))
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
        {isSymlink ? (
          <SymlinkViewer
            target={symlinkTargetResource.value ?? ''}
            loading={symlinkTargetResource.loading}
            onNavigate={resolvedTarget ? handleNavigateSymlink : undefined}
          />
        ) : isImage && inlineFileURL ? (
          <ImageFileViewer
            alt={path.split('/').filter(Boolean).at(-1) ?? 'image'}
            inlineFileURL={inlineFileURL}
          />
        ) : isPdf && inlineFileURL ? (
          <UnixFSPdfFileViewer
            title={path.split('/').filter(Boolean).at(-1) ?? 'pdf'}
            inlineFileURL={inlineFileURL}
          />
        ) : isAudio && inlineFileURL ? (
          <UnixFSAudioFileViewer
            title={path.split('/').filter(Boolean).at(-1) ?? 'audio'}
            inlineFileURL={inlineFileURL}
          />
        ) : isVideo && inlineFileURL ? (
          <UnixFSVideoFileViewer
            title={path.split('/').filter(Boolean).at(-1) ?? 'video'}
            inlineFileURL={inlineFileURL}
          />
        ) : isText ? (
          <TextFileViewer rootHandle={rootHandle} path={path} />
        ) : (
          <BinaryFileViewer mimeType={stat.mimeType} />
        )}
      </div>
    </div>
  )
}
