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
import { cn } from '@s4wave/web/style/utils.js'
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

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
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
      <div className="bg-foreground/5 aspect-square overflow-hidden">
        {item.previewURL ? (
          <img
            alt={item.label}
            className="h-full w-full object-cover"
            loading="lazy"
            src={item.previewURL}
          />
        ) : (
          <div className="text-foreground-alt/40 flex h-full w-full items-center justify-center">
            <LuImage sx={stylexStyles.rule1} />
          </div>
        )}
      </div>
      <div className="h-14 space-y-1 px-3 py-2 text-left">
        <div
          className="text-foreground truncate text-xs font-medium"
          title={item.label}
        >
          {item.label}
        </div>
        <div className="text-foreground-alt/50 truncate text-xs">
          {item.mimeType}
        </div>
      </div>
    </>
  )
  const className =
    'border-foreground/8 bg-background-card/20 overflow-hidden rounded-lg border'

  if (!interactive) {
    return (
      <div data-testid="unixfs-gallery-item" className={className}>
        {body}
      </div>
    )
  }

  return (
    <button
      data-testid="unixfs-gallery-item"
      type="button"
      className={cn(className, 'cursor-zoom-in text-left')}
      onClick={onClick}
    >
      {body}
    </button>
  )
}

const lightboxButtonClassName =
  'rounded-full border border-white/20 bg-white/10 p-2 text-white transition hover:bg-white/20'

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
    <div className="mr-2 flex items-center gap-2">
      <button
        type="button"
        className={lightboxButtonClassName}
        onClick={() => window.open(previewURL, '_blank', 'noopener,noreferrer')}
        title="Open In Browser"
      >
        <LuExternalLink sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        className={lightboxButtonClassName}
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
    <div className="mb-3 flex min-h-6 items-center justify-between gap-3">
      <div className="text-foreground-alt/60 min-w-0 truncate text-xs">
        {imageCount} image
        {imageCount === 1 ? '' : 's'} under {scopePath}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {issueCount > 0 && (
          <div className="border-destructive/20 bg-destructive/10 text-destructive rounded-full border px-2 py-0.5 text-xs font-medium">
            {issueCount} issue
            {issueCount === 1 ? '' : 's'}
          </div>
        )}
        {isScanning && (
          <div className="border-foreground/10 bg-foreground/5 text-foreground-alt flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium">
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
    <div className="flex h-full min-h-48 items-center justify-center">
      <div className="border-foreground/6 bg-background-card/30 flex max-w-xs flex-col items-center gap-2 rounded-lg border px-4 py-5 text-center">
        <LuImage sx={stylexStyles.rule3} />
        <div className="text-foreground text-sm font-semibold">
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
      <div
        className="grid-cols-gallery grid gap-3"
        style={{ '--gallery-columns': columns }}
      >
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
      className="h-full"
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
      className="relative flex h-full w-full flex-col px-4 py-3"
    >
      <GalleryHeader
        imageCount={previewItems.length}
        scopePath={scan.scopePath}
        issueCount={galleryErrors.length}
        isScanning={isScanning}
      />
      {galleryState.error && (
        <div className="text-destructive rounded-lg border border-current/20 bg-current/10 px-3 py-2 text-xs">
          {galleryState.error.message}
        </div>
      )}
      {!galleryState.error && galleryState.value?.truncated && (
        <div className="text-foreground-alt/60 mb-3 text-xs">
          Showing the first {previewItems.length} images. Open a subdirectory to
          see the rest.
        </div>
      )}
      {!galleryState.error && galleryErrors.length > 0 && (
        <div className="text-foreground-alt/60 mb-3 text-xs">
          Some descendants could not be scanned. Discovered images remain
          visible.
        </div>
      )}
      {!galleryState.error && !sessionIndex && (
        <div className="text-foreground-alt/60 mb-3 text-xs">
          Inline previews require a mounted session context.
        </div>
      )}
      <div className="min-h-0 flex-1">
        {!galleryState.error && !hasItems && (
          <GalleryEmptyState isScanning={isScanning} />
        )}
        {hasItems && (
          <GalleryGrid items={previewItems} onOpen={setLightboxPath} />
        )}
      </div>
      <PhotoSlider
        className="!absolute inset-0 h-full w-full"
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
