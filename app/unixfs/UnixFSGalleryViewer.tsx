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
        <div className="text-foreground-alt/50 truncate text-[0.6rem]">
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

function UnixFSGalleryBody({
  rootHandle,
  currentPath,
  unixfsId,
}: UnixFSBrowserBodyProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(
    null,
  )
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
  const galleryErrors = galleryState.value?.errors ?? []
  const galleryComplete = galleryState.value?.complete ?? false
  const scopePath = galleryState.value?.scopePath ?? currentPath
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
              ),
      })),
    [galleryItems, sessionIndex, spaceId, unixfsId],
  )
  const lightboxItems = useMemo(
    () => previewItems.filter((item) => !!item.previewURL),
    [previewItems],
  )
  const isScanning = !galleryComplete && !galleryState.error
  const hasItems = previewItems.length > 0
  const handlePortalContainer = useCallback((el: HTMLDivElement | null) => {
    setPortalContainer(el)
  }, [])

  return (
    <div
      data-testid="unixfs-gallery-viewer"
      ref={handlePortalContainer}
      className="relative h-full w-full overflow-auto px-4 py-3"
    >
      <div className="mb-3 flex min-h-6 items-center justify-between gap-3">
        <div className="text-foreground-alt/60 min-w-0 truncate text-[0.6rem]">
          {previewItems.length} image
          {previewItems.length === 1 ? '' : 's'} under {scopePath}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {galleryErrors.length > 0 && (
            <div className="border-destructive/20 bg-destructive/10 text-destructive rounded-full border px-2 py-0.5 text-[0.6rem] font-medium">
              {galleryErrors.length} issue
              {galleryErrors.length === 1 ? '' : 's'}
            </div>
          )}
          {isScanning && (
            <div className="border-foreground/10 bg-foreground/5 text-foreground-alt flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[0.6rem] font-medium">
              <Spinner size="sm" />
              Scanning
            </div>
          )}
        </div>
      </div>
      <div className="min-h-0">
        {galleryState.error && (
          <div className="text-destructive rounded-lg border border-current/20 bg-current/10 px-3 py-2 text-xs">
            {galleryState.error.message}
          </div>
        )}
        {!galleryState.error && !hasItems && isScanning && (
          <div className="flex h-full min-h-48 items-center justify-center">
            <div className="border-foreground/6 bg-background-card/30 flex max-w-xs flex-col items-center gap-2 rounded-lg border px-4 py-5 text-center">
              <LuImage className="text-foreground-alt size-5" />
              <div className="text-foreground text-sm font-semibold">
                Scanning for images
              </div>
              <div className="text-foreground-alt text-xs">
                The gallery will populate as image files are discovered in this
                subtree.
              </div>
            </div>
          </div>
        )}
        {!galleryState.error && !hasItems && !isScanning && (
          <div className="flex h-full min-h-48 items-center justify-center">
            <div className="border-foreground/6 bg-background-card/30 flex max-w-xs flex-col items-center gap-2 rounded-lg border px-4 py-5 text-center">
              <LuImage className="text-foreground-alt size-5" />
              <div className="text-foreground text-sm font-semibold">
                No images under this path
              </div>
            </div>
          </div>
        )}
        {hasItems && (
          <PhotoProvider
            className="!absolute inset-0 h-full w-full"
            portalContainer={portalContainer ?? undefined}
            toolbarRender={({ index }) => {
              const item = lightboxItems[index]
              if (!item?.previewURL || !spaceId || !sessionIndex) {
                return null
              }
              return (
                <div className="mr-2 flex items-center gap-2">
                  <button
                    type="button"
                    className="rounded-full border border-white/20 bg-white/10 p-2 text-white transition hover:bg-white/20"
                    onClick={() =>
                      window.open(
                        item.previewURL,
                        '_blank',
                        'noopener,noreferrer',
                      )
                    }
                    title="Open In Browser"
                  >
                    <LuExternalLink className="size-4" />
                  </button>
                  <button
                    type="button"
                    className="rounded-full border border-white/20 bg-white/10 p-2 text-white transition hover:bg-white/20"
                    onClick={() => {
                      void downloadURL(
                        buildUnixFSFileDownloadURL(
                          sessionIndex,
                          spaceId,
                          unixfsId,
                          item.path,
                        ),
                        item.name,
                      ).catch((err: unknown) => {
                        console.error('failed to download unixfs file', err)
                        toast.error('Download failed', {
                          description: String(err),
                        })
                      })
                    }}
                    title="Download"
                  >
                    <LuDownload className="size-4" />
                  </button>
                </div>
              )
            }}
          >
            <div
              data-testid="unixfs-gallery-grid"
              className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,12rem),1fr))] gap-3"
            >
              {previewItems.map((item) => {
                const supportsLightbox = !!item.previewURL
                if (!supportsLightbox) {
                  return (
                    <GalleryTile
                      key={item.path}
                      interactive={false}
                      item={item}
                    />
                  )
                }
                return (
                  <PhotoView key={item.path} src={item.previewURL}>
                    <GalleryTile interactive item={item} />
                  </PhotoView>
                )
              })}
            </div>
          </PhotoProvider>
        )}
        {!galleryState.error && galleryErrors.length > 0 && (
          <div className="text-foreground-alt/60 mt-3 text-[0.6rem]">
            Some descendants could not be scanned. Discovered images remain
            visible.
          </div>
        )}
        {!galleryState.error && !sessionIndex && (
          <div className="text-foreground-alt/60 mt-3 text-[0.6rem]">
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
