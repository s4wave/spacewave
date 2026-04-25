/**
 * E2E tests for FloatingWindow component z-index management.
 *
 * These tests verify that clicking on a floating window brings it to the front.
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'
import { useState, useCallback } from 'react'

import '@s4wave/web/style/app.css'

import {
  FloatingWindow,
  FloatingWindowManagerProvider,
  type FloatingWindowState,
} from './FloatingWindow.js'

const DEFAULT_STATE: FloatingWindowState = {
  position: { x: 50, y: 50 },
  size: { width: 200, height: 150 },
  expanded: true,
}

// Position window 2 to the right so they don't overlap
const SECOND_WINDOW_STATE: FloatingWindowState = {
  position: { x: 300, y: 50 },
  size: { width: 200, height: 150 },
  expanded: true,
}

function TestWindow({
  id,
  title,
  initialState,
}: {
  id: string
  title: string
  initialState: FloatingWindowState
}) {
  const [state, setState] = useState(initialState)

  const handleStateChange = useCallback((newState: FloatingWindowState) => {
    setState(newState)
  }, [])

  if (!state.expanded) return null

  return (
    <FloatingWindow
      id={id}
      title={title}
      state={state}
      onStateChange={handleStateChange}
      testId={`window-${id}`}
    >
      <div data-testid={`window-content-${id}`} style={{ padding: '16px' }}>
        Content for {title}
      </div>
    </FloatingWindow>
  )
}

function TwoWindowsTest() {
  return (
    <FloatingWindowManagerProvider>
      <div style={{ width: '800px', height: '600px', position: 'relative' }}>
        <TestWindow
          id="window-1"
          title="Window 1"
          initialState={DEFAULT_STATE}
        />
        <TestWindow
          id="window-2"
          title="Window 2"
          initialState={SECOND_WINDOW_STATE}
        />
      </div>
    </FloatingWindowManagerProvider>
  )
}

describe('FloatingWindow z-index management', () => {
  beforeEach(async () => {
    await cleanup()
  })

  it('renders two overlapping windows', async () => {
    await render(<TwoWindowsTest />)

    await expect
      .element(page.getByTestId('window-window-1'))
      .toBeInTheDocument()
    await expect
      .element(page.getByTestId('window-window-2'))
      .toBeInTheDocument()
  })

  it('brings window to front when clicked on content area', async () => {
    await render(<TwoWindowsTest />)

    // Wait for both windows to render
    await expect
      .element(page.getByTestId('window-window-1'))
      .toBeInTheDocument()
    await expect
      .element(page.getByTestId('window-window-2'))
      .toBeInTheDocument()

    // Get the window elements
    const window1 = page.getByTestId('window-window-1')
    const window2 = page.getByTestId('window-window-2')

    // Window 2 was rendered second, so it should be on top initially
    const getZIndex = (el: Element | null) => {
      if (!el) return 0
      return parseInt(window.getComputedStyle(el).zIndex || '0', 10)
    }

    // Verify initial state - window 2 should be on top
    await expect
      .poll(() => {
        const z1 = getZIndex(window1.element())
        const z2 = getZIndex(window2.element())
        return z2 > z1
      })
      .toBe(true)

    // Click on window 1's content area
    const window1Content = page.getByTestId('window-content-window-1')
    await userEvent.click(window1Content.element())

    // Now window 1 should be on top
    await expect
      .poll(
        () => {
          const z1 = getZIndex(window1.element())
          const z2 = getZIndex(window2.element())
          return z1 > z2
        },
        { timeout: 2000 },
      )
      .toBe(true)
  })

  it('brings window to front when clicked on header', async () => {
    await render(<TwoWindowsTest />)

    // Wait for both windows to render
    await expect
      .element(page.getByTestId('window-window-1'))
      .toBeInTheDocument()
    await expect
      .element(page.getByTestId('window-window-2'))
      .toBeInTheDocument()

    const window1 = page.getByTestId('window-window-1')
    const window2 = page.getByTestId('window-window-2')

    const getZIndex = (el: Element | null) => {
      if (!el) return 0
      return parseInt(window.getComputedStyle(el).zIndex || '0', 10)
    }

    // Window 2 should be on top initially
    await expect
      .poll(() => {
        const z1 = getZIndex(window1.element())
        const z2 = getZIndex(window2.element())
        return z2 > z1
      })
      .toBe(true)

    // Click on window 1's header (the title text) - use exact: true to avoid matching content
    const window1Title = page.getByText('Window 1', { exact: true })
    await userEvent.click(window1Title.element())

    // Now window 1 should be on top
    await expect
      .poll(
        () => {
          const z1 = getZIndex(window1.element())
          const z2 = getZIndex(window2.element())
          return z1 > z2
        },
        { timeout: 2000 },
      )
      .toBe(true)
  })

  it('maintains z-index order through multiple clicks', async () => {
    await render(<TwoWindowsTest />)

    await expect
      .element(page.getByTestId('window-window-1'))
      .toBeInTheDocument()
    await expect
      .element(page.getByTestId('window-window-2'))
      .toBeInTheDocument()

    const window1 = page.getByTestId('window-window-1')
    const window2 = page.getByTestId('window-window-2')

    const getZIndex = (el: Element | null) => {
      if (!el) return 0
      return parseInt(window.getComputedStyle(el).zIndex || '0', 10)
    }

    // Click window 1 to bring it to front
    await userEvent.click(page.getByTestId('window-content-window-1').element())
    await expect
      .poll(() => getZIndex(window1.element()) > getZIndex(window2.element()), {
        timeout: 2000,
      })
      .toBe(true)

    // Click window 2 to bring it to front
    await userEvent.click(page.getByTestId('window-content-window-2').element())
    await expect
      .poll(() => getZIndex(window2.element()) > getZIndex(window1.element()), {
        timeout: 2000,
      })
      .toBe(true)

    // Click window 1 again
    await userEvent.click(page.getByTestId('window-content-window-1').element())
    await expect
      .poll(() => getZIndex(window1.element()) > getZIndex(window2.element()), {
        timeout: 2000,
      })
      .toBe(true)
  })
})
