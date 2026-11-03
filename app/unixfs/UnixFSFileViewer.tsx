import { useCallback, useMemo } from 'react'
import {
  LuArrowRight,
  LuFile,
  LuFileText,
  LuImage,
  LuLink,
  LuMusic,
  LuVideo,
} from '@s4wave/web/images/icons.js'
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
import { UnixFSAudioFileViewer } from './UnixFSAudioFileViewer.js'
import { UnixFSPdfFileViewer } from './UnixFSPdfFileViewer.js'
import { UnixFSVideoFileViewer } from './UnixFSVideoFileViewer.js'
import { UnixFSTextFileViewer } from './UnixFSTextFileViewer.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    minHeight: 0,
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    maxWidth: 'var(--container-xs)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule5: { minWidth: 0 },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule8: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: themeLayout['--spacing'],
  },
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    overflow: 'auto',
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    minHeight: 0,
  },
  rule10: { objectFit: 'contain', maxHeight: '100%', maxWidth: '100%' },
  rule11: { flex: 1, minWidth: 0 },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule13: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    marginTop: themeLayout['--spacing'],
  },
  rule14: {
    display: 'flex',
    justifyContent: 'flex-end',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule15: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule16: {
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule17: {
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-file-back'],
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
  sx,
}: {
  mimeType: string
  sx?: import('@stylexjs/stylex').StyleXStyles
}) {
  const cls = sx ?? fileIconStyles.default

  if (isTextMimeType(mimeType)) {
    return <LuFileText sx={cls} />
  }
  if (isImageMimeType(mimeType)) {
    return <LuImage sx={cls} />
  }
  if (isAudioMimeType(mimeType)) {
    return <LuMusic sx={cls} />
  }
  if (isVideoMimeType(mimeType)) {
    return <LuVideo sx={cls} />
  }
  return <LuFile sx={cls} />
}

// BinaryFileViewer displays a placeholder for binary files.
function BinaryFileViewer({ mimeType }: { mimeType: string }) {
  return (
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <span sx={stylexStyles3.rule4}>
            <FileIcon mimeType={mimeType} sx={stylexStyles2.rule1} />
          </span>
          <div sx={stylexStyles3.rule5}>
            <p sx={stylexStyles3.rule6}>Preview not available</p>
            <p sx={stylexStyles3.rule7}>
              This file type can't be rendered inline. Download it to open in
              another app.
            </p>
            <p sx={stylexStyles3.rule8}>{mimeType}</p>
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
    <div sx={stylexStyles3.rule9}>
      <img alt={alt} sx={stylexStyles3.rule10} src={inlineFileURL} />
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
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <span sx={stylexStyles3.rule4}>
            <LuLink sx={stylexStyles.rule1} />
          </span>
          <div sx={stylexStyles3.rule11}>
            <p sx={stylexStyles3.rule6}>Symbolic link</p>
            {loading ? (
              <p sx={stylexStyles3.rule12}>Reading target…</p>
            ) : (
              <p sx={stylexStyles3.rule13}>{target}</p>
            )}
          </div>
        </div>
        {!loading && onNavigate && (
          <div sx={stylexStyles3.rule14}>
            <button
              type="button"
              onClick={onNavigate}
              sx={stylexStyles3.rule15}
            >
              Go to target
              <LuArrowRight sx={stylexStyles.rule2} />
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
    <div data-testid="unixfs-browser" sx={stylexStyles3.rule16}>
      {!hideToolbar && <FileToolbar path={path} />}

      {/* File content */}
      <div sx={stylexStyles3.rule17}>
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

const fileIconStyles = stylex.create({
  default: {
    color: themeColors['--color-foreground-alt'],
    height: 16,
    width: 16,
  },
})
