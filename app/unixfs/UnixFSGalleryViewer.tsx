import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import 'react-photo-view/dist/react-photo-view.css'

import { useCallback, useMemo, useState, type MouseEventHandler } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  LuDownload,
  LuExternalLink,
  LuImage,
} from '@s4wave/web/images/icons.js'
import { PhotoSlider } from 'react-photo-view'
import { List, type RowComponentProps } from 'react-window'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { usePath } from '@s4wave/web/router/router.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import { downloadURL } from '@s4wave/web/download.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { joinUnixFSDisplayPath } from '@s4wave/sdk/unixfs/path.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import {
  buildUnixFSFileDownloadURL,
  buildUnixFSFileInlineURL,
} from './download.js'
import {
  type UnixFSGalleryCandidate,
  type UnixFSGalleryDiscoveryState,
  streamUnixFSGalleryCandidates,
} from './gallery.js'
import { UnixFSBrowser, type UnixFSBrowserBodyProps } from './UnixFSBrowser.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    height: `calc(${themeLayout['--spacing']} * 14)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    overflow: 'hidden',
    aspectRatio: 1,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule2: { objectFit: 'cover', height: '100%', width: '100%' },
  rule3: {
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule6: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
  rule7: { cursor: 'zoom-in', textAlign: 'left' },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 20%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-white'],
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule10: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    minHeight: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule11: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    minHeight: `calc(${themeLayout['--spacing']} * 48)`,
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'center',
    maxWidth: 'var(--container-xs)',
  },
  rule17: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule18: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(var(--gallery-columns), minmax(0, 1fr))',
  },
  rule19: { height: '100%' },
  rule20: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    height: '100%',
    width: '100%',
  },
  rule21: {
    borderColor: 'color-mix(in oklab, currentcolor 20%, transparent)',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: 'color-mix(in oklab, currentcolor 10%, transparent)',
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule23: { flex: 1, minHeight: 0 },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const emptyGalleryItems: UnixFSGalleryCandidate[] = []

// galleryTileMinWidth is the narrowest a tile may be before the grid drops a
// column, in pixels.
const galleryTileMinWidth = 192

// galleryGap is the space between tiles and between rows, in pixels. It
// matches the gap-3 class of each row.
const galleryGap = 12

// galleryCaptionHeight is the height of the caption under each square
// preview, in pixels. It matches the h-14 class of the caption.
const galleryCaptionHeight = 56

interface GalleryPreviewItem {
  path: string
  name: string
  label: string
  mimeType: string
  previewURL?: string
}

function GalleryTile({
  interactive,
  item,
  onClick,
}: {
  interactive: boolean
  item: GalleryPreviewItem
  onClick?: MouseEventHandler<HTMLButtonElement>
}) {
  const body = (
    <>
      <div sx={stylexStyles2.rule1}>
        {item.previewURL ? (
          <img
            alt={item.label}
            sx={stylexStyles2.rule2}
            loading="lazy"
            src={item.previewURL}
          />
        ) : (
          <div sx={stylexStyles2.rule3}>
            <LuImage sx={stylexStyles.rule1} />
          </div>
        )}
      </div>
      <div sx={stylexStyles3.rule1}>
        <div
          sx={[nativeChildSpacing.s1, stylexStyles2.rule4]}
          title={item.label}
        >
          {item.label}
        </div>
        <div sx={[nativeChildSpacing.s1, stylexStyles2.rule5]}>
          {item.mimeType}
        </div>
      </div>
    </>
  )

  if (!interactive) {
    return (
      <div data-testid="unixfs-gallery-item" sx={stylexStyles2.rule6}>
        {body}
      </div>
    )
  }

  return (
    <button
      data-testid="unixfs-gallery-item"
      type="button"
      sx={[stylexStyles2.rule6, stylexStyles2.rule7]}
      onClick={onClick}
    >
      {body}
    </button>
  )
}

interface LightboxSource {
  spaceId: string | null
  sessionIndex: number | null
  unixfsId: string
  httpPathPrefix: string
}

interface LightboxToolbarProps {
  item: GalleryPreviewItem
  previewURL: string
  downloadURL: string
}

/** LightboxToolbar renders the open-in-browser and download actions for one lightbox image. */
function LightboxToolbar({
  item,
  previewURL,
  downloadURL: fileDownloadURL,
}: LightboxToolbarProps) {
  const handleDownload = () => {
    void downloadURL(fileDownloadURL, item.name).catch((err: unknown) => {
      console.error('failed to download unixfs file', err)
      toast.error('Download failed', { description: String(err) })
    })
  }

  return (
    <div sx={stylexStyles2.rule8}>
      <button
        type="button"
        sx={stylexStyles2.rule9}
        onClick={() => window.open(previewURL, '_blank', 'noopener,noreferrer')}
        title="Open In Browser"
      >
        <LuExternalLink sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule9}
        onClick={handleDownload}
        title="Download"
      >
        <LuDownload sx={stylexStyles.rule2} />
      </button>
    </div>
  )
}

