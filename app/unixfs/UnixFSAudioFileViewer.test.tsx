import type { ComponentProps, ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { UnixFSAudioFileViewer } from './UnixFSAudioFileViewer.js'

vi.mock('@videojs/react', () => ({
  createPlayer: () => ({
    Provider: ({ children }: { children?: ReactNode }) => children ?? null,
  }),
}))

vi.mock('@videojs/react/audio', () => ({
  Audio: (props: ComponentProps<'audio'>) => <audio {...props} />,
  audioFeatures: {},
  AudioSkin: ({
    children,
    ...props
  }: ComponentProps<'div'> & { children?: ReactNode }) => (
    <div {...props}>{children}</div>
  ),
}))

describe('UnixFSAudioFileViewer', () => {
  afterEach(() => {
    cleanup()
  })

  it('relies on the video.js chrome instead of native audio controls', () => {
    render(
      <UnixFSAudioFileViewer
        title="song.mp3"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.mp3?inline=1"
      />,
    )

    const audio = screen.getByTestId('unixfs-audio-element')
    expect(audio.hasAttribute('controls')).toBe(false)
    expect(screen.queryByText('Audio preview')).toBeNull()
  })

  it('shows a loading state until audio metadata is available', () => {
    render(
      <UnixFSAudioFileViewer
        title="song.mp3"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.mp3?inline=1"
      />,
    )

    expect(screen.getByText('Loading preview')).toBeDefined()

    const audio = screen.getByTestId('unixfs-audio-element')
    fireEvent(audio, new Event('loadedmetadata'))

    expect(screen.queryByText('Loading preview')).toBeNull()
  })

  it('shows an unsupported state when the browser rejects the audio source', () => {
    render(
      <UnixFSAudioFileViewer
        title="song.flac"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.flac?inline=1"
      />,
    )

    const audio = screen.getByTestId('unixfs-audio-element')
    Object.defineProperty(audio, 'error', {
      configurable: true,
      value: { code: 4 },
    })
    fireEvent(audio, new Event('error'))

    expect(screen.getByText('Audio preview unavailable')).toBeDefined()
    expect(
      screen.getByText('This browser cannot play this audio file.'),
    ).toBeDefined()
  })

  it('resets player chrome when the projected file url changes', () => {
    const { rerender } = render(
      <UnixFSAudioFileViewer
        title="song.mp3"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.mp3?inline=1"
      />,
    )

    const firstAudio = screen.getByTestId('unixfs-audio-element')
    fireEvent(firstAudio, new Event('loadedmetadata'))
    expect(screen.queryByText('Loading preview')).toBeNull()

    rerender(
      <UnixFSAudioFileViewer
        title="song.m4a"
        inlineFileURL="/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.m4a?inline=1"
      />,
    )

    expect(screen.getByText('Loading preview')).toBeDefined()
    const secondAudio = screen.getByTestId('unixfs-audio-element')
    expect(secondAudio.getAttribute('src')).toBe(
      '/p/spacewave-core/fs/u/1/so/space-test/-/docs/demo/-/nested/song.m4a?inline=1',
    )
  })
})
