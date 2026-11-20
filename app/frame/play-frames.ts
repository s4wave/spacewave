import {
  Codec,
  type Frame,
  type GroupHeader,
} from '@s4wave/core/frametrack/frametrack.pb.js'

import { AvcDecoder } from './avc-decoder.js'

// FrameSourceEvent is one event of a frame track: the track opened on the
// Device, or one encoded picture arrived.
export type FrameSourceEvent =
  | { kind: 'opened' }
  | { kind: 'frame'; header: GroupHeader; frame: Frame }

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
// function stops it; emit hears nothing after that. JPEG keeps the newest
// waiting picture. AVC feeds a bounded prefix to WebCodecs and recovers
// at the next keyframe after a drop. The last picture stays drawn until a
// newer one replaces it.
export function playFrames(
  source: FrameSource,
  canvas: HTMLCanvasElement,
  pane: HTMLElement,
  emit: (event: FramePlayEvent) => void,
): () => void {
  // The abort signal ends the stream and suppresses decoder callbacks.
  const abort = new AbortController()
  const report = (event: FramePlayEvent) => {
    if (!abort.signal.aborted) emit(event)
  }

  // shown is the picture on the canvas, kept to redraw on a resize.
  // pending is the newest undecoded picture; decoding is set while one
  // decodes.
  let shown: ImageBitmap | VideoFrame | null = null
  let pendingJPEG: Uint8Array | undefined
  let decodingJPEG = false
  let codec = Codec.UNKNOWN
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
      const [pictureWidth, pictureHeight] =
        'displayWidth' in shown
          ? [shown.displayWidth, shown.displayHeight]
          : [shown.width, shown.height]
      const fit = Math.min(
        pane.clientWidth / pictureWidth,
        pane.clientHeight / pictureHeight,
      )
      const width = pictureWidth * fit
      const height = pictureHeight * fit
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

  // Present the decoded picture and renew the deadline for the next one.
  const show = (picture: ImageBitmap | VideoFrame) => {
    shown?.close()
    shown = picture
    draw()
    report({ kind: 'drawn' })
    clearTimeout(stall)
    stall = window.setTimeout(() => report({ kind: 'stall' }), stallMs)
  }

  // Decode independent JPEGs one at a time, dropping superseded inputs.
  const decodeJPEG = async () => {
    if (decodingJPEG) return
    decodingJPEG = true
    while (pendingJPEG && !abort.signal.aborted) {
      // The generated frame codec returns an ordinary Uint8Array.
      const blob = new Blob([pendingJPEG as BlobPart], { type: 'image/jpeg' })
      pendingJPEG = undefined
      const bitmap = await createImageBitmap(blob).catch(() => null)
      if (!bitmap) continue

      // A picture that outlives its track or codec has no canvas to update.
      if (abort.signal.aborted || codec !== Codec.JPEG) bitmap.close()
      else show(bitmap)
    }
    decodingJPEG = false
  }

  // A missing browser codec is a track failure. A damaged group can recover.
  const avc = new AvcDecoder(show, (error) => {
    report({ kind: 'failed', detail: error.message })
    abort.abort()
  })

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
        if (abort.signal.aborted) return
        if (!opened) {
          opened = true
          report({ kind: 'opened' })
        }
        if (event.kind !== 'frame') continue

        // Forward the picture to the decoder named in the group header.
        codec = event.header.codec ?? Codec.UNKNOWN
        switch (codec) {
          case Codec.JPEG:
            avc.close()
            pendingJPEG = event.frame.data
            void decodeJPEG()
            break
          case Codec.H264_AVC:
            pendingJPEG = undefined
            avc.push(event.header, event.frame)
            break
          default:
            throw new Error(
              'The Device sent pictures in an unsupported format.',
            )
        }
      }
      report({ kind: 'failed', detail: 'The Device closed the track.' })
    } catch (err) {
      report({
        kind: 'failed',
        detail: err instanceof Error ? err.message : String(err),
      })
    } finally {
      abort.abort()
      avc.close()
    }
  })()

  return () => {
    // Stop drawing and close the track.
    abort.abort()
    avc.close()
    observer.disconnect()
    stopRatio()
    cancelAnimationFrame(frame)
    clearTimeout(stall)
    shown?.close()
    shown = null
  }
}