/**
 * lightboxToolbarRender builds the PhotoSlider toolbar renderer for the
 * lightbox items. It renders nothing for an item without a mounted preview.
 */
function lightboxToolbarRender(
  items: GalleryPreviewItem[],
  { spaceId, sessionIndex, unixfsId, httpPathPrefix }: LightboxSource,
) {
  return ({ index }: { index: number }) => {
    const item = items[index]
    if (!item?.previewURL || !spaceId || !sessionIndex) return null
    return (
      <LightboxToolbar
        item={item}
        previewURL={item.previewURL}
        downloadURL={buildUnixFSFileDownloadURL(
          sessionIndex,
          spaceId,
          unixfsId,
          item.path,
          httpPathPrefix,
        )}
      />
    )
  }
}

interface GalleryScan {
  galleryState: Resource<UnixFSGalleryDiscoveryState>
  previewItems: GalleryPreviewItem[]
  lightboxItems: GalleryPreviewItem[]
  scopePath: string
  isScanning: boolean
  spaceId: string | null
  sessionIndex: number | null
  httpPathPrefix: string
}

/**
 * useGalleryScan streams the image candidates under the current path and maps
 * them to preview items with inline URLs from the mounted session.
 */
function useGalleryScan(
  rootHandle: UnixFSBrowserBodyProps['rootHandle'],
  currentPath: string,
  unixfsId: string,
): GalleryScan {
  const { httpPathPrefix } = useAppEnvironment()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const sessionIndex = useSessionIndex()
  const spaceId = spaceCtx?.spaceId ?? null
  const galleryState: Resource<UnixFSGalleryDiscoveryState> =
    useStreamingResource(
      rootHandle,
      useCallback(
        (handle, signal): AsyncIterable<UnixFSGalleryDiscoveryState> =>
          streamUnixFSGalleryCandidates(handle, currentPath, signal),
        [currentPath],
      ),
      [currentPath],
    )
  const galleryItems = galleryState.value?.items ?? emptyGalleryItems
  const previewItems: GalleryPreviewItem[] = useMemo(
    () =>
      galleryItems.map((item) => ({
        path: item.path,
        name: item.name,
        label: item.label,
        mimeType: item.mimeType,
        previewURL:
          !sessionIndex || !spaceId
            ? undefined
            : buildUnixFSFileInlineURL(
                sessionIndex,
                spaceId,
                unixfsId,
                item.path,
                httpPathPrefix,
              ),
      })),
    [galleryItems, sessionIndex, spaceId, unixfsId, httpPathPrefix],
  )
  const lightboxItems = useMemo(
    () => previewItems.filter((item) => !!item.previewURL),
    [previewItems],
  )

  return {
    galleryState,
    previewItems,
    lightboxItems,
    scopePath: galleryState.value?.scopePath ?? currentPath,
    isScanning: !galleryState.value && !galleryState.error,
    spaceId,
    sessionIndex,
    httpPathPrefix,
  }
}

