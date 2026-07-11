import { useCallback, useEffect, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import type { CanvasNodeData, Viewport } from './types.js'
import {
  DEFAULT_CANVAS_COLOR,
  encodeCanvasGeometry,
  type CanvasPoint,
} from './geometry.js'

interface Stroke {
  points: CanvasPoint[]
}

const STROKE_PADDING = 8

interface CanvasDrawingLayerProps {
  visible: boolean
  viewport: Viewport
  color?: string
  onStrokeComplete?: (node: CanvasNodeData) => void
  className?: string
}

function screenToCanvas(sx: number, sy: number, vp: Viewport): CanvasPoint {
  return {
    x: (sx - vp.x) / vp.scale,
    y: (sy - vp.y) / vp.scale,
  }
}

// CanvasDrawingLayer captures and previews freeform pen strokes.
export function CanvasDrawingLayer({
  visible,
  viewport,
  color = DEFAULT_CANVAS_COLOR,
  onStrokeComplete,
  className,
}: CanvasDrawingLayerProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const currentStroke = useRef<Stroke | null>(null)
  const drawing = useRef(false)
  const viewportRef = useRef(viewport)
  const colorRef = useRef(color)
  const onStrokeCompleteRef = useRef(onStrokeComplete)

  const redraw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const vp = viewportRef.current
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    const stroke = currentStroke.current
    if (!stroke || stroke.points.length < 2) return

    ctx.strokeStyle = colorRef.current
    ctx.lineWidth = 2
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    ctx.beginPath()
    const start = stroke.points[0]
    ctx.moveTo(start.x * vp.scale + vp.x, start.y * vp.scale + vp.y)
    for (const point of stroke.points.slice(1)) {
      ctx.lineTo(point.x * vp.scale + vp.x, point.y * vp.scale + vp.y)
    }
    ctx.stroke()
  }, [])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect
        canvas.width = width
        canvas.height = height
        redraw()
      }
    })
    observer.observe(canvas)
    return () => observer.disconnect()
  }, [redraw])

  useEffect(() => {
    viewportRef.current = viewport
    redraw()
  }, [redraw, viewport])

  useEffect(() => {
    colorRef.current = color
    redraw()
  }, [color, redraw])

  useEffect(() => {
    onStrokeCompleteRef.current = onStrokeComplete
  }, [onStrokeComplete])

  const handlePointerDown = useCallback(
    (event: React.PointerEvent) => {
      if (!visible) return
      event.stopPropagation()
      const rect = canvasRef.current?.getBoundingClientRect()
      if (!rect) return
      drawing.current = true
      currentStroke.current = {
        points: [
          screenToCanvas(
            event.clientX - rect.left,
            event.clientY - rect.top,
            viewportRef.current,
          ),
        ],
      }
      ;(event.target as HTMLElement).setPointerCapture(event.pointerId)
    },
    [visible],
  )

  const handlePointerMove = useCallback(
    (event: React.PointerEvent) => {
      if (!drawing.current || !currentStroke.current) return
      const rect = canvasRef.current?.getBoundingClientRect()
      if (!rect) return
      currentStroke.current.points.push(
        screenToCanvas(
          event.clientX - rect.left,
          event.clientY - rect.top,
          viewportRef.current,
        ),
      )
      redraw()
    },
    [redraw],
  )

  const handlePointerUp = useCallback(() => {
    if (!drawing.current || !currentStroke.current) return
    drawing.current = false
    const stroke = currentStroke.current
    currentStroke.current = null
    if (stroke.points.length < 2) return

    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity
    for (const point of stroke.points) {
      minX = Math.min(minX, point.x)
      minY = Math.min(minY, point.y)
      maxX = Math.max(maxX, point.x)
      maxY = Math.max(maxY, point.y)
    }
    const x = minX - STROKE_PADDING
    const y = minY - STROKE_PADDING
    const id = `draw-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    onStrokeCompleteRef.current?.({
      id,
      x,
      y,
      width: Math.max(maxX - minX + STROKE_PADDING * 2, 20),
      height: Math.max(maxY - minY + STROKE_PADDING * 2, 20),
      zIndex: 0,
      type: 'drawing',
      shapeData: encodeCanvasGeometry({
        kind: 'pen',
        color: colorRef.current,
        points: stroke.points.map((point) => ({
          x: point.x - x,
          y: point.y - y,
        })),
      }),
    })
    redraw()
  }, [redraw])

  return (
    <canvas
      ref={canvasRef}
      className={cn(
        'absolute inset-0 h-full w-full',
        visible
          ? 'pointer-events-auto cursor-crosshair'
          : 'pointer-events-none',
        className,
      )}
      style={{ zIndex: visible ? 10 : -1 }}
      data-canvas-drawing-color={color}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
    />
  )
}
