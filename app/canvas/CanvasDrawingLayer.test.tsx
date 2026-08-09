import React from 'react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, cleanup, fireEvent } from '@testing-library/react'

import { CanvasDrawingLayer } from './CanvasDrawingLayer.js'
import { CanvasGeometryNode } from './CanvasGeometryNode.js'
import type { CanvasNodeData } from './types.js'

const defaultViewport = { x: 0, y: 0, scale: 1 }

describe('CanvasDrawingLayer', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders a canvas element', () => {
    render(<CanvasDrawingLayer visible={false} viewport={defaultViewport} />)
    const canvas = document.querySelector('canvas')
    expect(canvas).toBeTruthy()
  })

  it('has pointer-events-auto and cursor-crosshair when visible', () => {
    render(<CanvasDrawingLayer visible={true} viewport={defaultViewport} />)
    const canvas = document.querySelector('canvas') as HTMLElement
    expect(canvas.className).toContain('pointer-events-auto')
    expect(canvas.className).toContain('cursor-crosshair')
  })

  it('has pointer-events-none when not visible', () => {
    render(<CanvasDrawingLayer visible={false} viewport={defaultViewport} />)
    const canvas = document.querySelector('canvas') as HTMLElement
    expect(canvas.className).toContain('pointer-events-none')
    expect(canvas.className).not.toContain('cursor-crosshair')
  })

  it('sets higher z-index when visible', () => {
    render(<CanvasDrawingLayer visible={true} viewport={defaultViewport} />)
    const canvas = document.querySelector('canvas') as HTMLElement
    expect(canvas.style.zIndex).toBe('10')
  })

  it('sets lower z-index when not visible', () => {
    render(<CanvasDrawingLayer visible={false} viewport={defaultViewport} />)
    const canvas = document.querySelector('canvas') as HTMLElement
    expect(canvas.style.zIndex).toBe('-1')
  })

  it('uses the selected color for the live and committed stroke', () => {
    const context = {
      beginPath: vi.fn(),
      clearRect: vi.fn(),
      lineTo: vi.fn(),
      moveTo: vi.fn(),
      stroke: vi.fn(),
      strokeStyle: '',
      fillStyle: '',
      lineWidth: 0,
      lineCap: '',
      lineJoin: '',
    }
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      context as unknown as CanvasRenderingContext2D,
    )
    const onStrokeComplete = vi.fn<(node: CanvasNodeData) => void>()
    const { rerender } = render(
      <CanvasDrawingLayer
        visible
        viewport={defaultViewport}
        color="#dc2626"
        onStrokeComplete={onStrokeComplete}
      />,
    )
    const canvas = document.querySelector('canvas') as HTMLCanvasElement
    canvas.setPointerCapture = vi.fn()
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100,
      toJSON: () => ({}),
    })

    fireEvent.pointerDown(canvas, { clientX: 10, clientY: 10, pointerId: 1 })
    fireEvent.pointerMove(canvas, { clientX: 40, clientY: 40, pointerId: 1 })
    fireEvent.pointerUp(canvas, { clientX: 40, clientY: 40, pointerId: 1 })

    expect(context.strokeStyle).toBe('#dc2626')
    const node = onStrokeComplete.mock.calls[0]?.[0]
    if (!node) throw new Error('drawing did not commit a canvas node')
    expect(node.geometry?.color).toBe('#dc2626')

    rerender(<CanvasGeometryNode node={node} />)
    expect(document.querySelector('path')?.getAttribute('stroke')).toBe(
      '#dc2626',
    )
  })
})
