import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowRight,
  LuDownload,
  LuFile,
  LuFileText,
  LuImage,
  LuLink,
  LuMusic,
  LuScan,
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
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { getUnixFSFileInfoKind } from '@s4wave/sdk/unixfs/file-kind.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import {
  localNavigation,
  useHistory,
} from '@s4wave/web/router/HistoryRouter.js'
import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
import {
  ViewerFloat,
  ViewerFloatLink,
  ViewerFloatPress,
  ViewerStage,
} from '@s4wave/web/object/ViewerFloat.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
import { cn } from '@s4wave/web/style/utils.js'
import { UnixFSAudioFileViewer } from './UnixFSAudioFileViewer.js'
import { UnixFSPdfFileViewer } from './UnixFSPdfFileViewer.js'
import { UnixFSVideoFileViewer } from './UnixFSVideoFileViewer.js'
import { UnixFSTextFileViewer } from './UnixFSTextFileViewer.js'

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

// BinaryFileViewer displays a placeholder for binary files.
function BinaryFileViewer({ mimeType }: { mimeType: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="border-foreground/6 bg-background-card/30 w-full max-w-xs rounded-lg border p-4">
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

// ImageLoad is one load of an image: the address read, the attempt (a Retry
// mounts a fresh img), and whether the browser failed to load or decode it.
interface ImageLoad {
  url?: string
  attempt: number
  failed: boolean
}

// freshImage is the first load of an address.
function freshImage(url?: string): ImageLoad {
  return { url, attempt: 0, failed: false }
}

// ImageFileViewer fills the pane with the image, fitted to it or at its own
// size, with the size press and Download floating over it.
function ImageFileViewer({
  alt,
  inlineFileURL,
}: {
  alt: string
  inlineFileURL?: string
}) {
  // actual draws the image at its own size rather than fitted to the pane.
  const [actual, setActual] = useState(false)

  // image is the load the img shows: its address, its attempt, and whether
  // it failed. Another address starts a fresh load, so an image that failed
  // before loads again when the person comes back to it.
  const [image, setImage] = useState<ImageLoad>(() => freshImage(inlineFileURL))
  if (image.url !== inlineFileURL) {
    setImage(freshImage(inlineFileURL))
  }
  const failed = image.url === inlineFileURL && image.failed

  // handleError fails the load of the address the img still shows.
  const handleError = useCallback(() => {
    setImage((prev) =>
      prev.url === inlineFileURL ? { ...prev, failed: true } : prev,
    )
  }, [inlineFileURL])

  // handleRetry mounts a fresh img, which reads the image again. The browser
  // does not say whether the read or the decode failed, so Retry shows for
  // both; a decode failure fails again and still offers Download.
  const handleRetry = useCallback(() => {
    setImage((prev) => ({ ...prev, attempt: prev.attempt + 1, failed: false }))
  }, [])

  return (
    <>
      {failed && (
        <ViewerStatus
          state="failed"
          message="Could not show the image: this browser could not load or decode it. Download it to open it elsewhere."
          retry={handleRetry}
        />
      )}
      <ViewerStage media={failed ? 'failed' : 'still'}>
        <div
          className={cn(
            'flex min-h-0 flex-1 overflow-auto',
            !actual && 'items-center justify-center p-4',
          )}
        >
          {!failed && (
            <img
              key={image.attempt}
              alt={alt}
              className={cn(
                'shrink-0',
                actual
                  ? 'm-auto max-w-none'
                  : 'max-h-full max-w-full object-contain',
              )}
              src={inlineFileURL}
              onError={handleError}
            />
          )}
        </div>
        <ViewerFloat at="tr" label="Image">
          {!failed && (
            <ViewerFloatPress
              label="Actual size"
              icon={<LuScan aria-hidden />}
              aria-pressed={actual}
              onClick={() => setActual((v) => !v)}
            />
          )}
          {inlineFileURL && (
            <ViewerFloatLink
              label="Download"
              icon={<LuDownload aria-hidden />}
              essential
              href={inlineFileURL}
              download={alt}
            />
          )}
        </ViewerFloat>
      </ViewerStage>
    </>
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
      <div className="border-foreground/6 bg-background-card/30 w-full max-w-xs rounded-lg border p-4">
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

/** FileBodyKind is how the file content is shown; media kinds carry their URL. */
type FileBodyKind =
  | { kind: 'symlink' | 'text' | 'binary' }
  | { kind: 'image' | 'pdf' | 'audio' | 'video'; url: string }

/** fileBodyKindFor picks how the file content is shown. */
function fileBodyKindFor(
  stat: StatResult,
  inlineFileURL: string | undefined,
): FileBodyKind {
  const fileKind = stat.fileKind ?? getUnixFSFileInfoKind(stat.info)
  if (fileKind === 'symlink') return { kind: 'symlink' }

  const { mimeType } = stat
  if (inlineFileURL) {
    if (isImageMimeType(mimeType)) return { kind: 'image', url: inlineFileURL }
    if (mimeType === 'application/pdf') {
      return { kind: 'pdf', url: inlineFileURL }
    }
    if (isAudioMimeType(mimeType)) return { kind: 'audio', url: inlineFileURL }
    if (isVideoMimeType(mimeType)) return { kind: 'video', url: inlineFileURL }
  }
  return { kind: isTextMimeType(mimeType) ? 'text' : 'binary' }
}

/** baseName returns the last path segment, or the fallback for the root. */
function baseName(path: string, fallback: string): string {
  return path.split('/').filter(Boolean).at(-1) ?? fallback
}

/** resolveSymlinkTarget resolves a symlink target against its parent directory. */
function resolveSymlinkTarget(target: string, path: string): string {
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
}

/** SymlinkBody reads the symlink target and navigates to it on request. */
function SymlinkBody({
  rootHandle,
  path,
}: {
  rootHandle: Resource<FSHandle>
  path: string
}) {
  const navigate = useNavigate()
  const symlinkHandle = useUnixFSHandle(rootHandle, path)
  const targetResource = useResource(
    symlinkHandle,
    (h: { readlink: () => Promise<string> }) => h.readlink(),
    [],
  )

  const target = targetResource.value
  const resolvedTarget = useMemo(
    () => (target ? resolveSymlinkTarget(target, path) : null),
    [target, path],
  )

  const handleNavigate = useCallback(() => {
    if (resolvedTarget) {
      navigate(localNavigation({ path: resolvedTarget }))
    }
  }, [resolvedTarget, navigate])

  return (
    <SymlinkViewer
      target={target ?? ''}
      loading={targetResource.loading}
      onNavigate={resolvedTarget ? handleNavigate : undefined}
    />
  )
}

/** FileBody renders the file content for its kind. */
function FileBody({
  path,
  stat,
  rootHandle,
  inlineFileURL,
}: Pick<
  UnixFSFileViewerProps,
  'path' | 'stat' | 'rootHandle' | 'inlineFileURL'
>) {
  const body = fileBodyKindFor(stat, inlineFileURL)

  switch (body.kind) {
    case 'symlink':
      return <SymlinkBody rootHandle={rootHandle} path={path} />
    case 'image':
      return (
        <ImageFileViewer
          alt={baseName(path, 'image')}
          inlineFileURL={body.url}
        />
      )
    case 'pdf':
      return (
        <UnixFSPdfFileViewer
          title={baseName(path, 'pdf')}
          inlineFileURL={body.url}
        />
      )
    case 'audio':
      return (
        <UnixFSAudioFileViewer
          title={baseName(path, 'audio')}
          inlineFileURL={body.url}
        />
      )
    case 'video':
      return (
        <UnixFSVideoFileViewer
          title={baseName(path, 'video')}
          inlineFileURL={body.url}
        />
      )
    case 'text':
      return (
        <UnixFSTextFileViewer
          key={`${rootHandle.value?.id}/${path}`}
          rootHandle={rootHandle}
          path={path}
        />
      )
    case 'binary':
      return <BinaryFileViewer mimeType={stat.mimeType} />
  }
}

/** FileToolbar renders the path toolbar with history and parent navigation. */
function FileToolbar({ path }: { path: string }) {
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

  return (
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
  return (
    <div
      data-testid="unixfs-browser"
      className="flex h-full w-full flex-col overflow-hidden"
    >
      {!hideToolbar && <FileToolbar path={path} />}

      {/* File content */}
      <div className="bg-file-back flex min-h-0 flex-1 flex-col overflow-hidden">
        <FileBody
          path={path}
          stat={stat}
          rootHandle={rootHandle}
          inlineFileURL={inlineFileURL}
        />
      </div>
    </div>
  )
}
