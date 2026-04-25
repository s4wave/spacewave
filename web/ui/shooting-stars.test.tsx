import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render } from '@testing-library/react'

import { TabActiveProvider } from '@s4wave/web/contexts/TabActiveContext.js'
import { ShootingStars } from './shooting-stars.js'

const mockUseDocumentVisibility = vi.hoisted(() => vi.fn(() => 'visible'))

vi.mock('@aptre/bldr-react', () => ({
  useDocumentVisibility: mockUseDocumentVisibility,
}))

describe('ShootingStars', () => {
  beforeEach(() => {
    cleanup()
    vi.useFakeTimers()
    mockUseDocumentVisibility.mockReturnValue('visible')
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('does not start animation work for an inactive tab', () => {
    const requestAnimationFrameMock = vi.fn(() => 1)
    const setTimeoutMock = vi.fn(() => 1)

    vi.stubGlobal('requestAnimationFrame', requestAnimationFrameMock)
    vi.stubGlobal('setTimeout', setTimeoutMock)

    render(
      <TabActiveProvider value={false}>
        <ShootingStars />
      </TabActiveProvider>,
    )

    expect(requestAnimationFrameMock).not.toHaveBeenCalled()
    expect(setTimeoutMock).not.toHaveBeenCalled()
  })

  it('does not start animation work for a hidden document', () => {
    const requestAnimationFrameMock = vi.fn(() => 1)
    const setTimeoutMock = vi.fn(() => 1)

    mockUseDocumentVisibility.mockReturnValue('hidden')
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrameMock)
    vi.stubGlobal('setTimeout', setTimeoutMock)

    render(
      <TabActiveProvider value={true}>
        <ShootingStars />
      </TabActiveProvider>,
    )

    expect(requestAnimationFrameMock).not.toHaveBeenCalled()
    expect(setTimeoutMock).not.toHaveBeenCalled()
  })

  it('stops scheduled work when the tab becomes inactive', () => {
    const requestAnimationFrameMock = vi.fn(() => 7)
    const cancelAnimationFrameMock = vi.fn()
    const clearTimeoutMock = vi.spyOn(globalThis, 'clearTimeout')

    vi.stubGlobal('requestAnimationFrame', requestAnimationFrameMock)
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrameMock)

    const view = render(
      <TabActiveProvider value={true}>
        <ShootingStars />
      </TabActiveProvider>,
    )

    act(() => {
      vi.advanceTimersByTime(0)
    })

    expect(requestAnimationFrameMock).toHaveBeenCalledTimes(1)

    view.rerender(
      <TabActiveProvider value={false}>
        <ShootingStars />
      </TabActiveProvider>,
    )

    expect(cancelAnimationFrameMock).toHaveBeenCalledWith(7)
    expect(clearTimeoutMock).toHaveBeenCalled()
  })
})
