import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from 'react'
import { useGesture } from '@use-gesture/react'

import gridPattern from '@s4wave/web/images/patterns/grid.png'

import type { Viewport, CanvasTool } from './types.js'
import { MIN_SCALE, MAX_SCALE } from './types.js'

const PAN_THROTTLE_MS = 150

const SETTLE_MS = 200

function clampScale(s: number): number {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, s))
}

const GRID_TILE_PX = 24

const GRID_BKG_IMAGE = `url(${gridPattern})`

export function computeGridStyle(v: Viewport): {
  backgroundColor: string
  backgroundImage: string
  backgroundSize: string
  backgroundPosition: string
  opacity: string
} {
  const size = GRID_TILE_PX * v.scale
  return {
    backgroundColor: 'transparent',
    backgroundImage: GRID_BKG_IMAGE,
    backgroundSize: `${size}px ${size}px`,
    backgroundPosition: `${v.x % size}px ${v.y % size}px`,
    opacity: '0.15',
  }
}

export interface DragSelectHandler {
  onStart: (x: number, y: number) => void
  onMove: (x: number, y: number) => void
  onEnd: () => void
}

export interface UseCanvasViewportOptions {
  tool?: CanvasTool
  dragSelect?: DragSelectHandler
  containerRef?: RefObject<HTMLDivElement | null>
}

export interface UseCanvasViewportResult {
  viewport: Viewport
  setViewport: (v: Viewport) => void
  containerRef: RefObject<HTMLDivElement | null>
  gestureLayerRef: RefObject<HTMLDivElement | null>
  transformLayerRef: RefObject<HTMLDivElement | null>
  gridLayerRef: RefObject<HTMLDivElement | null>
}

function applyDomTransform(
  v: Viewport,
  transformLayer: HTMLDivElement | null,
  gridLayer: HTMLDivElement | null,
) {
  if (transformLayer) {
    transformLayer.style.transform = `translate3d(${v.x}px, ${v.y}px, 0) scale(${v.scale})`
  }
  if (gridLayer) {
    const gs = computeGridStyle(v)
    Object.assign(gridLayer.style, {
      backgroundSize: gs.backgroundSize,
      backgroundPosition: gs.backgroundPosition,
    })
  }
}

