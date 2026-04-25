import 'react-photo-view/dist/react-photo-view.css'

import { useCallback, useState, type MouseEventHandler } from 'react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  LuDownload,
  LuExternalLink,
  LuFolderOpen,
  LuImage,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { PhotoProvider, PhotoView } from 'react-photo-view'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { usePath } from '@s4wave/web/router/router.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useObjectViewer as useObjectViewerContext } from '@s4wave/web/object/ObjectViewerContext.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { downloadURL } from '@s4wave/web/download.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useUnixFSRootHandle } from '@s4wave/web/hooks/useUnixFSHandle.js'
import {
  buildUnixFSFileDownloadURL,
  buildUnixFSFileInlineURL,
} from './download.js'
import {
  type UnixFSGalleryDiscoveryState,
  streamUnixFSGalleryCandidates,
} from './gallery.js'

// joinPath joins two path segments.
function joinPath(base: string, rel: string): string {
  if (!rel || rel === '/') return base
  if (base.endsWith('/')) return base + rel.replace(/^\//, '')
  return base + '/' + rel.replace(/^\//, '')
}

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
        {item.previewURL ?
          <img
            alt={item.label}
            className="h-full w-full object-cover"
            loading="lazy"
            src={item.previewURL}
          />
        : <div className="text-foreground-alt/40 flex h-full w-full items-center justify-center">
            <LuImage className="h-8 w-8" />
          </div>
        }
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

// UnixFSGalleryViewer renders the shell for the UnixFS gallery viewer. Later
// iterations add filtering, progressive updates, and lightbox behavior.
export function UnixFSGalleryViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(
    null,
  )
  const routerPath = usePath()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const objectViewer = useObjectViewerContext()
  const sessionIndex = useSessionIndex()
  const spaceId = spaceCtx?.spaceId ?? null
  const unixfsId = getObjectKey(objectInfo)
  const basePath =
    objectInfo?.info?.case === 'unixfsObjectInfo' ?
      objectInfo.info.value.path || '/'
    : '/'
  const currentPath = joinPath(basePath, routerPath || '/')
  const rootHandle = useUnixFSRootHandle(worldState, unixfsId)
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
  const galleryItems = galleryState.value?.items ?? []
  const galleryErrors = galleryState.value?.errors ?? []
  const galleryComplete = galleryState.value?.complete ?? false
  const scopePath = galleryState.value?.scopePath ?? currentPath
  const previewItems: GalleryPreviewItem[] = galleryItems.map((item) => ({
    path: item.path,
    name: item.name,
    label: item.label,
    mimeType: item.mimeType,
    previewURL:
      !sessionIndex || !spaceId ?
        undefined
      : buildUnixFSFileInlineURL(sessionIndex, spaceId, unixfsId, item.path),
  }))
  const lightboxItems = previewItems.filter((item) => !!item.previewURL)
  const isScanning = !galleryComplete && !galleryState.error
  const hasItems = previewItems.length > 0
  const browserViewer =
    objectViewer?.visibleComponents.find(
      (component) => component.name === 'UnixFS Viewer',
    ) ?? null
  const handlePortalContainer = useCallback((el: HTMLDivElement | null) => {
    setPortalContainer(el)
  }, [])

  return (
    <div
      data-testid="unixfs-gallery-viewer"
      ref={handlePortalContainer}
      className="relative flex h-full w-full flex-col overflow-hidden"
    >
      <div className="border-foreground/8 flex shrink-0 items-center justify-between border-b px-4 py-2">
        <div className="flex items-center gap-2">
          <div className="bg-brand/10 text-brand flex h-8 w-8 items-center justify-center rounded-lg">
            <LuImage className="h-4 w-4" />
          </div>
          <div>
            <div className="text-foreground text-sm font-semibold tracking-tight">
              UnixFS Gallery
            </div>
            <div className="text-foreground-alt/50 text-[0.6rem]">
              {previewItems.length} image
              {previewItems.length === 1 ? '' : 's'} under {scopePath}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-3">
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
      <div className="flex-1 overflow-auto px-4 py-3">
        {galleryState.error && (
          <div className="text-destructive rounded-lg border border-current/20 bg-current/10 px-3 py-2 text-xs">
            {galleryState.error.message}
          </div>
        )}
        {!galleryState.error && !hasItems && isScanning && (
          <div className="flex h-full min-h-48 items-center justify-center">
            <div className="border-foreground/6 bg-background-card/30 flex max-w-xs flex-col items-center gap-2 rounded-lg border px-4 py-5 text-center">
              <LuImage className="text-foreground-alt h-5 w-5" />
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
            <div className="border-foreground/6 bg-background-card/30 flex max-w-xs flex-col items-center gap-3 rounded-lg border px-4 py-5 text-center">
              <LuImage className="text-foreground-alt h-5 w-5" />
              <div className="text-foreground text-sm font-semibold">
                No images under this path
              </div>
              <div className="text-foreground-alt text-xs">
                Switch back to the UnixFS browser to keep exploring this
                subtree.
              </div>
              {browserViewer && (
                <DashboardButton
                  icon={<LuFolderOpen className="h-3.5 w-3.5" />}
                  onClick={() => objectViewer?.onSelectComponent(browserViewer)}
                >
                  Switch to Browser
                </DashboardButton>
              )}
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
                    <LuExternalLink className="h-4 w-4" />
                  </button>
                  <button
                    type="button"
                    className="rounded-full border border-white/20 bg-white/10 p-2 text-white transition hover:bg-white/20"
                    onClick={() =>
                      downloadURL(
                        buildUnixFSFileDownloadURL(
                          sessionIndex,
                          spaceId,
                          unixfsId,
                          item.path,
                        ),
                        item.name,
                      )
                    }
                    title="Download"
                  >
                    <LuDownload className="h-4 w-4" />
                  </button>
                </div>
              )
            }}
          >
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
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
