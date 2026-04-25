import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { cloneElement, isValidElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UnixFSGalleryViewer } from './UnixFSGalleryViewer.js'
import type { UnixFSGalleryDiscoveryState } from './gallery.js'

const h = vi.hoisted(() => ({
  buildDownloadURL: vi.fn((_, __, ___, path: string) => `/download${path}`),
  buildInlineURL: vi.fn((_, __, ___, path: string) => `/inline${path}`),
  galleryState: null as unknown as ReturnType<typeof buildGalleryState>,
  downloadURL: vi.fn(),
  lightboxIndex: 0,
  onSelectComponent: vi.fn(),
  photoProviderProps: null as null | {
    className?: string
    portalContainer?: HTMLElement
  },
  photoViewClick: vi.fn(),
  routerPath: '',
}))

function buildGalleryState(
  value: UnixFSGalleryDiscoveryState = {
    scopePath: '/',
    items: [],
    errors: [],
    complete: true,
  },
) {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

function buildResource<T>(value: T) {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => h.galleryState,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  usePath: () => h.routerPath,
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: () => buildResource({}),
}))

vi.mock('@s4wave/web/download.js', () => ({
  downloadURL: h.downloadURL,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 1,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({ spaceId: 'space-test' }),
  },
}))

vi.mock('@s4wave/web/object/ObjectViewerContext.js', () => ({
  useObjectViewer: () => ({
    visibleComponents: [
      {
        typeID: 'unixfs/fs-node',
        name: 'UnixFS Viewer',
        component: () => null,
      },
      {
        typeID: 'unixfs/fs-node',
        name: 'UnixFS Gallery',
        component: () => null,
      },
    ],
    selectedComponent: undefined,
    onSelectComponent: h.onSelectComponent,
  }),
}))

vi.mock('./download.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./download.js')>()
  return {
    ...actual,
    buildUnixFSFileDownloadURL: h.buildDownloadURL,
    buildUnixFSFileInlineURL: h.buildInlineURL,
  }
})

vi.mock('react-photo-view', () => ({
  PhotoProvider: ({
    children,
    className,
    portalContainer,
    toolbarRender,
  }: {
    children?: ReactNode
    className?: string
    portalContainer?: HTMLElement
    toolbarRender?: (props: {
      images: unknown[]
      index: number
      onIndexChange: (index: number) => void
      visible: boolean
      onClose: () => void
      overlayVisible: boolean
      overlay?: ReactNode
      rotate: number
      onRotate: (rotate: number) => void
      scale: number
      onScale: (scale: number) => void
    }) => ReactNode
  }) => {
    h.photoProviderProps = {
      className,
      portalContainer,
    }
    return (
      <>
        {children}
        {toolbarRender?.({
          images: [],
          index: h.lightboxIndex,
          onIndexChange: () => undefined,
          visible: true,
          onClose: () => undefined,
          overlayVisible: true,
          overlay: null,
          rotate: 0,
          onRotate: () => undefined,
          scale: 1,
          onScale: () => undefined,
        })}
      </>
    )
  },
  PhotoView: ({ children }: { children?: ReactNode }) =>
    isValidElement<{ onClick?: () => void }>(children) ?
      cloneElement(children, {
        onClick: h.photoViewClick,
      })
    : children,
}))

describe('UnixFSGalleryViewer', () => {
  beforeEach(() => {
    h.buildDownloadURL.mockClear()
    h.buildInlineURL.mockClear()
    h.downloadURL.mockClear()
    h.lightboxIndex = 0
    h.onSelectComponent.mockReset()
    h.photoProviderProps = null
    h.photoViewClick.mockReset()
    h.routerPath = ''
    h.galleryState = buildGalleryState()
  })

  afterEach(() => {
    cleanup()
  })

  it('shows the no-images empty state and switches back to the browser viewer', () => {
    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/',
            },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(screen.getByText('No images under this path')).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Switch to Browser' }))

    expect(h.onSelectComponent).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'UnixFS Viewer' }),
    )
  })

  it('wires the lightbox toolbar actions to the current image', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    h.lightboxIndex = 1
    h.galleryState = buildGalleryState({
      scopePath: '/',
      items: [
        {
          path: '/gallery/first.png',
          name: 'first.png',
          label: 'first.png',
          mimeType: 'image/png',
        },
        {
          path: '/gallery/second.svg',
          name: 'second.svg',
          label: 'second.svg',
          mimeType: 'image/svg+xml',
        },
      ],
      errors: [],
      complete: true,
    })

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/',
            },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    fireEvent.click(screen.getByTitle('Open In Browser'))
    fireEvent.click(screen.getByTitle('Download'))

    expect(openSpy).toHaveBeenCalledWith(
      '/inline/gallery/second.svg',
      '_blank',
      'noopener,noreferrer',
    )
    expect(h.downloadURL).toHaveBeenCalledWith(
      '/download/gallery/second.svg',
      'second.svg',
    )

    openSpy.mockRestore()
  })

  it('shows the resolved parent scope when the current object path points at a file', () => {
    h.galleryState = buildGalleryState({
      scopePath: '/gallery',
      items: [
        {
          path: '/gallery/first.png',
          name: 'first.png',
          label: 'first.png',
          mimeType: 'image/png',
        },
      ],
      errors: [],
      complete: true,
    })

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/gallery/first.png',
            },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(screen.getByText('1 image under /gallery')).toBeDefined()
  })

  it('scopes the photo portal to the gallery viewer root', () => {
    h.galleryState = buildGalleryState({
      scopePath: '/gallery',
      items: [
        {
          path: '/gallery/first.png',
          name: 'first.png',
          label: 'first.png',
          mimeType: 'image/png',
        },
      ],
      errors: [],
      complete: true,
    })

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/gallery',
            },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(h.photoProviderProps?.portalContainer).toBe(
      screen.getByTestId('unixfs-gallery-viewer'),
    )
    expect(h.photoProviderProps?.className).toContain('!absolute')
  })

  it('forwards PhotoView click behavior into gallery tiles', () => {
    h.galleryState = buildGalleryState({
      scopePath: '/gallery',
      items: [
        {
          path: '/gallery/first.png',
          name: 'first.png',
          label: 'first.png',
          mimeType: 'image/png',
        },
      ],
      errors: [],
      complete: true,
    })

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/gallery',
            },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    fireEvent.click(screen.getByTestId('unixfs-gallery-item'))

    expect(h.photoViewClick).toHaveBeenCalledTimes(1)
  })
})
