import type { ComponentProps, ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { UnixFSVideoFileViewer } from './UnixFSVideoFileViewer.js'

vi.mock('@videojs/react', () => ({
  createPlayer: () => ({
    Player: ({ children }: { children?: ReactNode }) => children ?? null,
  }),
}))

vi.mock('@videojs/react/video', () => ({
  Video: (props: ComponentProps<'video'>) => <video {...props} />,
  videoFeatures: {},
  VideoSkin: ({
    children,
    ...props
  }: ComponentProps<'div'> & { children?: ReactNode }) => (
    <div {...props}>{children}</div>
  ),
}))

describe('UnixFSVideoFileViewer', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('relies on the video.js chrome instead of native video controls', () => {
    render(
      <UnixFSVideoFileViewer
        title="demo.mp4"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    const video = screen.getByTestId('unixfs-video-element')
    expect(video.hasAttribute('controls')).toBe(false)
  })

  it('shows a loading state until video metadata is available', () => {
    render(
      <UnixFSVideoFileViewer
        title="demo.mp4"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    expect(screen.getByText('Loading the video')).toBeDefined()

    const video = screen.getByTestId('unixfs-video-element')
    fireEvent(video, new Event('loadedmetadata'))

    expect(screen.queryByText('Loading the video')).toBeNull()
  })

  it('treats restored video metadata as ready on mount', () => {
    vi.spyOn(HTMLMediaElement.prototype, 'readyState', 'get').mockReturnValue(1)

    render(
      <UnixFSVideoFileViewer
        title="demo.mp4"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    expect(screen.queryByText('Loading the video')).toBeNull()
  })

  it('returns to loading when the browser starts loading the media again', () => {
    render(
      <UnixFSVideoFileViewer
        title="demo.mp4"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    const video = screen.getByTestId('unixfs-video-element')
    fireEvent(video, new Event('loadedmetadata'))
    expect(screen.queryByText('Loading the video')).toBeNull()

    fireEvent(video, new Event('loadstart'))
    expect(screen.getByText('Loading the video')).toBeDefined()

    fireEvent(video, new Event('loadedmetadata'))
    expect(screen.queryByText('Loading the video')).toBeNull()
  })

  it('shows an unsupported state when the browser rejects the video source', () => {
    render(
      <UnixFSVideoFileViewer
        title="demo.webm"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.webm?inline=1"
      />,
    )

    const video = screen.getByTestId('unixfs-video-element')
    Object.defineProperty(video, 'error', {
      configurable: true,
      value: { code: 4 },
    })
    fireEvent(video, new Event('error'))

    expect(
      screen.getByText(
        'This browser cannot play this video format. Download it to open it elsewhere.',
      ),
    ).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    expect(screen.getByRole('link', { name: 'Download' })).toBeDefined()
  })

  it('loads the video again at its position when a stopped load is retried', () => {
    render(
      <UnixFSVideoFileViewer
        title="demo.webm"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.webm?inline=1"
      />,
    )

    const video = screen.getByTestId('unixfs-video-element') as HTMLVideoElement
    const load = vi.fn()
    video.load = load
    Object.defineProperty(video, 'error', {
      configurable: true,
      value: { code: 2 },
    })
    video.currentTime = 42
    fireEvent(video, new Event('error'))

    expect(screen.getByRole('alert').textContent).toContain(
      'The video stopped loading.',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(load).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByText('Loading the video')).toBeDefined()

    // The new load starts at 0; its metadata seeks back to where play stood.
    video.currentTime = 0
    fireEvent(video, new Event('loadedmetadata'))
    expect(video.currentTime).toBe(42)
  })

  it('resets player chrome without replacing the viewer shell when the projected file url changes', () => {
    const { rerender } = render(
      <UnixFSVideoFileViewer
        title="demo.mp4"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    const shell = screen.getByTestId('unixfs-video-viewer')
    const firstVideo = screen.getByTestId('unixfs-video-element')
    fireEvent(firstVideo, new Event('loadedmetadata'))
    expect(screen.queryByText('Loading the video')).toBeNull()

    rerender(
      <UnixFSVideoFileViewer
        title="demo.webm"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.webm?inline=1"
      />,
    )

    expect(screen.getByText('Loading the video')).toBeDefined()
    expect(screen.getByTestId('unixfs-video-viewer')).toBe(shell)
    const secondVideo = screen.getByTestId('unixfs-video-element')
    expect(secondVideo).not.toBe(firstVideo)
    expect(secondVideo.getAttribute('src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.webm?inline=1',
    )
  })
})
