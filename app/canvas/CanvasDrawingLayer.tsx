import { useCallback, useEffect, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import type { CanvasNodeData, Viewport } from './types.js'
import {
  DEFAULT_CANVAS_COLOR,
  type CanvasGeometryKind,
  type CanvasPoint,
} from './geometry.js'

interface Stroke {
  points: CanvasPoint[]
}

const STROKE_PADDING = 8

interface CanvasDrawingLayerProps {
  visible: boolean
  viewport: Viewport
  kind?: CanvasGeometryKind
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

// CanvasDrawingLayer captures and previews pen strokes and primitive shapes.
export function CanvasDrawingLayer({
  visible,
  viewport,
  kind = 'pen',
  color = DEFAULT_CANVAS_COLOR,
  onStrokeComplete,
  className,
}: CanvasDrawingLayerProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const currentStroke = useRef<Stroke | null>(null)
  const drawing = useRef(false)
  const viewportRef = useRef(viewport)
  const kindRef = useRef(kind)
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

    const screenPoints = stroke.points.map((point) => ({
      x: point.x * vp.scale + vp.x,
      y: point.y * vp.scale + vp.y,
    }))
    const start = screenPoints[0]
    const end = screenPoints[screenPoints.length - 1]

    ctx.strokeStyle = colorRef.current
    ctx.fillStyle = colorRef.current
    ctx.lineWidth = 2
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    ctx.beginPath()

    switch (kindRef.current) {
      case 'pen':
        ctx.moveTo(start.x, start.y)
        for (const point of screenPoints.slice(1)) ctx.lineTo(point.x, point.y)
        ctx.stroke()
        break
      case 'line':
        ctx.moveTo(start.x, start.y)
        ctx.lineTo(end.x, end.y)
        ctx.stroke()
        break
      case 'arrow': {
        ctx.moveTo(start.x, start.y)
        ctx.lineTo(end.x, end.y)
        ctx.stroke()
        const angle = Math.atan2(end.y - start.y, end.x - start.x)
        ctx.beginPath()
        ctx.moveTo(end.x, end.y)
        ctx.lineTo(
          end.x - 10 * Math.cos(angle - Math.PI / 6),
          end.y - 10 * Math.sin(angle - Math.PI / 6),
        )
        ctx.lineTo(
          end.x - 10 * Math.cos(angle + Math.PI / 6),
          end.y - 10 * Math.sin(angle + Math.PI / 6),
        )
        ctx.closePath()
        ctx.fill()
        break
      }
      case 'rectangle':
        ctx.strokeRect(
          Math.min(start.x, end.x),
          Math.min(start.y, end.y),
          Math.abs(end.x - start.x),
          Math.abs(end.y - start.y),
        )
        break
      case 'ellipse':
        ctx.ellipse(
          (start.x + end.x) / 2,
          (start.y + end.y) / 2,
          Math.abs(end.x - start.x) / 2,
          Math.abs(end.y - start.y) / 2,
          0,
          0,
          Math.PI * 2,
        )
        ctx.stroke()
        break
    }
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
    kindRef.current = kind
    colorRef.current = color
    redraw()
  }, [color, kind, redraw])

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
      const point = screenToCanvas(
        event.clientX - rect.left,
        event.clientY - rect.top,
        viewportRef.current,
      )
      if (kindRef.current === 'pen') currentStroke.current.points.push(point)
      else currentStroke.current.points[1] = point
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
    if (maxX - minX < 2 && maxY - minY < 2) return

    const x = minX - STROKE_PADDING
    const y = minY - STROKE_PADDING
    const width = Math.max(maxX - minX + STROKE_PADDING * 2, 20)
    const height = Math.max(maxY - minY + STROKE_PADDING * 2, 20)
    const geometry = {
      kind: kindRef.current,
      color: colorRef.current,
      points: stroke.points.map((point) => ({
        x: point.x - x,
        y: point.y - y,
      })),
    }
    const id = `draw-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    onStrokeCompleteRef.current?.({
      id,
      x,
      y,
      width,
      height,
      zIndex: 0,
      type: geometry.kind === 'pen' ? 'drawing' : 'shape',
      geometry,
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
