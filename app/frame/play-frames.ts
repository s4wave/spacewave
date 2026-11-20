// FrameSourceEvent is one event of a frame track: the track opened on the
// Device, or one encoded picture arrived.
export type FrameSourceEvent =
  | { kind: 'opened' }
  | { kind: 'frame'; data: Uint8Array; mimeType: string }

// FrameSource opens a frame track and yields its events until signal aborts.
// The track has failed when the iterable throws, and closed when it ends.
export type FrameSource = (
  signal: AbortSignal,
) => AsyncIterable<FrameSourceEvent>

// FramePlayEvent is what playing a track reports: the track opened, a
// picture was drawn, no picture came for stallMs, or the track failed.
export type FramePlayEvent =
  | { kind: 'opened' }
  | { kind: 'drawn' }
  | { kind: 'stall' }
  | { kind: 'failed'; detail: string }

// stallMs is how long a track goes without a picture before it reports a
// stall.
const stallMs = 1000

// watchPixelRatio calls onChange with the device pixels per CSS pixel now
// and each time it changes, as when the window moves to another screen or
// zooms, until the returned function stops it.
function watchPixelRatio(onChange: (ratio: number) => void): () => void {
  let query: MediaQueryList | null = null
  const check = () => {
    query?.removeEventListener('change', check)
    const ratio = window.devicePixelRatio || 1
    onChange(ratio)
    query = window.matchMedia(`(resolution: ${ratio}dppx)`)
    query.addEventListener('change', check)
  }
  check()
  return () => query?.removeEventListener('change', check)
}

// playFrames opens source and draws its pictures on canvas, fitted to pane
// at the picture's aspect and the screen's pixel ratio, until the returned
// function stops it; emit hears nothing after that. It decodes only the
// newest picture that arrived, one at a time, and draws once per animation
// frame, so a slow decode drops pictures rather than queueing them. The last
// picture stays drawn until a newer one replaces it.
export function playFrames(
  source: FrameSource,
  canvas: HTMLCanvasElement,
  pane: HTMLElement,
  emit: (event: FramePlayEvent) => void,
): () => void {
  // stopped is set once the player stops, after which nothing is drawn or
  // reported.
  const abort = new AbortController()
  let stopped = false
  const report = (event: FramePlayEvent) => {
    if (!stopped) emit(event)
  }

  // shown is the picture on the canvas, kept to redraw on a resize.
  // pending is the newest undecoded picture; decoding is set while one
  // decodes.
  let shown: ImageBitmap | null = null
  let pending: { data: Uint8Array; mimeType: string } | null = null
  let decoding = false
  let ratio = window.devicePixelRatio || 1
  let frame = 0
  let stall = 0

  // draw fits the canvas to the pane and draws the shown picture on the
  // next animation frame, once however many changes arrive before it.
  const draw = () => {
    if (frame) return
    frame = requestAnimationFrame(() => {
      // Fit the picture's aspect inside the pane.
      frame = 0
      if (!shown) return
      const fit = Math.min(
        pane.clientWidth / shown.width,
        pane.clientHeight / shown.height,
      )
      const width = shown.width * fit
      const height = shown.height * fit
      if (!width || !height) return

      // Size the backing store in device pixels and draw.
      canvas.style.width = `${width}px`
      canvas.style.height = `${height}px`
      canvas.width = Math.round(width * ratio)
      canvas.height = Math.round(height * ratio)
      canvas
        .getContext('2d')
        ?.drawImage(shown, 0, 0, canvas.width, canvas.height)
    })
  }

  // decode turns the newest pending picture into the shown one, then takes
  // the next that arrived meanwhile, and restarts the stall deadline.
  const decode = async () => {
    if (decoding) return
    decoding = true
    while (pending && !stopped) {
      // Decode the newest picture; one that fails is dropped.
      const next = pending
      pending = null
      // The track's bytes are never in a SharedArrayBuffer.
      const blob = new Blob([next.data as BlobPart], { type: next.mimeType })
      const bitmap = await createImageBitmap(blob).catch(() => null)
      if (!bitmap) continue
      if (stopped) {
        bitmap.close()
        break
      }

      // Show it and wait for the next.
      shown?.close()
      shown = bitmap
      draw()
      report({ kind: 'drawn' })
      clearTimeout(stall)
      stall = window.setTimeout(() => report({ kind: 'stall' }), stallMs)
    }
    decoding = false
  }

  // Refit on a pane resize or a pixel ratio change.
  const observer = new ResizeObserver(draw)
  observer.observe(pane)
  const stopRatio = watchPixelRatio((next) => {
    ratio = next
    draw()
  })

  // Read the track until it ends or fails.
  void (async () => {
    let opened = false
    try {
      for await (const event of source(abort.signal)) {
        if (stopped) return
        if (!opened) {
          opened = true
          report({ kind: 'opened' })
        }
        if (event.kind === 'frame') {
          pending = { data: event.data, mimeType: event.mimeType }
          void decode()
        }
      }
      report({ kind: 'failed', detail: 'The Device closed the track.' })
    } catch (err) {
      report({
        kind: 'failed',
        detail: err instanceof Error ? err.message : String(err),
      })
    }
  })()

  return () => {
    // Stop drawing and close the track.
    stopped = true
    abort.abort()
    observer.disconnect()
    stopRatio()
    cancelAnimationFrame(frame)
    clearTimeout(stall)
    shown?.close()
    shown = null
  }
}
