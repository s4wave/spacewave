import {
  type CSSProperties,
  type SyntheticEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { LuDownload } from 'react-icons/lu'
import { createPlayer } from '@videojs/react'
import { Video, videoFeatures, VideoSkin } from '@videojs/react/video'
import '@videojs/react/video/skin.css'

import {
  ViewerFloat,
  ViewerFloatLink,
  ViewerStage,
} from '@s4wave/web/object/ViewerFloat.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
import { cn } from '@s4wave/web/style/utils.js'
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

const mediaErrDecode = 3
const mediaErrSrcNotSupported = 4
const mediaHaveMetadata = 1

// VideoPreviewState is what the video shows: loading until its metadata
// arrives, ready (buffering while a seek or stall waits for data), or failed
// with what went wrong and whether loading again can succeed.
type VideoPreviewState =
  | { status: 'loading' }
  | { status: 'ready'; buffering: boolean }
  | { status: 'failed'; message: string; retryable: boolean }

const loadingState: VideoPreviewState = { status: 'loading' }

const videoSkinStyle: CSSProperties = {
  '--media-border-radius': '0px',
}

// videoFailure says what a media error means for the person. A file this
// browser cannot decode or play fails the same way on every load, so it points
// at Download; a stopped or aborted load can succeed when loaded again.
function videoFailure(error: MediaError | null): VideoPreviewState {
  if (error?.code === mediaErrSrcNotSupported) {
    return {
      status: 'failed',
      message:
        'This browser cannot play this video format. Download it to open it elsewhere.',
      retryable: false,
    }
  }
  if (error?.code === mediaErrDecode) {
    return {
      status: 'failed',
      message:
        'This video could not be decoded. Download it to open it elsewhere.',
      retryable: false,
    }
  }
  return {
    status: 'failed',
    message: 'The video stopped loading.',
    retryable: true,
  }
}

function UnixFSVideoPlayerSurface({
  title,
  inlineFileURL,
}: UnixFSVideoFileViewerProps) {
  // The video element, and what it shows.
  const mediaRef = useRef<HTMLVideoElement | null>(null)
  const [state, setState] = useState<VideoPreviewState>(loadingState)

  // resumeAt is where play stood when the video failed; a Retry's load starts
  // at 0, so the metadata seeks back here once.
  const resumeAt = useRef(0)

  // playing moves the stage's media, so the floats leave while the video
  // plays and the pointer is still, as the video's own controls do.
  const [playing, setPlaying] = useState(false)

  // handleReady marks the video ready once its metadata or data arrives.
  const handleReady = useCallback(() => {
    setState((prev) =>
      prev.status === 'ready' && !prev.buffering
        ? prev
        : { status: 'ready', buffering: false },
    )
  }, [])

  // handleLoadStart returns to loading when the element loads its source.
  const handleLoadStart = useCallback(() => {
    setState((prev) => (prev.status === 'loading' ? prev : loadingState))
  }, [])

  // handleBuffering marks a ready video waiting for data; before the metadata
  // arrives the loading state already says so.
  const handleBuffering = useCallback(() => {
    setState((prev) =>
      prev.status === 'ready' && !prev.buffering
        ? { status: 'ready', buffering: true }
        : prev,
    )
  }, [])

  // handleError stops play and shows what the element's error means.
  const handleError = useCallback((event: SyntheticEvent<HTMLVideoElement>) => {
    const node = event.currentTarget
    resumeAt.current = node.currentTime || resumeAt.current
    setPlaying(false)
    setState(videoFailure(node.error))
  }, [])

  // handleLoadedMetadata seeks back to where a failed play stood, then marks
  // the video ready.
  const handleLoadedMetadata = useCallback(
    (event: SyntheticEvent<HTMLVideoElement>) => {
      if (resumeAt.current > 0) {
        event.currentTarget.currentTime = resumeAt.current
        resumeAt.current = 0
      }
      handleReady()
    },
    [handleReady],
  )

  // handleRetry loads the source again; the element fires loadstart and then
  // the metadata or another error, as on the first load.
  const handleRetry = useCallback(() => {
    setState(loadingState)
    mediaRef.current?.load()
  }, [])

  // handleVideoRef keeps the element, ready at once if it has metadata.
  const handleVideoRef = useCallback(
    (node: HTMLVideoElement | null) => {
      mediaRef.current = node
      if (node && node.readyState >= mediaHaveMetadata) {
        handleReady()
      }
    },
    [handleReady],
  )

  // Mark a video whose metadata arrived before the handlers ready.
  useEffect(() => {
    const node = mediaRef.current
    if (node && node.readyState >= mediaHaveMetadata) {
      handleReady()
    }
  }, [handleReady, inlineFileURL])

  // failed hides the player and keeps the stage for its floats.
  const failed = state.status === 'failed'

  return (
    <>
      {state.status === 'loading' && (
        <ViewerStatus state="waiting" what="the video" rows={0} />
      )}
      {state.status === 'failed' && (
        <ViewerStatus
          state="failed"
          message={state.message}
          retry={state.retryable ? handleRetry : undefined}
        />
      )}
      <ViewerStage
        media={failed ? 'failed' : playing ? 'moving' : 'still'}
        className="items-center justify-center bg-black/80"
      >
        {state.status === 'ready' && state.buffering && (
          <div className="border-foreground/10 bg-background/78 absolute top-4 right-4 z-20 flex items-center rounded-full border px-3 py-1.5 backdrop-blur-sm">
            <LoadingInline label="Buffering preview" tone="muted" size="sm" />
          </div>
        )}

        <Player.Player>
          <VideoSkin
            className={cn(
              'flex h-full min-h-0 w-full items-center justify-center rounded-none',
              failed && 'invisible',
            )}
            style={videoSkinStyle}
          >
            <Video
              aria-label={title}
              className="h-full max-h-full w-full max-w-full bg-black object-contain"
              data-testid="unixfs-video-element"
              onCanPlay={handleReady}
              onError={handleError}
              onLoadedMetadata={handleLoadedMetadata}
              onLoadStart={handleLoadStart}
              onEnded={() => setPlaying(false)}
              onPause={() => setPlaying(false)}
              onPlay={() => setPlaying(true)}
              onPlaying={handleReady}
              onSeeked={handleReady}
              onSeeking={handleBuffering}
              onWaiting={handleBuffering}
              playsInline
              preload="metadata"
              ref={handleVideoRef}
              src={inlineFileURL}
            />
          </VideoSkin>
        </Player.Player>

        <ViewerFloat at="tl" label="Video">
          <ViewerFloatLink
            label="Download"
            icon={<LuDownload aria-hidden />}
            essential
            href={inlineFileURL}
            download={title}
          />
        </ViewerFloat>
      </ViewerStage>
    </>
  )
}

// UnixFSVideoFileViewer renders a dedicated inline preview surface for video
// files: the video on a stage, with its waiting or failed state in a row above.
export function UnixFSVideoFileViewer(props: UnixFSVideoFileViewerProps) {
  return (
    <div
      data-testid="unixfs-video-viewer"
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
    >
      <UnixFSVideoPlayerSurface key={props.inlineFileURL} {...props} />
    </div>
  )
}
