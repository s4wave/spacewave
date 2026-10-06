import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import 'react-photo-view/dist/react-photo-view.css'

import { useCallback, useMemo, useState, type MouseEventHandler } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuDownload, LuExternalLink, LuImage } from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { PhotoProvider, PhotoView } from 'react-photo-view'
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

const emptyGalleryItems: UnixFSGalleryCandidate[] = []

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
            <LuImage className="size-8" />
          </div>
        )}
      </div>
      <div className="space-y-1 px-3 py-2 text-left">
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
        <LuExternalLink className="size-4" />
      </button>
      <button
        type="button"
        className={lightboxButtonClassName}
        onClick={handleDownload}
        title="Download"
      >
        <LuDownload className="size-4" />
      </button>
    </div>
  )
}

/**
 * lightboxToolbarRender builds the PhotoProvider toolbar renderer for the
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
    isScanning: !(galleryState.value?.complete ?? false) && !galleryState.error,
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
        <LuImage className="text-foreground-alt size-5" />
        <div className="text-foreground text-sm font-semibold">
          {isScanning ? 'Scanning for images' : 'No images under this path'}
        </div>
        {isScanning && (
          <div className="text-foreground-alt text-xs">
            The gallery will populate as image files are discovered in this
            subtree.
          </div>
        )}
      </div>
    </div>
  )
}

/** GalleryGrid lays out the preview tiles, opening a lightbox for tiles with a preview. */
function GalleryGrid({ items }: { items: GalleryPreviewItem[] }) {
  return (
    <div
      data-testid="unixfs-gallery-grid"
      className="grid-cols-gallery grid gap-3"
    >
      {items.map((item) => {
        if (!item.previewURL) {
          return <GalleryTile key={item.path} interactive={false} item={item} />
        }
        return (
          <PhotoView key={item.path} src={item.previewURL}>
            <GalleryTile interactive item={item} />
          </PhotoView>
        )
      })}
    </div>
  )
}

function UnixFSGalleryBody({
  rootHandle,
  currentPath,
  unixfsId,
}: UnixFSBrowserBodyProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(
    null,
  )
  const scan = useGalleryScan(rootHandle, currentPath, unixfsId)
  const { galleryState, previewItems, lightboxItems, isScanning } = scan
  const { spaceId, sessionIndex, httpPathPrefix } = scan
  const galleryErrors = galleryState.value?.errors ?? []
  const hasItems = previewItems.length > 0
  const handlePortalContainer = useCallback((el: HTMLDivElement | null) => {
    setPortalContainer(el)
  }, [])
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
      className="relative h-full w-full overflow-auto px-4 py-3"
    >
      <GalleryHeader
        imageCount={previewItems.length}
        scopePath={scan.scopePath}
        issueCount={galleryErrors.length}
        isScanning={isScanning}
      />
      <div className="min-h-0">
        {galleryState.error && (
          <div className="text-destructive rounded-lg border border-current/20 bg-current/10 px-3 py-2 text-xs">
            {galleryState.error.message}
          </div>
        )}
        {!galleryState.error && !hasItems && (
          <GalleryEmptyState isScanning={isScanning} />
        )}
        {hasItems && (
          <PhotoProvider
            className="!absolute inset-0 h-full w-full"
            portalContainer={portalContainer ?? undefined}
            toolbarRender={toolbarRender}
          >
            <GalleryGrid items={previewItems} />
          </PhotoProvider>
        )}
        {!galleryState.error && galleryErrors.length > 0 && (
          <div className="text-foreground-alt/60 mt-3 text-xs">
            Some descendants could not be scanned. Discovered images remain
            visible.
          </div>
        )}
        {!galleryState.error && !sessionIndex && (
          <div className="text-foreground-alt/60 mt-3 text-xs">
            Inline previews require a mounted session context.
          </div>
        )}
      </div>
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
