import { pushable, type Pushable } from 'it-pushable'
import { useEffect, useReducer, useRef, useState } from 'react'
import type { MessageStream } from 'starpc'

import type {
  WasiVmDisplayInput,
  WasiVmScene,
} from '@s4wave/sdk/vm/wasivm.pb.js'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

import { openCanvasRenderer } from './display-canvas.js'
import { framePainter } from './display-frame.js'
import { openGpuRenderer } from './display-gpu.js'
import { keysymOf, xButtons } from './display-input.js'
import { Scene, type SceneRenderer, type SceneScreen } from './display-scene.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    minHeight: 0,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    backgroundColor: 'var(--color-black)',
  },
  rule2: {
    maxHeight: '100%',
    maxWidth: '100%',
    touchAction: 'none',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule3: { visibility: 'hidden' },
})

const stylexStyles = stylex.create({
  rule1: {
    position: 'absolute',
    width: '100%',
    maxWidth: 'var(--container-sm)',
  },
})

// WasiVmDisplayConnector opens the display stream of a running VM.
export type WasiVmDisplayConnector = (
  input: MessageStream<WasiVmDisplayInput>,
  signal: AbortSignal,
) => MessageStream<WasiVmScene>

// WasiVmPaneSize is the size of the pane a display fits in, in CSS pixels.
export interface WasiVmPaneSize {
  width: number
  height: number
}

// WasiVmDisplayViews describes a display that is not showing: waiting for
// its first scene, closed by the VM, or failed. retry reconnects.
export interface WasiVmDisplayViews {
  waiting: LoadingView
  closed: (retry: () => void) => LoadingView
  failed: (detail: string, retry: () => void) => LoadingView
}

// WasiVmDisplayProps configures the display of a VM.
export interface WasiVmDisplayProps {
  connectDisplay?: WasiVmDisplayConnector
  // views replaces the views of the desktop display.
  views?: WasiVmDisplayViews
  // onPaneSize receives the pane's size when it is first measured and each
  // time it changes.
  onPaneSize?: (size: WasiVmPaneSize) => void
}

// desktopViews describes the desktop display while it is not showing.
const desktopViews: WasiVmDisplayViews = {
  waiting: {
    state: 'active',
    title: 'Waiting for the display',
    detail: 'The desktop appears once the VM starts its display.',
  },
  closed: (retry) => ({
    state: 'error',
    title: 'The display disconnected',
    detail: 'The display closed.',
    onRetry: retry,
  }),
  failed: (detail, retry) => ({
    state: 'error',
    title: 'The display disconnected',
    detail,
    onRetry: retry,
  }),
}

// DisplayStatus is the state of the display stream: waiting for the first
// scene, showing the screen, closed by the VM, or failed with a reason.
type DisplayStatus =
  | { kind: 'waiting' }
  | { kind: 'ready' }
  | { kind: 'closed' }
  | { kind: 'failed'; detail: string }

// wheelUp and wheelDown are the masks of X buttons 4 and 5, which a wheel
// step presses and releases to scroll up and down.
const wheelUp = 1 << 3
const wheelDown = 1 << 4

// pressBits is the mask of the X buttons whose press acts on a window frame.
const pressBits = 7

// settleMs is how long a move the viewer predicted from a title bar press
// keeps its place after the release when the compositor shows no change,
// as when the press never reached a title bar there.
const settleMs = 500

// gpuFailed is set once a WebGPU device is lost and cannot be opened again,
// after which displays draw with Canvas 2D.
let gpuFailed = false

// openRenderer opens a WebGPU renderer on canvas, or a Canvas 2D one when
// WebGPU is missing, failed before, or cannot open a device.
async function openRenderer(
  canvas: HTMLCanvasElement,
  onLost: () => void,
): Promise<SceneRenderer | null> {
  if (!gpuFailed && 'gpu' in navigator) {
    const gpu = await openGpuRenderer(canvas, onLost).catch(() => null)
    if (gpu) return gpu
  }
  return openCanvasRenderer(canvas)
}

// watchPixelRatio calls onChange with the device pixels per CSS pixel,
// rounded up, now and each time it changes, as when the window moves to
// another screen or zooms, until the returned function stops it.
function watchPixelRatio(onChange: (scale: number) => void): () => void {
  let query: MediaQueryList | null = null
  const check = () => {
    // Report the ratio and watch for the next change.
    query?.removeEventListener('change', check)
    const ratio = window.devicePixelRatio || 1
    onChange(Math.max(1, Math.ceil(ratio - 0.001)))
    query = window.matchMedia(`(resolution: ${ratio}dppx)`)
    query.addEventListener('change', check)
  }
  check()
  return () => query?.removeEventListener('change', check)
}