interface GalleryHeaderProps {
  imageCount: number
  scopePath: string
  issueCount: number
  isScanning: boolean
}

/** GalleryHeader summarizes the discovered image count, scan issues, and scan progress. */
function GalleryHeader({
  imageCount,
  scopePath,
  issueCount,
  isScanning,
}: GalleryHeaderProps) {
  return (
    <div sx={stylexStyles2.rule10}>
      <div sx={stylexStyles2.rule11}>
        {imageCount} image
        {imageCount === 1 ? '' : 's'} under {scopePath}
      </div>
      <div sx={stylexStyles2.rule12}>
        {issueCount > 0 && (
          <div sx={stylexStyles2.rule13}>
            {issueCount} issue
            {issueCount === 1 ? '' : 's'}
          </div>
        )}
        {isScanning && (
          <div sx={stylexStyles2.rule14}>
            <Spinner size="sm" />
            Scanning
          </div>
        )}
      </div>
    </div>
  )
}

/** GalleryEmptyState explains an empty gallery: still scanning, or no images found. */
function GalleryEmptyState({ isScanning }: { isScanning: boolean }) {
  return (
    <div sx={stylexStyles2.rule15}>
      <div sx={stylexStyles2.rule16}>
        <LuImage sx={stylexStyles.rule3} />
        <div sx={stylexStyles2.rule17}>
          {isScanning ? 'Scanning for images' : 'No images under this path'}
        </div>
      </div>
    </div>
  )
}

interface GalleryRowProps {
  items: GalleryPreviewItem[]
  columns: number
  onOpen: (path: string) => void
}

/** GalleryRow renders one row of tiles of the virtualized gallery grid. */
function GalleryRow({
  index,
  style,
  ariaAttributes,
  items,
  columns,
  onOpen,
}: RowComponentProps<GalleryRowProps>) {
  const rowItems = items.slice(index * columns, (index + 1) * columns)
  return (
    <div style={style} {...ariaAttributes}>
      <div sx={stylexStyles2.rule18} style={{ '--gallery-columns': columns }}>
        {rowItems.map((item) => (
          <GalleryTile
            key={item.path}
            interactive={!!item.previewURL}
            item={item}
            onClick={() => onOpen(item.path)}
          />
        ))}
      </div>
    </div>
  )
}

/**
 * GalleryGrid lays out the preview tiles in rows that fit the measured width,
 * and renders only the rows near the visible part of the gallery.
 */
function GalleryGrid({
  items,
  onOpen,
}: {
  items: GalleryPreviewItem[]
  onOpen: (path: string) => void
}) {
  const [width, setWidth] = useState(0)
  const columns = Math.max(
    1,
    Math.floor((width + galleryGap) / (galleryTileMinWidth + galleryGap)),
  )
  const tileWidth =
    width > 0
      ? (width - galleryGap * (columns - 1)) / columns
      : galleryTileMinWidth
  const rowProps = useMemo(
    () => ({ items, columns, onOpen }),
    [items, columns, onOpen],
  )

  return (
    <List
      data-testid="unixfs-gallery-grid"
      {...stylex.props(stylexStyles2.rule19)}
      data-columns={columns}
      defaultHeight={600}
      onResize={(size) => setWidth(size.width)}
      overscanCount={2}
      rowComponent={GalleryRow}
      rowCount={Math.ceil(items.length / columns)}
      rowHeight={Math.ceil(tileWidth + galleryCaptionHeight + galleryGap)}
      rowProps={rowProps}
    />
  )
}

/**
 * UnixFSGalleryBody shows the image index of the current path as a gallery
 * grid, and opens a lightbox over the images with a preview.
 */
