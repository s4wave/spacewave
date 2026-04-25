import React from 'react'
import { describe, it, expect, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'

import { CanvasSelectionOverlay } from './CanvasSelectionOverlay.js'

describe('CanvasSelectionOverlay', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders nothing when dragRect is null', () => {
    const { container } = render(<CanvasSelectionOverlay dragRect={null} />)
    expect(container.firstChild).toBeFalsy()
  })

  it('renders a rectangle when dragRect is provided', () => {
    const rect = { startX: 10, startY: 20, endX: 110, endY: 120 }
    render(<CanvasSelectionOverlay dragRect={rect} />)
    const overlay = document.querySelector(
      '.pointer-events-none.border-brand\\/30',
    )
    expect(overlay).toBeTruthy()
  })

  it('computes correct position and dimensions from dragRect', () => {
    const rect = { startX: 50, startY: 30, endX: 200, endY: 180 }
    render(<CanvasSelectionOverlay dragRect={rect} />)
    const overlay = document.querySelector(
      '.pointer-events-none',
    ) as HTMLElement
    expect(overlay.style.left).toBe('50px')
    expect(overlay.style.top).toBe('30px')
    expect(overlay.style.width).toBe('150px')
    expect(overlay.style.height).toBe('150px')
  })

  it('handles inverted (dragged backwards) coordinates', () => {
    // End coordinates smaller than start (drag up-left).
    const rect = { startX: 200, startY: 180, endX: 50, endY: 30 }
    render(<CanvasSelectionOverlay dragRect={rect} />)
    const overlay = document.querySelector(
      '.pointer-events-none',
    ) as HTMLElement
    // Math.min picks the smaller coordinate for left/top.
    expect(overlay.style.left).toBe('50px')
    expect(overlay.style.top).toBe('30px')
    expect(overlay.style.width).toBe('150px')
    expect(overlay.style.height).toBe('150px')
  })

  it('handles zero-size selection rectangle', () => {
    const rect = { startX: 100, startY: 100, endX: 100, endY: 100 }
    render(<CanvasSelectionOverlay dragRect={rect} />)
    const overlay = document.querySelector(
      '.pointer-events-none',
    ) as HTMLElement
    expect(overlay.style.width).toBe('0px')
    expect(overlay.style.height).toBe('0px')
  })
})
