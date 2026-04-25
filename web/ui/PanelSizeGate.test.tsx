import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PanelSizeGate } from './PanelSizeGate.js'

class ResizeObserverMock {
  private callback: ResizeObserverCallback

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }

  observe(el: Element) {
    if (el instanceof HTMLElement) {
      const layoutWidth = Number(el.dataset.layoutWidth ?? 0)
      const screenWidth = Number(el.dataset.screenWidth ?? layoutWidth)
      Object.defineProperty(el, 'clientWidth', {
        value: layoutWidth,
        configurable: true,
      })
      Object.defineProperty(el, 'offsetWidth', {
        value: layoutWidth,
        configurable: true,
      })
      el.getBoundingClientRect = () => new DOMRect(0, 0, screenWidth, 100)
    }

    const rect = el.getBoundingClientRect()
    const entry: ResizeObserverEntry = {
      target: el,
      contentRect: new DOMRect(0, 0, rect.width, rect.height),
      borderBoxSize: [],
      contentBoxSize: [],
      devicePixelContentBoxSize: [],
    }
    this.callback([entry], this)
  }

  unobserve() {}

  disconnect() {}
}

describe('PanelSizeGate', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
    const baseGetComputedStyle = getComputedStyle
    vi.stubGlobal('getComputedStyle', (el: Element) => {
      const style = baseGetComputedStyle(el)
      if (el instanceof HTMLElement && el.dataset.transformScale) {
        Object.defineProperty(style, 'transform', {
          value: `scale(${el.dataset.transformScale})`,
          configurable: true,
        })
      }
      return style
    })
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders the fallback when the unscaled panel width is too small', async () => {
    render(
      <PanelSizeGate
        minWidth={400}
        fallback={<div>make bigger</div>}
        data-layout-width="320"
      >
        <div>content</div>
      </PanelSizeGate>,
    )

    await waitFor(() => {
      expect(screen.getByText('make bigger')).toBeDefined()
    })
  })

  it('keeps content visible when a scaled canvas ancestor shrinks the screen rect', async () => {
    render(
      <PanelSizeGate
        minWidth={400}
        fallback={<div>make bigger</div>}
        data-layout-width="500"
        data-screen-width="250"
        data-transform-scale="0.5"
      >
        <div>content</div>
      </PanelSizeGate>,
    )

    await waitFor(() => {
      expect(screen.getByText('content')).toBeDefined()
    })
    expect(screen.queryByText('make bigger')).toBeNull()
  })
})
