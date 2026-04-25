import { useRef, useState, useCallback, useEffect } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import type { Viewport, CanvasNodeData } from './types.js'

// Stroke represents a freeform drawing stroke.
interface Stroke {
  points: Array<{ x: number; y: number }>
}

// STROKE_PADDING is extra pixels around the bounding box of a stroke.
const STROKE_PADDING = 8

// CanvasDrawingLayerProps are the props for CanvasDrawingLayer.
interface CanvasDrawingLayerProps {
  visible: boolean
  viewport: Viewport
  onStrokeComplete?: (node: CanvasNodeData) => void
  className?: string
}

// screenToCanvas converts screen-relative coordinates to canvas space.
function screenToCanvas(
  sx: number,
  sy: number,
  vp: Viewport,
): { x: number; y: number } {
  return {
    x: (sx - vp.x) / vp.scale,
    y: (sy - vp.y) / vp.scale,
  }
}

// CanvasDrawingLayer renders a canvas element for freeform drawing.
export function CanvasDrawingLayer({
  visible,
  viewport,
  onStrokeComplete,
  className,
}: CanvasDrawingLayerProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const currentStroke = useRef<Stroke | null>(null)
  const drawing = useRef(false)

  const redraw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const vp = viewportRef.current
    ctx.clearRect(0, 0, canvas.width, canvas.height)

    const stroke = currentStroke.current
    if (!stroke?.points || stroke.points.length < 2) return

    ctx.strokeStyle = 'currentColor'
    ctx.lineWidth = 2
    ctx.lineCap = 'round'
    ctx.lineJoin = 'round'
    ctx.beginPath()
    const p0 = stroke.points[0]
    ctx.moveTo(p0.x * vp.scale + vp.x, p0.y * vp.scale + vp.y)
    for (let i = 1; i < stroke.points.length; i++) {
      const p = stroke.points[i]
      ctx.lineTo(p.x * vp.scale + vp.x, p.y * vp.scale + vp.y)
    }
    ctx.stroke()
  }, [])

  // Resize canvas to match its display size.
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

  const viewportRef = useRef(viewport)
  viewportRef.current = viewport

  const handlePointerDown = useCallback(
    (e: React.PointerEvent) => {
      if (!visible) return
      e.stopPropagation()
      drawing.current = true
      const rect = canvasRef.current?.getBoundingClientRect()
      if (!rect) return
      const pt = screenToCanvas(
        e.clientX - rect.left,
        e.clientY - rect.top,
        viewportRef.current,
      )
      currentStroke.current = { points: [pt] }
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
    },
    [visible],
  )

  const handlePointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!drawing.current || !currentStroke.current) return
      const rect = canvasRef.current?.getBoundingClientRect()
      if (!rect) return
      const pt = screenToCanvas(
        e.clientX - rect.left,
        e.clientY - rect.top,
        viewportRef.current,
      )
      currentStroke.current.points.push(pt)
      redraw()
    },
    [redraw],
  )

  const onStrokeCompleteRef = useRef(onStrokeComplete)
  onStrokeCompleteRef.current = onStrokeComplete

  const handlePointerUp = useCallback(() => {
    if (!drawing.current || !currentStroke.current) return
    drawing.current = false
    const stroke = currentStroke.current
    currentStroke.current = null
    if (stroke.points.length < 2) return

    // Compute bounding box in canvas space.
    let minX = Infinity,
      minY = Infinity,
      maxX = -Infinity,
      maxY = -Infinity
    for (const p of stroke.points) {
      if (p.x < minX) minX = p.x
      if (p.y < minY) minY = p.y
      if (p.x > maxX) maxX = p.x
      if (p.y > maxY) maxY = p.y
    }
    const x = minX - STROKE_PADDING
    const y = minY - STROKE_PADDING
    const w = maxX - minX + STROKE_PADDING * 2
    const h = maxY - minY + STROKE_PADDING * 2

    // Normalize points relative to the node origin.
    const relPoints = stroke.points.map((p) => ({
      x: p.x - x,
      y: p.y - y,
    }))

    const id = `draw-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    const shapeData = new TextEncoder().encode(JSON.stringify(relPoints))

    const node: CanvasNodeData = {
      id,
      x,
      y,
      width: Math.max(w, 20),
      height: Math.max(h, 20),
      zIndex: 0,
      type: 'drawing',
      shapeData,
    }
    onStrokeCompleteRef.current?.(node)
    redraw()
  }, [redraw])

  return (
    <canvas
      ref={canvasRef}
      className={cn(
        'text-foreground absolute inset-0 h-full w-full',
        visible ?
          'pointer-events-auto cursor-crosshair'
        : 'pointer-events-none',
        className,
      )}
      style={{ zIndex: visible ? 10 : -1 }}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
    />
  )
}
