import {
  type CSSProperties,
  type SyntheticEvent,
  useCallback,
  useState,
} from 'react'
import { LuTriangleAlert } from 'react-icons/lu'
import { createPlayer } from '@videojs/react'
import { Video, videoFeatures, VideoSkin } from '@videojs/react/video'
import '@videojs/react/video/skin.css'

import { cn } from '@s4wave/web/style/utils.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

// UnixFSVideoFileViewerProps are the props passed to the UnixFSVideoFileViewer.
export interface UnixFSVideoFileViewerProps {
  // title is the accessible label for the media element.
  title: string
  // inlineFileURL is the projected raw file URL used for playback.
  inlineFileURL: string
}

const Player = createPlayer({
  displayName: 'UnixFSVideoFileViewer',
  features: videoFeatures,
})

const mediaErrAborted = 1
const mediaErrNetwork = 2
const mediaErrDecode = 3
const mediaErrSrcNotSupported = 4

interface VideoPreviewState {
  status: 'loading' | 'ready' | 'error'
  buffering: boolean
  message?: string
  unsupported?: boolean
}

const initialVideoPreviewState: VideoPreviewState = {
  status: 'loading',
  buffering: false,
}

const videoSkinStyle: CSSProperties = {
  '--media-border-radius': '0px',
  borderRadius: 0,
}

function buildVideoErrorState(error: MediaError | null): VideoPreviewState {
  if (error?.code === mediaErrSrcNotSupported) {
    return {
      status: 'error',
      buffering: false,
      unsupported: true,
      message: 'This browser cannot play this video file.',
    }
  }
  if (error?.code === mediaErrDecode) {
    return {
      status: 'error',
      buffering: false,
      message: 'This video file could not be decoded.',
    }
  }
  if (error?.code === mediaErrNetwork) {
    return {
      status: 'error',
      buffering: false,
      message: 'This video file stopped loading unexpectedly.',
    }
  }
  if (error?.code === mediaErrAborted) {
    return {
      status: 'error',
      buffering: false,
      message: 'Video playback was aborted before the preview became ready.',
    }
  }
  return {
    status: 'error',
    buffering: false,
    message: 'This video file failed before playback could start.',
  }
}

function UnixFSVideoPlayerSurface({
  title,
  inlineFileURL,
}: UnixFSVideoFileViewerProps) {
  const [state, setState] = useState<VideoPreviewState>(
    initialVideoPreviewState,
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

  const handleError = useCallback((event: SyntheticEvent<HTMLVideoElement>) => {
    setState(buildVideoErrorState(event.currentTarget.error))
  }, [])

  const showLoading = state.status === 'loading'
  const showBuffering = state.status === 'ready' && state.buffering

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <div className="relative flex min-h-0 flex-1 items-center justify-center bg-black/80">
        {showLoading && (
          <div
            data-testid="unixfs-video-loading"
            className="bg-background/72 absolute inset-0 z-20 flex items-center justify-center p-6 backdrop-blur-sm"
          >
            <div className="w-full max-w-sm">
              <LoadingCard
                view={{
                  state: 'active',
                  title: 'Loading preview',
                  detail: 'Waiting for video metadata.',
                }}
              />
            </div>
          </div>
        )}

        {state.status === 'error' && (
          <div
            data-testid="unixfs-video-error"
            className="bg-background/82 absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 p-6 text-center backdrop-blur-sm"
          >
            <div className="bg-destructive/10 text-destructive flex h-10 w-10 items-center justify-center rounded-full">
              <LuTriangleAlert className="h-5 w-5" />
            </div>
            <div className="text-foreground text-sm font-semibold">
              {state.unsupported ?
                'Video preview unavailable'
              : 'Video playback failed'}
            </div>
            <div className="text-foreground-alt max-w-md text-xs">
              {state.message}
            </div>
          </div>
        )}

        {showBuffering && (
          <div className="border-foreground/10 bg-background/78 absolute top-4 right-4 z-20 flex items-center rounded-full border px-3 py-1.5 backdrop-blur-sm">
            <LoadingInline label="Buffering preview" tone="muted" size="sm" />
          </div>
        )}

        <Player.Provider>
          <VideoSkin
            className="flex h-full min-h-0 w-full items-center justify-center rounded-none"
            style={videoSkinStyle}
          >
            <Video
              aria-label={title}
              className={cn(
                'h-full max-h-full w-full max-w-full bg-black object-contain',
                state.status === 'error' && 'opacity-20',
              )}
              data-testid="unixfs-video-element"
              onCanPlay={handleReady}
              onError={handleError}
              onLoadedMetadata={handleReady}
              onPlaying={handleReady}
              onSeeked={handleReady}
              onSeeking={handleBuffering}
              onWaiting={handleBuffering}
              playsInline
              preload="metadata"
              src={inlineFileURL}
            />
          </VideoSkin>
        </Player.Provider>
      </div>
    </div>
  )
}

// UnixFSVideoFileViewer renders a dedicated inline preview surface for video files.
export function UnixFSVideoFileViewer(props: UnixFSVideoFileViewerProps) {
  return <UnixFSVideoPlayerSurface key={props.inlineFileURL} {...props} />
}
