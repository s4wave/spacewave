import {
  useEffect,
  useReducer,
  useRef,
  type KeyboardEvent,
  type PointerEvent,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import { FrameInput } from './frame-input.js'
import {
  type FramePlayEvent,
  type FrameSource,
  playFrames,
} from './play-frames.js'

// FrameTrack is the frame track's state as defined by FrameViewer.tla, for a
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
// unhandled event leaves the state unchanged.
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
  // label identifies the picture for assistive technology.
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
  const inputRef = useRef<FrameInput | null>(null)
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
    if (source.kind === 'view') {
      return playFrames(source.open, canvas, pane, dispatch)
    }

    // Bind the event queue to this connection and release it before retrying.
    const input = new FrameInput()
    inputRef.current = input
    const stop = playFrames(
      (signal) => {
        signal.addEventListener('abort', () => input.close(), { once: true })
        return source.open(input, signal)
      },
      canvas,
      pane,
      dispatch,
    )
    return () => {
      inputRef.current = null
      stop()
    }
  }, [source, attempt])

  // Input is exposed only while this Display has live pictures.
  const interactive = source?.kind === 'control' && track.kind === 'live'
  const sendPointer = (event: PointerEvent<HTMLCanvasElement>) => {
    // Use the picture's rectangle, which excludes the letterbox around it.
    const canvas = event.currentTarget
    const bounds = canvas.getBoundingClientRect()
    if (!bounds.width || !bounds.height) return
    const x = Math.min(
      1,
      Math.max(0, (event.clientX - bounds.left) / bounds.width),
    )
    const y = Math.min(
      1,
      Math.max(0, (event.clientY - bounds.top) / bounds.height),
    )

    // Keep movement and button releases over this surface while dragging.
    if (event.type === 'pointerdown') {
      canvas.focus({ preventScroll: true })
      canvas.setPointerCapture(event.pointerId)
    }

    // A canceled pointer releases its buttons at the latest position.
    const buttons = event.type === 'pointercancel' ? 0 : event.buttons
    inputRef.current?.push(
      { event: { case: 'pointer', value: { x, y, buttons } } },
      event.type === 'pointermove' ? buttons : undefined,
    )
  }

  const sendKey = (event: KeyboardEvent<HTMLCanvasElement>) => {
    // Keep Tab available for leaving the display in the local interface.
    if (event.key === 'Tab') return
    event.preventDefault()
    event.stopPropagation()
    inputRef.current?.push({
      event: {
        case: 'key',
        value: {
          code: event.code,
          key: event.key,
          pressed: event.type === 'keydown',
          repeat: event.repeat,
        },
      },
    })
  }

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
        role={interactive ? 'application' : 'img'}
        aria-label={label}
        tabIndex={interactive ? 0 : undefined}
        onPointerMove={interactive ? sendPointer : undefined}
        onPointerDown={interactive ? sendPointer : undefined}
        onPointerUp={interactive ? sendPointer : undefined}
        onPointerCancel={interactive ? sendPointer : undefined}
        onKeyDown={interactive ? sendKey : undefined}
        onKeyUp={interactive ? sendKey : undefined}
        onContextMenu={
          interactive ? (event) => event.preventDefault() : undefined
        }
        className={cn(
          view && 'invisible',
          interactive &&
            'focus-visible:outline-primary touch-none focus-visible:outline-2 focus-visible:-outline-offset-2',
        )}
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
