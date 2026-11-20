import { useEffect, useReducer, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import {
  type FramePlayEvent,
  type FrameSource,
  playFrames,
} from './play-frames.js'

// FrameTrack is the frame track's state as FrameViewer.tla names it, for a
// mounted FrameView: the viewer mounts one only on an open handle and an
// online Device, so the spec's none is an unmounted FrameView.
type FrameTrack =
  | { kind: 'connecting' }
  | { kind: 'starting' }
  | { kind: 'live' }
  | { kind: 'stalled' }
  | { kind: 'failed'; detail: string }

// FrameViewState is the track and the connection attempt Retry advances.
interface FrameViewState {
  attempt: number
  track: FrameTrack
}

// frameViewReducer applies FrameViewer.tla's TrackStep and RetryTrack. An
// event a state does not take leaves it unchanged.
function frameViewReducer(
  state: FrameViewState,
  event: FramePlayEvent | { kind: 'retry' },
): FrameViewState {
  const { track } = state
  switch (event.kind) {
    case 'opened':
      return track.kind === 'connecting'
        ? { ...state, track: { kind: 'starting' } }
        : state
    case 'drawn':
      return track.kind === 'starting' || track.kind === 'stalled'
        ? { ...state, track: { kind: 'live' } }
        : state
    case 'stall':
      return track.kind === 'live'
        ? { ...state, track: { kind: 'stalled' } }
        : state
    case 'failed':
      return track.kind === 'failed'
        ? state
        : { ...state, track: { kind: 'failed', detail: event.detail } }
    case 'retry':
      return track.kind === 'failed'
        ? { attempt: state.attempt + 1, track: { kind: 'connecting' } }
        : state
  }
}

// trackView describes a track that shows no picture yet, or null once a
// picture is drawn.
function trackView(track: FrameTrack, retry: () => void): LoadingView | null {
  switch (track.kind) {
    case 'connecting':
      return { state: 'loading', title: 'Connecting to the Device' }
    case 'starting':
      return { state: 'active', title: 'Waiting for the first picture' }
    case 'failed':
      return {
        state: 'error',
        title: 'The picture stopped',
        detail: track.detail,
        onRetry: retry,
      }
    case 'live':
    case 'stalled':
      return null
  }
}

export interface FrameViewProps {
  // source opens the frame track. Without one the track stays connecting.
  source?: FrameSource
  // label names the picture for assistive technology.
  label: string
}

// FrameView draws a frame track's pictures on a canvas fitted to the pane,
// as FrameViewer.tla's track models it: connecting, waiting for the first
// picture, live, paused on the last picture when frames stop, or failed with
// Retry, which opens the track again on a fresh canvas.
export function FrameView({ source, label }: FrameViewProps) {
  // Hold the elements and the track's state.
  const paneRef = useRef<HTMLDivElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const [state, dispatch] = useReducer(frameViewReducer, {
    attempt: 0,
    track: { kind: 'connecting' },
  })
  const { attempt, track } = state

  // Play the track on the canvas for each attempt.
  useEffect(() => {
    const canvas = canvasRef.current
    const pane = paneRef.current
    if (!source || !canvas || !pane) return
    return playFrames(source, canvas, pane, dispatch)
  }, [source, attempt])

  // A track with a picture keeps it on the canvas, paused or live.
  const view = trackView(track, () => dispatch({ kind: 'retry' }))
  return (
    <div
      ref={paneRef}
      className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-black"
    >
      <canvas
        key={attempt}
        ref={canvasRef}
        role="img"
        aria-label={label}
        className={cn(view && 'invisible')}
      />
      {view ? (
        <LoadingCard className="absolute w-full max-w-sm" view={view} />
      ) : null}
      {track.kind === 'stalled' ? (
        <p
          role="status"
          className="bg-background/80 text-foreground-alt absolute top-2 left-2 rounded-sm px-2 py-1 text-xs"
        >
          Paused: last picture held until frames resume
        </p>
      ) : null}
    </div>
  )
}
