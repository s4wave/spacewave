import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  type ViewerMedia,
  ViewerFloat,
  ViewerFloatPress,
  ViewerStage,
} from './ViewerFloat.js'

function renderStage(media: ViewerMedia) {
  return render(
    <ViewerStage media={media} idleMs={1000}>
      <div data-testid="content" />
      <ViewerFloat at="bc" label="Pages">
        <ViewerFloatPress label="Next page" essential />
      </ViewerFloat>
    </ViewerStage>,
  )
}

function floatState() {
  return screen.getByRole('toolbar', { name: 'Pages' }).dataset.state
}

describe('ViewerStage', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('hides floats over moving content only once the pointer is idle', () => {
    // The idle window runs on fake time.
    vi.useFakeTimers()

    // Over still content the floats show.
    const { rerender } = renderStage('still')
    expect(floatState()).toBe('shown')

    // Content that starts moving keeps the floats for one idle window.
    rerender(
      <ViewerStage media="moving" idleMs={1000}>
        <ViewerFloat at="bc" label="Pages">
          <ViewerFloatPress label="Next page" essential />
        </ViewerFloat>
      </ViewerStage>,
    )
    expect(floatState()).toBe('shown')

    // Nobody touches it, so they leave.
    act(() => vi.advanceTimersByTime(1000))
    expect(floatState()).toBe('hidden')

    // A key brings them back until the idle window ends.
    fireEvent.keyDown(screen.getByRole('toolbar'))
    expect(floatState()).toBe('shown')
    act(() => vi.advanceTimersByTime(1000))
    expect(floatState()).toBe('hidden')
  })

  it('keeps floats one idle window when the content starts to move', () => {
    // The idle window runs on fake time.
    vi.useFakeTimers()

    // Moving content nobody touches loses its floats.
    const { rerender } = renderStage('moving')
    act(() => vi.advanceTimersByTime(1000))
    expect(floatState()).toBe('hidden')

    // A pause shows them, and a play begun off the stage keeps them a window.
    const stage = (media: ViewerMedia) => (
      <ViewerStage media={media} idleMs={1000}>
        <ViewerFloat at="bc" label="Pages">
          <ViewerFloatPress label="Next page" essential />
        </ViewerFloat>
      </ViewerStage>
    )
    rerender(stage('still'))
    act(() => vi.advanceTimersByTime(5000))
    rerender(stage('moving'))
    expect(floatState()).toBe('shown')
    act(() => vi.advanceTimersByTime(1000))
    expect(floatState()).toBe('hidden')
  })

  it('keeps floats while focus is inside them', () => {
    // The idle window runs on fake time.
    vi.useFakeTimers()

    // Over moving content nobody touched, they leave after the idle window.
    renderStage('moving')
    expect(floatState()).toBe('shown')
    act(() => vi.advanceTimersByTime(1000))
    expect(floatState()).toBe('hidden')

    // A hidden float stays in the tab order, and focus in it holds it shown.
    const press = screen.getByRole('button', { name: 'Next page' })
    act(() => press.focus())
    expect(floatState()).toBe('shown')
    act(() => vi.advanceTimersByTime(5000))
    expect(floatState()).toBe('shown')

    // Focus leaving lets them go again.
    act(() => press.blur())
    expect(floatState()).toBe('hidden')
  })
})
