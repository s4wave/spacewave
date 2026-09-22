import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import {
  FloatingWindow,
  FloatingWindowManagerProvider,
  type FloatingWindowState,
} from './FloatingWindow.js'

// Helper to create a default expanded state for tests.
function makeState(
  overrides?: Partial<FloatingWindowState>,
): FloatingWindowState {
  return {
    position: { x: 100, y: 100 },
    size: { width: 400, height: 300 },
    expanded: true,
    ...overrides,
  }
}

function getStyleNumber(element: HTMLElement, name: string): number {
  return Number.parseInt(element.style.getPropertyValue(name), 10)
}

// -- FloatingWindowManagerProvider tests --

describe('FloatingWindowManagerProvider', () => {
  afterEach(() => {
    cleanup()
  })

  it('multiple windows get sequential z-indices', () => {
    const state = makeState()
    const onStateChange = vi.fn()

    render(
      <FloatingWindowManagerProvider>
        <FloatingWindow
          id="win-a"
          title="Window A"
          state={state}
          onStateChange={onStateChange}
          testId="win-a"
        >
          <div>A content</div>
        </FloatingWindow>
        <FloatingWindow
          id="win-b"
          title="Window B"
          state={state}
          onStateChange={onStateChange}
          testId="win-b"
        >
          <div>B content</div>
        </FloatingWindow>
      </FloatingWindowManagerProvider>,
    )

    const winA = screen.getByTestId('win-a')
    const winB = screen.getByTestId('win-b')

    const zIndexA = getStyleNumber(winA, '--floating-window-z-index')
    const zIndexB = getStyleNumber(winB, '--floating-window-z-index')

    // Both should have valid z-index values
    expect(zIndexA).toBeGreaterThanOrEqual(1000)
    expect(zIndexB).toBeGreaterThanOrEqual(1000)
    // Window B registered second, so it should have a higher z-index
    expect(zIndexB).toBe(zIndexA + 1)
  })

  it('bringToFront moves window to top z-index', () => {
    const state = makeState()
    const onStateChange = vi.fn()

    render(
      <FloatingWindowManagerProvider>
        <FloatingWindow
          id="win-a"
          title="Window A"
          state={state}
          onStateChange={onStateChange}
          testId="win-a"
        >
          <div>A content</div>
        </FloatingWindow>
        <FloatingWindow
          id="win-b"
          title="Window B"
          state={state}
          onStateChange={onStateChange}
          testId="win-b"
        >
          <div>B content</div>
        </FloatingWindow>
      </FloatingWindowManagerProvider>,
    )

    const winA = screen.getByTestId('win-a')
    const winB = screen.getByTestId('win-b')

    // Initially, win-b is on top
    expect(getStyleNumber(winB, '--floating-window-z-index')).toBeGreaterThan(
      getStyleNumber(winA, '--floating-window-z-index'),
    )

    // Click on win-a to bring it to front (mousedown triggers bringToFront)
    fireEvent.mouseDown(winA)

    // Now win-a should have a higher z-index than win-b
    expect(getStyleNumber(winA, '--floating-window-z-index')).toBeGreaterThan(
      getStyleNumber(winB, '--floating-window-z-index'),
    )
  })

  it('unregister removes window from z-index ordering', () => {
    const stateA = makeState()
    const stateB = makeState()
    const onStateChangeA = vi.fn()
    const onStateChangeB = vi.fn()

    // Render two windows, then remove one and verify the other still works.
    const { rerender } = render(
      <FloatingWindowManagerProvider>
        <FloatingWindow
          id="win-a"
          title="Window A"
          state={stateA}
          onStateChange={onStateChangeA}
          testId="win-a"
        >
          <div>A content</div>
        </FloatingWindow>
        <FloatingWindow
          id="win-b"
          title="Window B"
          state={stateB}
          onStateChange={onStateChangeB}
          testId="win-b"
        >
          <div>B content</div>
        </FloatingWindow>
      </FloatingWindowManagerProvider>,
    )

    // Both windows are present
    expect(screen.getByTestId('win-a')).toBeTruthy()
    expect(screen.getByTestId('win-b')).toBeTruthy()

    // Remove win-a by re-rendering without it
    rerender(
      <FloatingWindowManagerProvider>
        <FloatingWindow
          id="win-b"
          title="Window B"
          state={stateB}
          onStateChange={onStateChangeB}
          testId="win-b"
        >
          <div>B content</div>
        </FloatingWindow>
      </FloatingWindowManagerProvider>,
    )

    // win-a should be gone, win-b should still have a valid z-index
    expect(screen.queryByTestId('win-a')).toBeNull()
    const winB = screen.getByTestId('win-b')
    expect(
      getStyleNumber(winB, '--floating-window-z-index'),
    ).toBeGreaterThanOrEqual(1000)
  })
})

