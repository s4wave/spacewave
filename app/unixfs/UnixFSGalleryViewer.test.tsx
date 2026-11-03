import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UnixFSGalleryViewer } from './UnixFSGalleryViewer.js'
import type { UnixFSGalleryDiscoveryState } from './gallery.js'

const h = vi.hoisted(() => ({
  browserProps: {
    rendered: false,
    unixfsId: '',
    currentPath: '',
  },
  buildDownloadURL: vi.fn((_, __, ___, path: string) => `/download${path}`),
  buildInlineURL: vi.fn((_, __, ___, path: string) => `/inline${path}`),
  galleryState: null as unknown as ReturnType<typeof buildGalleryState>,
  downloadURL: vi.fn(() => Promise.resolve()),
  photoSliderProps: null as null | {
    className?: string
    index?: number
    portalContainer?: HTMLElement
    visible: boolean
  },
  routerPath: '',
  rootHandle: {
    value: {},
    loading: false,
    error: null,
    retry: vi.fn(),
  },
}))

function buildGalleryState(
  value: UnixFSGalleryDiscoveryState = {
    scopePath: '/',
    items: [],
    errors: [],
    truncated: false,
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

vi.mock('./UnixFSBrowser.js', () => ({
  UnixFSBrowser: ({
    unixfsId,
    currentPath,
    browserBody: BrowserBody,
  }: {
    unixfsId: string
    currentPath: string
    browserBody?: (props: {
      rootHandle: typeof h.rootHandle
      unixfsId: string
      currentPath: string
    }) => ReactNode
  }) => {
    h.browserProps = { rendered: true, unixfsId, currentPath }
    return (
      <div data-testid="unixfs-browser">
        <div>Toolbar</div>
        {BrowserBody && (
          <BrowserBody
            rootHandle={h.rootHandle}
            unixfsId={unixfsId}
            currentPath={currentPath}
          />
        )}
      </div>
    )
  },
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

vi.mock('./download.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./download.js')>()
  return {
    ...actual,
    buildUnixFSFileDownloadURL: h.buildDownloadURL,
    buildUnixFSFileInlineURL: h.buildInlineURL,
  }
})

vi.mock('react-photo-view', () => ({
  PhotoSlider: ({
    className,
    index,
    portalContainer,
    toolbarRender,
    visible,
  }: {
    className?: string
    index?: number
    portalContainer?: HTMLElement
    toolbarRender?: (props: { index: number }) => ReactNode
    visible: boolean
  }) => {
    h.photoSliderProps = { className, index, portalContainer, visible }
    return visible ? <>{toolbarRender?.({ index: index ?? 0 })}</> : null
  },
}))

// buildItems returns count gallery items under /gallery.
function buildItems(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    path: `/gallery/${i}.png`,
    name: `${i}.png`,
    label: `${i}.png`,
    mimeType: 'image/png',
  }))
}

describe('UnixFSGalleryViewer', () => {
  beforeEach(() => {
    h.browserProps = { rendered: false, unixfsId: '', currentPath: '' }
    h.buildDownloadURL.mockClear()
    h.buildInlineURL.mockClear()
    h.downloadURL.mockClear()
    h.photoSliderProps = null
    h.routerPath = ''
    h.galleryState = buildGalleryState()
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders inside the file browser shell and shows the no-images empty state', () => {
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

    expect(screen.getByTestId('unixfs-browser')).toBeDefined()
    expect(screen.getByText('Toolbar')).toBeDefined()
    expect(screen.getByText('No images under this path')).toBeDefined()
    expect(
      screen.queryByRole('button', { name: 'Switch to Browser' }),
    ).toBeNull()
    expect(h.browserProps).toEqual({
      rendered: true,
      unixfsId: 'files',
      currentPath: '/',
    })
  })

  it('renders only the tiles near the visible part of a large gallery', () => {
    h.galleryState = buildGalleryState({
      scopePath: '/gallery',
      items: buildItems(1000),
      errors: [],
      truncated: false,
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

    expect(screen.getByText('1000 images under /gallery')).toBeDefined()
    const tiles = screen.getAllByTestId('unixfs-gallery-item')
    expect(tiles.length).toBeGreaterThan(0)
    expect(tiles.length).toBeLessThan(50)
  })

  it('wires the lightbox toolbar actions to the current image', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
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
      truncated: false,
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

    fireEvent.click(screen.getAllByTestId('unixfs-gallery-item')[1])
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
  })

  it('memoizes preview URLs while the discovered image list is unchanged', () => {
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
      truncated: false,
    })

    const props = {
      objectInfo: {
        info: {
          case: 'unixfsObjectInfo' as const,
          value: {
            unixfsId: 'files',
            path: '/gallery',
          },
        },
      },
      worldState: buildResource(null),
    }
    const { rerender } = render(<UnixFSGalleryViewer {...props} />)

    expect(h.buildInlineURL).toHaveBeenCalledTimes(1)

    rerender(<UnixFSGalleryViewer {...props} />)

    expect(h.buildInlineURL).toHaveBeenCalledTimes(1)
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
      truncated: false,
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
      truncated: false,
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

    expect(h.photoSliderProps?.portalContainer).toBe(
      screen.getByTestId('unixfs-gallery-viewer'),
    )
    expect(h.photoSliderProps?.className).toContain('unixfs-photo-slider')
  })

  it('opens the lightbox at the clicked tile', () => {
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
      truncated: false,
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

    expect(h.photoSliderProps?.visible).toBe(false)
    fireEvent.click(screen.getByTestId('unixfs-gallery-item'))

    expect(h.photoSliderProps).toMatchObject({ index: 0, visible: true })
  })
})
