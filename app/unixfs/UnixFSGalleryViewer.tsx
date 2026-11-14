import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import 'react-photo-view/dist/react-photo-view.css'

import { useCallback, useMemo, useState, type MouseEventHandler } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuDownload, LuExternalLink, LuImage } from 'react-icons/lu'
import { PhotoSlider } from 'react-photo-view'
import { List, type RowComponentProps } from 'react-window'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
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
import { viewerFloatPressClass } from '@s4wave/web/object/ViewerFloat.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
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

export interface GalleryPreviewItem {
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
            <LuImage aria-hidden className="size-8" />
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
        <div className="text-foreground-alt truncate text-xs">
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

  // The focus ring sits inside the tile: the list's scroll box clips
  // anything outside the first row and column.
  return (
    <button
      data-testid="unixfs-gallery-item"
      type="button"
      aria-label={`Open ${item.label}`}
      className={cn(
        className,
        'focus-ring cursor-zoom-in text-left focus-visible:-outline-offset-2',
      )}
      onClick={onClick}
    >
      {body}
    </button>
  )
}

// lightboxButtonClassName draws a lightbox press as a viewer float press,
// light on the lightbox's dark backdrop.
const lightboxButtonClassName = cn(
  viewerFloatPressClass,
  'w-viewer-press px-0 text-white hover:bg-white/15 pointer-coarse:min-w-11',
)

export interface LightboxSource {
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
    <div className="mr-2 flex items-center gap-0.5">
      <button
        type="button"
        className={lightboxButtonClassName}
        onClick={() => window.open(previewURL, '_blank', 'noopener,noreferrer')}
        aria-label="Open in browser"
        title="Open in browser"
      >
        <LuExternalLink aria-hidden />
      </button>
      <button
        type="button"
        className={lightboxButtonClassName}
        onClick={handleDownload}
        aria-label="Download"
        title="Download"
      >
        <LuDownload aria-hidden />
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

/**
 * galleryPreviewItems maps the index's images to tiles, each with an inline
 * preview URL when the pane has a session and Space to read it through.
 * Session indexes start at 1, so 0 is the context's "no session".
 */
export function galleryPreviewItems(
  items: UnixFSGalleryCandidate[],
  { spaceId, sessionIndex, unixfsId, httpPathPrefix }: LightboxSource,
): GalleryPreviewItem[] {
  return items.map((item) => ({
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
  }))
}

// galleryWatchOptions reopens the index watch after a failure and keeps the
// last index, so the tiles found stay pressable while it reads again and
// after it fails past its backoff.
const galleryWatchOptions = { reconnect: true }

interface GalleryScan {
  galleryState: Resource<UnixFSGalleryDiscoveryState>
  previewItems: GalleryPreviewItem[]
  lightboxItems: GalleryPreviewItem[]
  scopePath: string
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
      galleryWatchOptions,
    )
  const index = resourceContent(galleryState)
  const galleryItems = index?.items ?? emptyGalleryItems
  const previewItems: GalleryPreviewItem[] = useMemo(
    () =>
      galleryPreviewItems(galleryItems, {
        spaceId,
        sessionIndex,
        unixfsId,
        httpPathPrefix,
      }),
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
    scopePath: index?.scopePath ?? currentPath,
    spaceId,
    sessionIndex,
    httpPathPrefix,
  }
}

// galleryNoteClass is a muted note row above the tiles, padded as the bar.
const galleryNoteClass =
  'min-h-viewer-fact-row text-foreground-alt/70 viewer-inset-x flex shrink-0 items-center text-xs'

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

export interface GalleryBodyProps {
  galleryState: Resource<UnixFSGalleryDiscoveryState>
  items: GalleryPreviewItem[]
  scopePath: string
  onOpen: (path: string) => void
}

/**
 * GalleryBody draws the states of UnixFSGalleryViewer.tla: the wait or the
 * failure until the first index, then the tiles, or a row saying the index
 * holds no image. An index once arrived stays while the watch reads again,
 * and under the watch's failure once it gives up backing off, so the tiles
 * found stay pressable; a failed index never says it holds no image. Notes
 * above the tiles say the index stopped
 * at its limit, some folders could not be read, or previews need a session
 * and a Space: a tile opens only with a preview to show.
 */
export function GalleryBody({
  galleryState,
  items,
  scopePath,
  onOpen,
}: GalleryBodyProps) {
  const index = resourceContent(galleryState)
  if (!index) {
    if (galleryState.error) {
      return (
        <ViewerStatus
          state="failed"
          message={`Could not read the images under ${scopePath}: ${galleryState.error.message}`}
          retry={galleryState.retry}
        />
      )
    }
    return (
      <ViewerStatus
        state="waiting"
        what={`the images under ${scopePath}`}
        rows={0}
      />
    )
  }

  return (
    <>
      {galleryState.error && (
        <ViewerStatus
          state="failed"
          message={`Could not read the images under ${scopePath}: ${galleryState.error.message}`}
          retry={galleryState.retry}
        />
      )}
      {galleryState.loading && (
        <ViewerStatus
          state="waiting"
          what={`the images under ${scopePath} again`}
          rows={0}
        />
      )}
      {items.length > 0 && (
        <p className={galleryNoteClass}>
          <span className="truncate">
            {items.length} image{items.length === 1 ? '' : 's'} under{' '}
            {scopePath}
          </span>
        </p>
      )}
      {index.truncated && (
        <p className={galleryNoteClass}>
          Showing the first {items.length} images. Open a folder inside to see
          the rest.
        </p>
      )}
      {index.errors.length > 0 && (
        <p className={galleryNoteClass}>
          Some folders could not be read; the images found are shown.
        </p>
      )}
      {items.some((item) => !item.previewURL) && (
        <p className={galleryNoteClass}>
          Previews need a session; open this folder from a Space to see them.
        </p>
      )}
      {items.length === 0 ? (
        !galleryState.error && (
          <ViewerStatus
            state="empty"
            icon={<LuImage />}
            message="No images under this path."
          />
        )
      ) : (
        <div className="viewer-inset-x min-h-0 flex-1 pt-1">
          <GalleryGrid items={items} onOpen={onOpen} />
        </div>
      )}
    </>
  )
}

/**
 * UnixFSGalleryBody gives each path its own gallery, so a new path starts
 * from waiting rather than drawing the last path's tiles, and its first
 * failure is reported rather than retried as a reconnect.
 */
function UnixFSGalleryBody(props: UnixFSBrowserBodyProps) {
  return <UnixFSGalleryScope key={props.currentPath} {...props} />
}

/**
 * UnixFSGalleryScope shows the image index of one path as a gallery grid,
 * and opens a lightbox over the images with a preview.
 */
function UnixFSGalleryScope({
  rootHandle,
  currentPath,
  unixfsId,
}: UnixFSBrowserBodyProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(
    null,
  )
  const [lightboxPath, setLightboxPath] = useState<string | null>(null)
  const scan = useGalleryScan(rootHandle, currentPath, unixfsId)
  const { galleryState, previewItems, lightboxItems } = scan
  const { spaceId, sessionIndex, httpPathPrefix } = scan
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

  const body = (
    <GalleryBody
      galleryState={galleryState}
      items={previewItems}
      scopePath={scan.scopePath}
      onOpen={setLightboxPath}
    />
  )

  return (
    <div
      data-testid="unixfs-gallery-viewer"
      ref={handlePortalContainer}
      className="relative flex h-full w-full flex-col"
    >
      {body}
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
