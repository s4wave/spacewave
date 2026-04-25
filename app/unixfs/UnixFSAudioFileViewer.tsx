import {
  type CSSProperties,
  type SyntheticEvent,
  useCallback,
  useState,
} from 'react'
import { LuMusic, LuTriangleAlert } from 'react-icons/lu'
import { createPlayer } from '@videojs/react'
import { Audio, audioFeatures, AudioSkin } from '@videojs/react/audio'
import '@videojs/react/audio/skin.css'

import { cn } from '@s4wave/web/style/utils.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

// UnixFSAudioFileViewerProps are the props passed to the UnixFSAudioFileViewer.
export interface UnixFSAudioFileViewerProps {
  // title is the accessible label for the media element.
  title: string
  // inlineFileURL is the projected raw file URL used for playback.
  inlineFileURL: string
}

const Player = createPlayer({
  displayName: 'UnixFSAudioFileViewer',
  features: audioFeatures,
})

const mediaErrAborted = 1
const mediaErrNetwork = 2
const mediaErrDecode = 3
const mediaErrSrcNotSupported = 4

interface AudioPreviewState {
  status: 'loading' | 'ready' | 'error'
  buffering: boolean
  message?: string
  unsupported?: boolean
}

const initialAudioPreviewState: AudioPreviewState = {
  status: 'loading',
  buffering: false,
}

const audioSkinStyle: CSSProperties = {
  '--media-border-radius': '0px',
  borderRadius: 0,
}

function buildAudioErrorState(error: MediaError | null): AudioPreviewState {
  if (error?.code === mediaErrSrcNotSupported) {
    return {
      status: 'error',
      buffering: false,
      unsupported: true,
      message: 'This browser cannot play this audio file.',
    }
  }
  if (error?.code === mediaErrDecode) {
    return {
      status: 'error',
      buffering: false,
      message: 'This audio file could not be decoded.',
    }
  }
  if (error?.code === mediaErrNetwork) {
    return {
      status: 'error',
      buffering: false,
      message: 'This audio file stopped loading unexpectedly.',
    }
  }
  if (error?.code === mediaErrAborted) {
    return {
      status: 'error',
      buffering: false,
      message: 'Audio playback was aborted before the preview became ready.',
    }
  }
  return {
    status: 'error',
    buffering: false,
    message: 'This audio file failed before playback could start.',
  }
}

function UnixFSAudioPlayerSurface({
  title,
  inlineFileURL,
}: UnixFSAudioFileViewerProps) {
  const [state, setState] = useState<AudioPreviewState>(
    initialAudioPreviewState,
  )

  const handleReady = useCallback(() => {
    setState((prev) => {
      if (prev.status === 'ready' && !prev.buffering) {
        return prev
      }
      return {
        status: 'ready',
        buffering: false,
      }
    })
  }, [])

  const handleBuffering = useCallback(() => {
    setState((prev) => {
      if (prev.status === 'error') {
        return prev
      }
      if (prev.status !== 'ready') {
        return prev
      }
      if (prev.buffering) {
        return prev
      }
      return {
        status: 'ready',
        buffering: true,
      }
    })
  }, [])

  const handleError = useCallback((event: SyntheticEvent<HTMLAudioElement>) => {
    setState(buildAudioErrorState(event.currentTarget.error))
  }, [])

  const showLoading = state.status === 'loading'
  const showBuffering = state.status === 'ready' && state.buffering

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <div className="bg-background-primary flex min-h-0 flex-1 items-center justify-center p-4">
        <div className="border-foreground/10 bg-background-card/60 relative flex w-full max-w-2xl flex-col overflow-hidden rounded-xl border shadow-lg">
          <div className="border-foreground/8 flex items-center gap-3 border-b px-4 py-3">
            <div className="bg-brand/10 text-brand flex h-8 w-8 items-center justify-center rounded-full">
              <LuMusic className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-foreground truncate text-sm font-semibold">
                {title}
              </div>
            </div>
            {showBuffering && (
              <div className="border-foreground/10 bg-background/78 flex items-center rounded-full border px-3 py-1.5">
                <LoadingInline label="Buffering" tone="muted" size="sm" />
              </div>
            )}
          </div>

          <div className="relative px-4 py-3">
            {showLoading && (
              <div
                data-testid="unixfs-audio-loading"
                className="bg-background/72 absolute inset-0 z-20 flex items-center justify-center p-6 backdrop-blur-sm"
              >
                <div className="w-full max-w-sm">
                  <LoadingCard
                    view={{
                      state: 'active',
                      title: 'Loading preview',
                      detail: 'Waiting for audio metadata.',
                    }}
                  />
                </div>
              </div>
            )}

            {state.status === 'error' && (
              <div
                data-testid="unixfs-audio-error"
                className="bg-background/82 absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 p-6 text-center backdrop-blur-sm"
              >
                <div className="bg-destructive/10 text-destructive flex h-10 w-10 items-center justify-center rounded-full">
                  <LuTriangleAlert className="h-5 w-5" />
                </div>
                <div className="text-foreground text-sm font-semibold">
                  {state.unsupported ?
                    'Audio preview unavailable'
                  : 'Audio playback failed'}
                </div>
                <div className="text-foreground-alt max-w-md text-xs">
                  {state.message}
                </div>
              </div>
            )}

            <Player.Provider>
              <AudioSkin
                className="flex min-h-0 w-full items-center rounded-none"
                style={audioSkinStyle}
              >
                <Audio
                  aria-label={title}
                  className={cn(
                    'w-full',
                    state.status === 'error' && 'opacity-20',
                  )}
                  data-testid="unixfs-audio-element"
                  onCanPlay={handleReady}
                  onError={handleError}
                  onLoadedMetadata={handleReady}
                  onPlaying={handleReady}
                  onSeeked={handleReady}
                  onSeeking={handleBuffering}
                  onWaiting={handleBuffering}
                  preload="metadata"
                  src={inlineFileURL}
                />
              </AudioSkin>
            </Player.Provider>
          </div>
        </div>
      </div>
    </div>
  )
}

// UnixFSAudioFileViewer renders a dedicated inline preview surface for audio files.
export function UnixFSAudioFileViewer(props: UnixFSAudioFileViewerProps) {
  return <UnixFSAudioPlayerSurface key={props.inlineFileURL} {...props} />
}
