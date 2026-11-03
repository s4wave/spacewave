import {
  type CSSProperties,
  type SyntheticEvent,
  useCallback,
  useState,
} from 'react'
import { LuMusic, LuTriangleAlert } from '@s4wave/web/images/icons.js'
import { createPlayer } from '@videojs/react'
import { Audio, audioFeatures, AudioSkin } from '@videojs/react/audio'
import '@videojs/react/audio/skin.css'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { display: 'flex', minHeight: 0, flex: 1, overflow: 'hidden' },
  rule2: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    position: 'relative',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-2xl)',
    flexDirection: 'column',
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule6: { minWidth: 0, flex: 1 },
  rule7: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 78%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    position: 'relative',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
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
  rule11: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule12: {
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
  rule13: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule15: {
    maxWidth: 'var(--container-md)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule16: {
    display: 'flex',
    minHeight: 0,
    width: '100%',
    alignItems: 'center',
    borderRadius: 0,
  },
  rule17: { width: '100%' },
  rule18: { opacity: 0.2 },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>
            <div sx={stylexStyles2.rule5}>
              <LuMusic sx={stylexStyles.rule1} />
            </div>
            <div sx={stylexStyles2.rule6}>
              <div sx={stylexStyles2.rule7}>{title}</div>
            </div>
            {showBuffering && (
              <div sx={stylexStyles2.rule8}>
                <LoadingInline label="Buffering" tone="muted" size="sm" />
              </div>
            )}
          </div>

          <div sx={stylexStyles2.rule9}>
            {showLoading && (
              <div data-testid="unixfs-audio-loading" sx={stylexStyles2.rule10}>
                <div sx={stylexStyles2.rule11}>
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
              <div data-testid="unixfs-audio-error" sx={stylexStyles2.rule12}>
                <div sx={stylexStyles2.rule13}>
                  <LuTriangleAlert sx={stylexStyles.rule2} />
                </div>
                <div sx={stylexStyles2.rule14}>
                  {state.unsupported
                    ? 'Audio preview unavailable'
                    : 'Audio playback failed'}
                </div>
                <div sx={stylexStyles2.rule15}>{state.message}</div>
              </div>
            )}

            <Player.Player>
              <AudioSkin
                {...stylex.props(stylexStyles2.rule16)}
                style={audioSkinStyle}
              >
                <Audio
                  aria-label={title}
                  {...stylex.props([
                    stylexStyles2.rule17,
                    state.status === 'error' && stylexStyles2.rule18,
                  ])}
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
            </Player.Player>
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