// -- FloatingWindow basic rendering tests --

describe('FloatingWindow', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders title text', () => {
    const state = makeState()
    render(
      <FloatingWindow
        id="test"
        title="My Window"
        state={state}
        onStateChange={vi.fn()}
      >
        <div>content</div>
      </FloatingWindow>,
    )

    expect(screen.getByText('My Window')).toBeTruthy()
  })

  it('renders icon when provided', () => {
    const state = makeState()
    render(
      <FloatingWindow
        id="test"
        title="With Icon"
        icon={<span data-testid="test-icon">IC</span>}
        state={state}
        onStateChange={vi.fn()}
      >
        <div>content</div>
      </FloatingWindow>,
    )

    expect(screen.getByTestId('test-icon')).toBeTruthy()
  })

  it('renders children content', () => {
    const state = makeState()
    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={vi.fn()}
      >
        <div data-testid="child-content">Hello from child</div>
      </FloatingWindow>,
    )

    expect(screen.getByTestId('child-content')).toBeTruthy()
    expect(screen.getByText('Hello from child')).toBeTruthy()
  })

  it('applies testId as data-testid', () => {
    const state = makeState()
    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={vi.fn()}
        testId="my-floating-window"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    expect(screen.getByTestId('my-floating-window')).toBeTruthy()
  })

  it('applies custom className', () => {
    const state = makeState()
    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={vi.fn()}
        className="mt-1"
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    const el = screen.getByTestId('win')
    expect(el.className).toContain('mt-1')
  })

  it('minimize button calls onStateChange with expanded: false', () => {
    const state = makeState({ expanded: true })
    const onStateChange = vi.fn()

    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={onStateChange}
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    // The minimize button is the first button in the header
    const buttons = screen.getByTestId('win').querySelectorAll('button')
    expect(buttons.length).toBeGreaterThanOrEqual(2)

    fireEvent.click(buttons[0]) // minimize button

    expect(onStateChange).toHaveBeenCalledOnce()
    expect(onStateChange).toHaveBeenCalledWith({
      ...state,
      expanded: false,
    })
  })

  it('close button calls onClose when provided', () => {
    const state = makeState({ expanded: true })
    const onStateChange = vi.fn()
    const onClose = vi.fn()

    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={onStateChange}
        onClose={onClose}
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    const buttons = screen.getByTestId('win').querySelectorAll('button')
    fireEvent.click(buttons[1]) // close button

    expect(onClose).toHaveBeenCalledOnce()
    expect(onStateChange).not.toHaveBeenCalled()
  })

  it('close button calls onStateChange (minimize) when onClose is not provided', () => {
    const state = makeState({ expanded: true })
    const onStateChange = vi.fn()

    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={onStateChange}
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    const buttons = screen.getByTestId('win').querySelectorAll('button')
    fireEvent.click(buttons[1]) // close button (no onClose provided, falls back to minimize)

    expect(onStateChange).toHaveBeenCalledOnce()
    expect(onStateChange).toHaveBeenCalledWith({
      ...state,
      expanded: false,
    })
  })

  it('renders 8 resize handles', () => {
    const state = makeState()
    render(
      <FloatingWindow
        id="test"
        title="Win"
        state={state}
        onStateChange={vi.fn()}
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    const win = screen.getByTestId('win')
    // Resize handles have cursor-*-resize classes. Each handle div is a direct
    // child of the window container with an absolute position and resize cursor.
    const resizeHandles = win.querySelectorAll(
      '[class*="cursor-"][class*="-resize"]',
    )
    expect(resizeHandles.length).toBe(8)
  })

  it('double-click on header resets position and size to defaults', () => {
    const state = makeState({
      position: { x: 500, y: 500 },
      size: { width: 800, height: 600 },
    })
    const onStateChange = vi.fn()
    const defaultPosition = { x: 10, y: 10 }
    const defaultSize = { width: 320, height: 240 }

    render(
      <FloatingWindow
        id="test"
        title="Resettable"
        state={state}
        onStateChange={onStateChange}
        defaultPosition={defaultPosition}
        defaultSize={defaultSize}
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    // The header is the drag target - find it by the title text's grandparent
    const titleSpan = screen.getByText('Resettable')
    // The header div is the parent of the flex container that holds the title
    const header = titleSpan.closest('[class*="cursor-grab"]')
    expect(header).toBeTruthy()

    fireEvent.doubleClick(header!)

    expect(onStateChange).toHaveBeenCalledOnce()
    expect(onStateChange).toHaveBeenCalledWith({
      ...state,
      position: defaultPosition,
      size: defaultSize,
    })
  })

  it('applies position and size from state as inline styles', () => {
    const state = makeState({
      position: { x: 42, y: 84 },
      size: { width: 500, height: 350 },
    })

    render(
      <FloatingWindow
        id="test"
        title="Styled"
        state={state}
        onStateChange={vi.fn()}
        testId="win"
      >
        <div>content</div>
      </FloatingWindow>,
    )

    const win = screen.getByTestId('win')
    expect(win.style.getPropertyValue('--floating-window-left')).toBe('42px')
    expect(win.style.getPropertyValue('--floating-window-top')).toBe('84px')
    expect(win.style.getPropertyValue('--floating-window-width')).toBe('500px')
    expect(win.style.getPropertyValue('--floating-window-height')).toBe('350px')
  })
})

