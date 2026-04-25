import React from 'react'
import { describe, it, expect, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'

import { CanvasDrawingLayer } from './CanvasDrawingLayer.js'

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
})
