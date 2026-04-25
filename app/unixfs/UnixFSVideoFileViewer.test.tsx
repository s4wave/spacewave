import type { ComponentProps, ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { UnixFSVideoFileViewer } from './UnixFSVideoFileViewer.js'

vi.mock('@videojs/react', () => ({
  createPlayer: () => ({
    Provider: ({ children }: { children?: ReactNode }) => children ?? null,
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

    expect(screen.getByText('Loading preview')).toBeDefined()

    const video = screen.getByTestId('unixfs-video-element')
    fireEvent(video, new Event('loadedmetadata'))

    expect(screen.queryByText('Loading preview')).toBeNull()
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

    expect(screen.getByText('Video preview unavailable')).toBeDefined()
    expect(
      screen.getByText('This browser cannot play this video file.'),
    ).toBeDefined()
  })

  it('resets player chrome when the projected file url changes', () => {
    const { rerender } = render(
      <UnixFSVideoFileViewer
        title="demo.mp4"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    const firstVideo = screen.getByTestId('unixfs-video-element')
    fireEvent(firstVideo, new Event('loadedmetadata'))
    expect(screen.queryByText('Loading preview')).toBeNull()

    rerender(
      <UnixFSVideoFileViewer
        title="demo.webm"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.webm?inline=1"
      />,
    )

    expect(screen.getByText('Loading preview')).toBeDefined()
    const secondVideo = screen.getByTestId('unixfs-video-element')
    expect(secondVideo.getAttribute('src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.webm?inline=1',
    )
  })
})
