import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { UnixFSFileViewer } from './UnixFSFileViewer.js'

function buildResource<T>(value: T) {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: () => buildResource(null),
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  isTextMimeType: (mimeType: string) => mimeType.startsWith('text/'),
  isImageMimeType: (mimeType: string) => mimeType.startsWith('image/'),
  isAudioMimeType: (mimeType: string) => mimeType.startsWith('audio/'),
  isVideoMimeType: (mimeType: string) => mimeType.startsWith('video/'),
  useUnixFSHandle: () => buildResource(null),
  useUnixFSHandleTextContent: () => buildResource(''),
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  useHistory: () => null,
}))

vi.mock('@s4wave/web/editors/file-browser/Toolbar.js', () => ({
  Toolbar: () => <div>Toolbar</div>,
}))

vi.mock('./UnixFSPdfFileViewer.js', () => ({
  UnixFSPdfFileViewer: ({
    inlineFileURL,
    title,
  }: {
    inlineFileURL: string
    title: string
  }) => (
    <div
      data-src={inlineFileURL}
      data-testid="unixfs-pdf-viewer"
      aria-label={title}
    />
  ),
}))

vi.mock('./UnixFSAudioFileViewer.js', () => ({
  UnixFSAudioFileViewer: ({
    inlineFileURL,
    title,
  }: {
    inlineFileURL: string
    title: string
  }) => (
    <div
      data-src={inlineFileURL}
      data-testid="unixfs-audio-viewer"
      aria-label={title}
    />
  ),
}))

vi.mock('./UnixFSVideoFileViewer.js', () => ({
  UnixFSVideoFileViewer: ({
    inlineFileURL,
    title,
  }: {
    inlineFileURL: string
    title: string
  }) => (
    <div
      data-src={inlineFileURL}
      data-testid="unixfs-video-viewer"
      aria-label={title}
    />
  ),
}))

describe('UnixFSFileViewer image preview', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders an image tag for image mime types when an inline raw url is provided', () => {
    render(
      <UnixFSFileViewer
        path="/nested/logo.png"
        stat={{
          info: { isDir: false, name: 'logo.png' },
          mimeType: 'image/png',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/logo.png?inline=1"
      />,
    )

    const img = screen.getByRole('img', { name: 'logo.png' })
    expect(img.getAttribute('src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/logo.png?inline=1',
    )
  })

  it('keeps the binary placeholder when no inline raw url is available', () => {
    render(
      <UnixFSFileViewer
        path="/nested/logo.png"
        stat={{
          info: { isDir: false, name: 'logo.png' },
          mimeType: 'image/png',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
      />,
    )

    expect(screen.queryByRole('img', { name: 'logo.png' })).toBeNull()
    expect(screen.getByText('Binary file preview not available')).toBeDefined()
  })

  it('keeps the binary placeholder for non-image files', () => {
    render(
      <UnixFSFileViewer
        path="/nested/archive.bin"
        stat={{
          info: { isDir: false, name: 'archive.bin' },
          mimeType: 'application/octet-stream',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
      />,
    )

    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('Binary file preview not available')).toBeDefined()
  })

  it('renders a pdf viewer for pdf mime types when an inline raw url is provided', () => {
    render(
      <UnixFSFileViewer
        path="/nested/guide.pdf"
        stat={{
          info: { isDir: false, name: 'guide.pdf' },
          mimeType: 'application/pdf',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/guide.pdf?inline=1"
      />,
    )

    const pdf = screen.getByTestId('unixfs-pdf-viewer')
    expect(pdf.getAttribute('aria-label')).toBe('guide.pdf')
    expect(pdf.getAttribute('data-src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/guide.pdf?inline=1',
    )
  })

  it('keeps the binary placeholder for pdf files when no inline raw url is available', () => {
    render(
      <UnixFSFileViewer
        path="/nested/guide.pdf"
        stat={{
          info: { isDir: false, name: 'guide.pdf' },
          mimeType: 'application/pdf',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
      />,
    )

    expect(screen.queryByTestId('unixfs-pdf-viewer')).toBeNull()
    expect(screen.getByText('Binary file preview not available')).toBeDefined()
  })

  it('renders an audio element for audio mime types when an inline raw url is provided', () => {
    render(
      <UnixFSFileViewer
        path="/nested/song.mp3"
        stat={{
          info: { isDir: false, name: 'song.mp3' },
          mimeType: 'audio/mpeg',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.mp3?inline=1"
      />,
    )

    const audio = screen.getByTestId('unixfs-audio-viewer')
    expect(audio.getAttribute('aria-label')).toBe('song.mp3')
    expect(audio.getAttribute('data-src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.mp3?inline=1',
    )
  })

  it('renders an audio element for opus files when an inline raw url is provided', () => {
    render(
      <UnixFSFileViewer
        path="/nested/song.opus"
        stat={{
          info: { isDir: false, name: 'song.opus' },
          mimeType: 'audio/ogg',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.opus?inline=1"
      />,
    )

    const audio = screen.getByTestId('unixfs-audio-viewer')
    expect(audio.getAttribute('aria-label')).toBe('song.opus')
    expect(audio.getAttribute('data-src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.opus?inline=1',
    )
  })

  it('renders an audio element for webm files when the mime type is audio/webm', () => {
    render(
      <UnixFSFileViewer
        path="/nested/song.webm"
        stat={{
          info: { isDir: false, name: 'song.webm' },
          mimeType: 'audio/webm',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.webm?inline=1"
      />,
    )

    const audio = screen.getByTestId('unixfs-audio-viewer')
    expect(audio.getAttribute('aria-label')).toBe('song.webm')
    expect(audio.getAttribute('data-src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.webm?inline=1',
    )
  })

  it('keeps the binary placeholder for audio files when no inline raw url is available', () => {
    render(
      <UnixFSFileViewer
        path="/nested/song.mp3"
        stat={{
          info: { isDir: false, name: 'song.mp3' },
          mimeType: 'audio/mpeg',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
      />,
    )

    expect(screen.queryByTestId('unixfs-audio-viewer')).toBeNull()
    expect(screen.getByText('Binary file preview not available')).toBeDefined()
  })

  it('renders a video element for video mime types when an inline raw url is provided', () => {
    render(
      <UnixFSFileViewer
        path="/nested/demo.mp4"
        stat={{
          info: { isDir: false, name: 'demo.mp4' },
          mimeType: 'video/mp4',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1"
      />,
    )

    const video = screen.getByTestId('unixfs-video-viewer')
    expect(video.getAttribute('aria-label')).toBe('demo.mp4')
    expect(video.getAttribute('data-src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/demo.mp4?inline=1',
    )
  })

  it('keeps the binary placeholder for video files when no inline raw url is available', () => {
    render(
      <UnixFSFileViewer
        path="/nested/demo.mp4"
        stat={{
          info: { isDir: false, name: 'demo.mp4' },
          mimeType: 'video/mp4',
        }}
        rootHandle={buildResource(null)}
        hideToolbar
      />,
    )

    expect(screen.queryByTestId('unixfs-video-viewer')).toBeNull()
    expect(screen.getByText('Binary file preview not available')).toBeDefined()
  })
})