// -- Integration tests: FloatingWindowManagerProvider + FloatingWindow --

describe('FloatingWindowManagerProvider + FloatingWindow integration', () => {
  afterEach(() => {
    cleanup()
  })

  it('windows render with z-index from manager', () => {
    const state = makeState()
    const onStateChange = vi.fn()

    render(
      <FloatingWindowManagerProvider>
        <FloatingWindow
          id="w1"
          title="Win 1"
          state={state}
          onStateChange={onStateChange}
          testId="w1"
        >
          <div>1</div>
        </FloatingWindow>
        <FloatingWindow
          id="w2"
          title="Win 2"
          state={state}
          onStateChange={onStateChange}
          testId="w2"
        >
          <div>2</div>
        </FloatingWindow>
      </FloatingWindowManagerProvider>,
    )

    const w1 = screen.getByTestId('w1')
    const w2 = screen.getByTestId('w2')

    // Both should have z-index set from the manager (base 1000)
    const z1 = getStyleNumber(w1, '--floating-window-z-index')
    const z2 = getStyleNumber(w2, '--floating-window-z-index')

    expect(z1).toBe(1000)
    expect(z2).toBe(1001)
  })

  it('clicking a window brings it to front', () => {
    const state = makeState()
    const onStateChange = vi.fn()

    render(
      <FloatingWindowManagerProvider>
        <FloatingWindow
          id="w1"
          title="Win 1"
          state={state}
          onStateChange={onStateChange}
          testId="w1"
        >
          <div data-testid="w1-content">1</div>
        </FloatingWindow>
        <FloatingWindow
          id="w2"
          title="Win 2"
          state={state}
          onStateChange={onStateChange}
          testId="w2"
        >
          <div data-testid="w2-content">2</div>
        </FloatingWindow>
      </FloatingWindowManagerProvider>,
    )

    const w1 = screen.getByTestId('w1')
    const w2 = screen.getByTestId('w2')

    // Initially w2 is on top
    expect(getStyleNumber(w2, '--floating-window-z-index')).toBeGreaterThan(
      getStyleNumber(w1, '--floating-window-z-index'),
    )

    // Click on w1 to bring it to front
    fireEvent.mouseDown(w1)

    // Now w1 should have the higher z-index
    expect(getStyleNumber(w1, '--floating-window-z-index')).toBeGreaterThan(
      getStyleNumber(w2, '--floating-window-z-index'),
    )
  })
})