export function useCanvasViewport(
  options?: UseCanvasViewportOptions,
): UseCanvasViewportResult {
  const [viewport, setViewport] = useState<Viewport>({
    x: 0,
    y: 0,
    scale: 1,
  })

  const internalContainerRef = useRef<HTMLDivElement | null>(null)
  const containerRef = options?.containerRef ?? internalContainerRef
  const gestureLayerRef = useRef<HTMLDivElement | null>(null)
  const transformLayerRef = useRef<HTMLDivElement | null>(null)
  const gridLayerRef = useRef<HTMLDivElement | null>(null)
  const viewportRef = useRef(viewport)
  useEffect(() => {
    viewportRef.current = viewport
  }, [viewport])

  // Batch viewport updates. Scale changes flush via rAF (nodes need
  // the scale prop for semantic zoom). Pan-only changes apply the CSS
  // transform directly to the DOM and throttle React state updates so
  // culling refreshes at ~7fps instead of 60fps during fast panning.
  const pendingRef = useRef<Viewport | null>(null)
  const rafRef = useRef<number>(0)
  const panTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const settleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const flushToReact = useCallback((v: Viewport) => {
    pendingRef.current = null
    setViewport(v)
  }, [])

  const scheduleScaleFlush = useCallback(
    (v: Viewport) => {
      if (panTimerRef.current) {
        clearTimeout(panTimerRef.current)
        panTimerRef.current = null
      }
      pendingRef.current = v
      if (!rafRef.current) {
        rafRef.current = requestAnimationFrame(() => {
          rafRef.current = 0
          const pending = pendingRef.current
          if (pending) flushToReact(pending)
        })
      }
    },
    [flushToReact],
  )

  const schedulePanFlush = useCallback(
    (v: Viewport) => {
      pendingRef.current = v
      if (!panTimerRef.current) {
        panTimerRef.current = setTimeout(() => {
          panTimerRef.current = null
          const pending = pendingRef.current
          if (pending) flushToReact(pending)
        }, PAN_THROTTLE_MS)
      }
    },
    [flushToReact],
  )

  const scheduleViewportUpdate = useCallback(
    (v: Viewport) => {
      const scaleChanged = v.scale !== viewportRef.current.scale
      viewportRef.current = v

      const tl = transformLayerRef.current
      if (tl && !tl.style.willChange) {
        tl.style.willChange = 'transform'
      }

      applyDomTransform(v, tl, gridLayerRef.current)

      if (scaleChanged) {
        scheduleScaleFlush(v)
      } else {
        schedulePanFlush(v)
      }

      if (settleTimerRef.current) clearTimeout(settleTimerRef.current)
      settleTimerRef.current = setTimeout(() => {
        settleTimerRef.current = null
        if (tl) tl.style.willChange = ''
      }, SETTLE_MS)
    },
    [scheduleScaleFlush, schedulePanFlush],
  )

  useEffect(() => {
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current)
      if (panTimerRef.current) clearTimeout(panTimerRef.current)
      if (settleTimerRef.current) clearTimeout(settleTimerRef.current)
    }
  }, [])

  const zoomToward = useCallback(
    (clientX: number, clientY: number, newScale: number) => {
      const container = containerRef.current
      if (!container) return

      const rect = container.getBoundingClientRect()
      const px = clientX - rect.left
      const py = clientY - rect.top

      const prev = viewportRef.current
      const clamped = clampScale(newScale)

      const nx = px - ((px - prev.x) / prev.scale) * clamped
      const ny = py - ((py - prev.y) / prev.scale) * clamped

      scheduleViewportUpdate({ x: nx, y: ny, scale: clamped })
    },
    [containerRef, scheduleViewportUpdate],
  )

  const toolRef = useRef(options?.tool ?? 'select')
  useEffect(() => {
    toolRef.current = options?.tool ?? 'select'
  }, [options?.tool])
  const dragSelectRef = useRef(false)

  // Drag gesture on the gesture layer (sits below canvas nodes so node
  // pointer events are never intercepted by the viewport pan gesture).
  useGesture(
    {
      onDrag: ({ delta: [dx, dy], first, last, event, xy: [x, y] }) => {
        if (toolRef.current !== 'select') return

        const shiftKey =
          event instanceof MouseEvent || event instanceof PointerEvent
            ? event.shiftKey
            : false

        if (first && shiftKey && options?.dragSelect) {
          dragSelectRef.current = true
          const rect = containerRef.current?.getBoundingClientRect()
          if (rect) {
            options.dragSelect.onStart(x - rect.left, y - rect.top)
          }
          return
        }

        if (dragSelectRef.current && options?.dragSelect) {
          const rect = containerRef.current?.getBoundingClientRect()
          if (rect) {
            options.dragSelect.onMove(x - rect.left, y - rect.top)
          }
          if (last) {
            options.dragSelect.onEnd()
            dragSelectRef.current = false
          }
          return
        }

        if (last) {
          dragSelectRef.current = false
        }

        const prev = viewportRef.current
        scheduleViewportUpdate({
          ...prev,
          x: prev.x + dx,
          y: prev.y + dy,
        })
      },
    },
    {
      target: gestureLayerRef,
      drag: {
        filterTaps: true,
        pointer: { touch: true },
      },
    },
  )

  useGesture(
    {
      onWheel: ({ event, delta: [, dy] }) => {
        const target = event.target
        if (
          target instanceof Element &&
          target.closest('[data-interactive-content]')
        ) {
          return
        }
        event.preventDefault()
        const prev = viewportRef.current
        const factor = 1 - dy * 0.003
        zoomToward(event.clientX, event.clientY, prev.scale * factor)
      },
      onPinch: ({ origin: [ox, oy], offset: [scale] }) => {
        zoomToward(ox, oy, scale)
      },
    },
    {
      target: containerRef,
      wheel: {
        eventOptions: { passive: false },
      },
      pinch: {
        scaleBounds: { min: MIN_SCALE, max: MAX_SCALE },
      },
    },
  )

  return useMemo(
    () => ({
      viewport,
      setViewport,
      containerRef,
      gestureLayerRef,
      transformLayerRef,
      gridLayerRef,
    }),
    [viewport, setViewport, containerRef, gestureLayerRef],
  )
}