// fitCanvas shows the canvas at the screen's size in units, which are CSS
// pixels, shrunk to fit the pane.
function fitCanvas(
  canvas: HTMLCanvasElement,
  pane: HTMLElement,
  screen: SceneScreen,
) {
  // Scale the screen's size down to the pane, never up.
  const { width, height } = screen
  if (!width || !height) return
  const fit = Math.min(1, pane.clientWidth / width, pane.clientHeight / height)
  canvas.style.width = `${width * fit}px`
  canvas.style.height = `${height * fit}px`
}

// WasiVmDisplay shows the display of a running VM, X or Wayland, or one
// window of it, on a canvas scaled to fit the pane. Pointer, wheel and key
// input on the canvas drive the VM.
export function WasiVmDisplay({
  connectDisplay,
  views = desktopViews,
  onPaneSize,
}: WasiVmDisplayProps) {
  // Hold the elements, the input stream, the last scene's screen, the
  // scene's pointer and buttons, and each held key's keysym by code, so a
  // release sends the keysym its press sent after a modifier changes.
  const paneRef = useRef<HTMLDivElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const inputRef = useRef<Pushable<WasiVmDisplayInput> | null>(null)
  const screenRef = useRef<SceneScreen>(new Scene().screen)
  const pointRef = useRef<
    ((x: number, y: number, buttons: number) => void) | null
  >(null)
  const heldKeys = useRef(new Map<string, number>())

  // Track the stream's status and the connection attempt Retry advances.
  const [status, setStatus] = useState<DisplayStatus>({ kind: 'waiting' })
  const [attempt, retry] = useReducer((n: number) => n + 1, 0)

  // Fit the canvas to the pane and report the pane's size as it changes.
  useEffect(() => {
    // Watch the pane.
    const pane = paneRef.current
    if (!pane) return
    let last = ''
    const observer = new ResizeObserver(() => {
      // Refit the canvas.
      if (canvasRef.current) {
        fitCanvas(canvasRef.current, pane, screenRef.current)
      }

      // Report a new pane size once.
      const width = Math.floor(pane.clientWidth)
      const height = Math.floor(pane.clientHeight)
      const key = width + 'x' + height
      if (!onPaneSize || !width || !height || key === last) return
      last = key
      onPaneSize({ width, height })
    })
    observer.observe(pane)
    return () => observer.disconnect()
  }, [onPaneSize])

  // Stream the scene and draw it on the canvas each animation frame it
  // changes.
  useEffect(() => {
    // Open the input stream.
    const canvas = canvasRef.current
    const pane = paneRef.current
    if (!connectDisplay || !canvas || !pane) return
    const abort = new AbortController()
    const input = pushable<WasiVmDisplayInput>({ objectMode: true })
    inputRef.current = input
    setStatus({ kind: 'waiting' })

    // Report the screen's pixel ratio, which the VM draws at.
    const stopScale = watchPixelRatio((scale) =>
      input.push({ body: { case: 'scale', value: { scale } } }),
    )

    // cancelled is set once the effect is cleaned up, after which no state
    // is set and no renderer is kept.
    let cancelled = false
    const fail = (detail: string) => {
      if (!cancelled) setStatus({ kind: 'failed', detail })
    }

    // draw draws the scene on the next animation frame, once however many
    // updates arrive before it.
    const scene = new Scene(framePainter(canvas))
    let renderer: SceneRenderer | null = null
    let frame = 0
    const draw = () => {
      if (frame) return
      frame = requestAnimationFrame(() => {
        frame = 0
        renderer?.draw(scene)
      })
    }

    // Give the scene each pointer position and button change sent, and
    // redraw when a window moves with it, without waiting for the
    // compositor. A first press on a title bar starts the move here; after
    // the release, the move holds until the compositor shows the result or
    // settleMs passes.
    let held = 0
    let settle = 0
    pointRef.current = (x, y, buttons) => {
      // Move the pointer and act on the buttons that changed.
      buttons &= pressBits
      let changed = scene.point(x, y)
      if (!held && buttons) {
        changed = scene.press() || changed
      }
      if (held && !buttons) {
        scene.release()
        clearTimeout(settle)
        settle = window.setTimeout(() => {
          if (scene.settle()) draw()
        }, settleMs)
      }
      held = buttons
      if (changed) draw()

      // Show the frame's cursor while no button is held.
      if (!buttons) canvas.style.cursor = scene.cursorAt()
    }

    // Open a renderer. A lost GPU device is reopened once and every
    // texture uploaded again from the scene; a second loss fails the
    // display, and Retry draws with Canvas 2D on a new canvas.
    const open = async (onLost: () => void) => {
      // Keep the renderer only while the effect lasts.
      const r = await openRenderer(canvas, onLost)
      if (cancelled) {
        r?.destroy()
        return
      }
      if (!r) {
        fail('This browser cannot draw the display.')
        return
      }

      // Draw the whole scene with it.
      renderer = r
      scene.allDirty()
      draw()
    }
    const lostTwice = () => {
      gpuFailed = true
      renderer?.destroy()
      renderer = null
      fail('The graphics device was lost.')
    }
    void open(() => {
      renderer?.destroy()
      renderer = null
      void open(lostTwice)
    })

    // Apply each update until the stream ends.
    void (async () => {
      let ready = false
      try {
        for await (const update of connectDisplay(input, abort.signal)) {
          // Fold the update into the scene, refit the canvas when the
          // screen changed, and draw on the next frame.
          scene.apply(update)
          if (update.screen) {
            screenRef.current = scene.screen
            fitCanvas(canvas, pane, scene.screen)
          }
          draw()

          // Take the next update, which holds every change made meanwhile.
          input.push({ body: { case: 'drawn', value: {} } })
          if (!ready && !cancelled) {
            ready = true
            setStatus({ kind: 'ready' })
          }
        }
        if (!cancelled) {
          setStatus({ kind: 'closed' })
        }
      } catch (err) {
        fail(err instanceof Error ? err.message : String(err))
      }
    })()

    return () => {
      // Stop drawing.
      cancelled = true
      pointRef.current = null
      clearTimeout(settle)
      cancelAnimationFrame(frame)
      renderer?.destroy()
      renderer = null

      // Close the streams.
      stopScale()
      abort.abort()
      input.end()
      inputRef.current = null
    }
  }, [connectDisplay, attempt])

  // send queues one input event for the VM.
  const send = (body: WasiVmDisplayInput['body']) => {
    inputRef.current?.push({ body })
  }

  // sendPointer sends the pointer's position on the screen and the held
  // buttons, with extra X buttons added, and gives the scene the position.
  const sendPointer = (
    e: { clientX: number; clientY: number; buttons: number },
    extra = 0,
  ) => {
    // Map the client position to screen pixels.
    const canvas = canvasRef.current
    if (!canvas) return
    const rect = canvas.getBoundingClientRect()
    if (!rect.width || !rect.height) return
    const { width, height, scale } = screenRef.current
    const x = Math.floor(((e.clientX - rect.left) * width * scale) / rect.width)
    const y = Math.floor(
      ((e.clientY - rect.top) * height * scale) / rect.height,
    )

    // Send it, and move a grabbed window there on the next frame.
    const buttons = xButtons(e.buttons) | extra
    send({ case: 'pointer', value: { x, y, buttons } })
    pointRef.current?.(x, y, buttons)
  }

  // releasePointer releases every button where an interrupted drag left the
  // pointer.
  const releasePointer = (e: { clientX: number; clientY: number }) => {
    sendPointer({ clientX: e.clientX, clientY: e.clientY, buttons: 0 })
  }

  // releaseKeys releases every held key, as when the canvas loses focus.
  const releaseKeys = () => {
    for (const keysym of heldKeys.current.values()) {
      send({ case: 'key', value: { keysym, down: false } })
    }
    heldKeys.current.clear()
  }

  return (
    <div ref={paneRef} sx={stylexStyles2.rule1}>
      <canvas
        key={attempt}
        ref={canvasRef}
        tabIndex={0}
        aria-label="VM display"
        sx={[
          stylexStyles2.rule2,
          status.kind !== 'ready' && stylexStyles2.rule3,
        ]}
        onContextMenu={(e) => e.preventDefault()}
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture(e.pointerId)
          e.currentTarget.focus()
          sendPointer(e)
        }}
        onPointerMove={(e) => sendPointer(e)}
        onPointerUp={(e) => sendPointer(e)}
        onPointerCancel={releasePointer}
        onLostPointerCapture={releasePointer}
        onWheel={(e) => {
          if (!e.deltaY) return
          sendPointer(e, e.deltaY < 0 ? wheelUp : wheelDown)
          sendPointer(e)
        }}
        onKeyDown={(e) => {
          // Send the key's keysym and remember it for the release.
          const keysym = heldKeys.current.get(e.code) ?? keysymOf(e.key, e.code)
          if (!keysym) return
          e.preventDefault()
          heldKeys.current.set(e.code, keysym)
          send({ case: 'key', value: { keysym, down: true } })
        }}
        onKeyUp={(e) => {
          // Release the keysym the press sent.
          const keysym = heldKeys.current.get(e.code)
          if (keysym === undefined) return
          e.preventDefault()
          heldKeys.current.delete(e.code)
          send({ case: 'key', value: { keysym, down: false } })
        }}
        onBlur={releaseKeys}
      />
      {status.kind !== 'ready' && (
        <LoadingCard
          sx={stylexStyles.rule1}
          view={
            status.kind === 'waiting'
              ? views.waiting
              : status.kind === 'closed'
                ? views.closed(retry)
                : views.failed(status.detail, retry)
          }
        />
      )}
    </div>
  )
}