function UnixFSGalleryBody({
  rootHandle,
  currentPath,
  unixfsId,
}: UnixFSBrowserBodyProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(
    null,
  )
  const [lightboxPath, setLightboxPath] = useState<string | null>(null)
  const scan = useGalleryScan(rootHandle, currentPath, unixfsId)
  const { galleryState, previewItems, lightboxItems, isScanning } = scan
  const { spaceId, sessionIndex, httpPathPrefix } = scan
  const galleryErrors = galleryState.value?.errors ?? []
  const hasItems = previewItems.length > 0
  const handlePortalContainer = useCallback((el: HTMLDivElement | null) => {
    setPortalContainer(el)
  }, [])

  // The lightbox follows the open image by path, so it stays on that image
  // while the index changes and closes when the image leaves.
  const lightboxIndex =
    lightboxPath === null
      ? -1
      : lightboxItems.findIndex((item) => item.path === lightboxPath)
  const lightboxImages = useMemo(
    () =>
      lightboxItems.map((item) => ({ key: item.path, src: item.previewURL })),
    [lightboxItems],
  )
  const handleLightboxIndex = useCallback(
    (index: number) => setLightboxPath(lightboxItems[index]?.path ?? null),
    [lightboxItems],
  )
  const toolbarRender = useMemo(
    () =>
      lightboxToolbarRender(lightboxItems, {
        spaceId,
        sessionIndex,
        unixfsId,
        httpPathPrefix,
      }),
    [lightboxItems, spaceId, sessionIndex, unixfsId, httpPathPrefix],
  )

  return (
    <div
      data-testid="unixfs-gallery-viewer"
      ref={handlePortalContainer}
      sx={stylexStyles2.rule20}
    >
      <GalleryHeader
        imageCount={previewItems.length}
        scopePath={scan.scopePath}
        issueCount={galleryErrors.length}
        isScanning={isScanning}
      />
      {galleryState.error && (
        <div sx={stylexStyles2.rule21}>{galleryState.error.message}</div>
      )}
      {!galleryState.error && galleryState.value?.truncated && (
        <div sx={stylexStyles2.rule22}>
          Showing the first {previewItems.length} images. Open a subdirectory to
          see the rest.
        </div>
      )}
      {!galleryState.error && galleryErrors.length > 0 && (
        <div sx={stylexStyles2.rule22}>
          Some descendants could not be scanned. Discovered images remain
          visible.
        </div>
      )}
      {!galleryState.error && !sessionIndex && (
        <div sx={stylexStyles2.rule22}>
          Inline previews require a mounted session context.
        </div>
      )}
      <div sx={stylexStyles2.rule23}>
        {!galleryState.error && !hasItems && (
          <GalleryEmptyState isScanning={isScanning} />
        )}
        {hasItems && (
          <GalleryGrid items={previewItems} onOpen={setLightboxPath} />
        )}
      </div>
      <PhotoSlider
        className={'unixfs-photo-slider'}
        images={lightboxImages}
        index={Math.max(lightboxIndex, 0)}
        onClose={() => setLightboxPath(null)}
        onIndexChange={handleLightboxIndex}
        portalContainer={portalContainer ?? undefined}
        toolbarRender={toolbarRender}
        visible={lightboxIndex !== -1}
      />
    </div>
  )
}

// UnixFSGalleryViewer renders the UnixFS browser shell with the gallery body in
// the file-list slot.
export function UnixFSGalleryViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const routerPath = usePath()
  const unixfsId = getObjectKey(objectInfo)
  const unixfsInfo =
    objectInfo?.info?.case === 'unixfsObjectInfo' ? objectInfo.info.value : null
  const basePath = unixfsInfo?.path || '/'
  const currentPath = joinUnixFSDisplayPath(basePath, routerPath || '/')

  return (
    <UnixFSBrowser
      unixfsId={unixfsId}
      basePath={basePath}
      currentPath={currentPath}
      mimeTypeOverride={unixfsInfo?.mimeType}
      worldState={worldState}
      browserBody={UnixFSGalleryBody}
    />
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
