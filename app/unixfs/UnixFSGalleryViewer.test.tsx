import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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
  spaceId: 'space-test' as string | null,
  // realStream runs the real useStreamingResource over streamIndex.
  realStream: false,
  streamIndex: null as
    | null
    | ((path: string, signal: AbortSignal) => AsyncIterable<unknown>),
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

vi.mock(
  '@aptre/bldr-sdk/hooks/useStreamingResource.js',
  async (importOriginal) => {
    const actual =
      await importOriginal<
        typeof import('@aptre/bldr-sdk/hooks/useStreamingResource.js')
      >()
    return {
      useStreamingResource: (
        ...args: Parameters<typeof actual.useStreamingResource>
      ) =>
        h.realStream ? actual.useStreamingResource(...args) : h.galleryState,
    }
  },
)

vi.mock('./gallery.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./gallery.js')>()
  return {
    ...actual,
    streamUnixFSGalleryCandidates: (
      _handle: unknown,
      path: string,
      signal: AbortSignal,
    ) => h.streamIndex!(path, signal),
  }
})

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
    useContextSafe: () => (h.spaceId ? { spaceId: h.spaceId } : null),
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
    h.spaceId = 'space-test'
    h.realStream = false
    h.streamIndex = null
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
    expect(screen.getByText('No images under this path.')).toBeDefined()
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
    fireEvent.click(screen.getByRole('button', { name: 'Open in browser' }))
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
    expect(h.photoSliderProps?.className).toContain('!absolute')
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

  it('names each tile by its file and keeps the tiles pressable while the index reads again', () => {
    h.galleryState = {
      ...buildGalleryState({
        scopePath: '/gallery',
        items: buildItems(2),
        errors: ['/gallery/locked'],
        truncated: false,
      }),
      loading: true,
    }

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: { unixfsId: 'files', path: '/gallery' },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(screen.getByRole('status').textContent).toBe(
      'Loading the images under /gallery again',
    )
    expect(
      screen.getByText(
        'Some folders could not be read; the images found are shown.',
      ),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Open 1.png' }))
    expect(h.photoSliderProps).toMatchObject({ index: 1, visible: true })
  })

  it('offers Retry when the first read fails', () => {
    const retry = vi.fn()
    h.galleryState = {
      value: null as unknown as UnixFSGalleryDiscoveryState,
      loading: false,
      error: new Error('watch closed') as unknown as null,
      retry,
    }

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: { unixfsId: 'files', path: '/gallery' },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(screen.getByRole('alert').textContent).toContain(
      'Could not read the images under /gallery: watch closed',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('keeps the last tiles pressable under the failure once the watch gives up backing off', () => {
    const retry = vi.fn()
    h.galleryState = {
      value: null as unknown as UnixFSGalleryDiscoveryState,
      last: {
        scopePath: '/gallery',
        items: buildItems(2),
        errors: [],
        truncated: false,
      },
      loading: false,
      error: new Error('watch closed') as unknown as null,
      retry,
    } as ReturnType<typeof buildGalleryState>

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: { unixfsId: 'files', path: '/gallery' },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(screen.getByRole('alert').textContent).toContain(
      'Could not read the images under /gallery: watch closed',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: 'Open 1.png' }))
    expect(h.photoSliderProps).toMatchObject({ index: 1, visible: true })
  })

  it('makes no empty claim for a failed index with no image', () => {
    h.galleryState = {
      value: null as unknown as UnixFSGalleryDiscoveryState,
      last: { scopePath: '/gallery', items: [], errors: [], truncated: false },
      loading: false,
      error: new Error('watch closed') as unknown as null,
      retry: vi.fn(),
    } as ReturnType<typeof buildGalleryState>

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: { unixfsId: 'files', path: '/gallery' },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(screen.getByRole('alert')).toBeDefined()
    expect(screen.queryByText('No images under this path.')).toBeNull()
  })

  it('names the missing Space and keeps tiles unpressable without previews', () => {
    h.spaceId = null
    h.galleryState = buildGalleryState({
      scopePath: '/gallery',
      items: buildItems(2),
      errors: [],
      truncated: false,
    })

    render(
      <UnixFSGalleryViewer
        objectInfo={{
          info: {
            case: 'unixfsObjectInfo',
            value: { unixfsId: 'files', path: '/gallery' },
          },
        }}
        worldState={buildResource(null)}
      />,
    )

    expect(
      screen.getByText(
        'Previews need a session; open this folder from a Space to see them.',
      ),
    ).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Open 1.png' })).toBeNull()
    expect(h.buildInlineURL).not.toHaveBeenCalled()
  })

  it("starts a new folder from waiting, never with the last folder's tiles", async () => {
    h.realStream = true
    // The first folder yields its index; the second never answers.
    h.streamIndex = (path, signal) =>
      (async function* () {
        if (path === '/a') {
          yield {
            scopePath: '/a',
            items: [
              {
                path: '/a/x.png',
                name: 'x.png',
                label: 'x.png',
                mimeType: 'image/png',
              },
            ],
            errors: [],
            truncated: false,
          }
        }
        await new Promise((resolve) =>
          signal.addEventListener('abort', resolve),
        )
      })()
    const objectInfo = (path: string) => ({
      info: {
        case: 'unixfsObjectInfo' as const,
        value: { unixfsId: 'files', path },
      },
    })

    const view = render(
      <UnixFSGalleryViewer
        objectInfo={objectInfo('/a')}
        worldState={buildResource(null)}
      />,
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Open x.png' })).toBeDefined(),
    )

    view.rerender(
      <UnixFSGalleryViewer
        objectInfo={objectInfo('/b')}
        worldState={buildResource(null)}
      />,
    )
    expect(screen.queryByRole('button', { name: 'Open x.png' })).toBeNull()
    expect(screen.getByRole('status').textContent).toBe(
      'Loading the images under /b',
    )
  })
})
