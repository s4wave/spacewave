import {
  type CSSProperties,
  type SyntheticEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { LuTriangleAlert } from '@s4wave/web/images/icons.js'
import { createPlayer } from '@videojs/react'
import { Video, videoFeatures, VideoSkin } from '@videojs/react/video'
import '@videojs/react/video/skin.css'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    minHeight: 0,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: 'color-mix(in oklab, var(--color-black) 80%, transparent)',
  },
  rule2: {
    position: 'absolute',
    inset: 0,
    zIndex: 20,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 72%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule3: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule4: {
    position: 'absolute',
    inset: 0,
    zIndex: 20,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 82%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule5: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    maxWidth: 'var(--container-md)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 4)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 20,
    display: 'flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 78%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule9: {
    display: 'flex',
    height: '100%',
    minHeight: 0,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 0,
  },
  rule10: {
    height: '100%',
    maxHeight: '100%',
    width: '100%',
    maxWidth: '100%',
    backgroundColor: 'var(--color-black)',
    objectFit: 'contain',
  },
  rule11: { opacity: 0.2 },
  rule12: { display: 'flex', minHeight: 0, flex: 1, overflow: 'hidden' },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

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
const mediaHaveMetadata = 1

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
  const mediaRef = useRef<HTMLVideoElement | null>(null)
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

  const handleLoadStart = useCallback(() => {
    setState((prev) => {
      if (prev.status === 'loading' && !prev.buffering) {
        return prev
      }
      return initialVideoPreviewState
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

  const handleVideoRef = useCallback(
    (node: HTMLVideoElement | null) => {
      mediaRef.current = node
      if (node && node.readyState >= mediaHaveMetadata) {
        handleReady()
      }
    },
    [handleReady],
  )

  useEffect(() => {
    const node = mediaRef.current
    if (node && node.readyState >= mediaHaveMetadata) {
      handleReady()
    }
  }, [handleReady, inlineFileURL])

  const showLoading = state.status === 'loading'
  const showBuffering = state.status === 'ready' && state.buffering

  return (
    <div sx={stylexStyles2.rule1}>
      {showLoading && (
        <div data-testid="unixfs-video-loading" sx={stylexStyles2.rule2}>
          <div sx={stylexStyles2.rule3}>
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
        <div data-testid="unixfs-video-error" sx={stylexStyles2.rule4}>
          <div sx={stylexStyles2.rule5}>
            <LuTriangleAlert sx={stylexStyles.rule1} />
          </div>
          <div sx={stylexStyles2.rule6}>
            {state.unsupported
              ? 'Video preview unavailable'
              : 'Video playback failed'}
          </div>
          <div sx={stylexStyles2.rule7}>{state.message}</div>
        </div>
      )}

      {showBuffering && (
        <div sx={stylexStyles2.rule8}>
          <LoadingInline label="Buffering preview" tone="muted" size="sm" />
        </div>
      )}

      <Player.Player>
        <VideoSkin
          {...stylex.props(stylexStyles2.rule9)}
          style={videoSkinStyle}
        >
          <Video
            aria-label={title}
            {...stylex.props([
              stylexStyles2.rule10,
              state.status === 'error' && stylexStyles2.rule11,
            ])}
            data-testid="unixfs-video-element"
            onCanPlay={handleReady}
            onError={handleError}
            onLoadedMetadata={handleReady}
            onLoadStart={handleLoadStart}
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
    </div>
  )
}

// UnixFSVideoFileViewer renders a dedicated inline preview surface for video files.
export function UnixFSVideoFileViewer(props: UnixFSVideoFileViewerProps) {
  return (
    <div data-testid="unixfs-video-viewer" sx={stylexStyles2.rule12}>
      <UnixFSVideoPlayerSurface key={props.inlineFileURL} {...props} />
    </div>
  )
}
