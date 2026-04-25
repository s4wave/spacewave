import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useLandingBackNavigation } from './useLandingBackNavigation.js'

interface MockHistory {
  canGoBack: boolean
  canGoForward: boolean
  goBack: () => void
  goForward: () => void
}

const mockNavigate = vi.fn()
const mockGoBack = vi.fn()
const mockUseHistory = vi.fn()

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  useHistory: () => mockUseHistory() as MockHistory | null,
}))

describe('useLandingBackNavigation', () => {
  afterEach(() => {
    mockNavigate.mockReset()
    mockGoBack.mockReset()
    mockUseHistory.mockReset()
  })

  it('uses tab-local history when a back entry exists', () => {
    mockUseHistory.mockReturnValue({
      canGoBack: true,
      canGoForward: false,
      goBack: mockGoBack,
      goForward: vi.fn(),
    })

    const { result } = renderHook(() => useLandingBackNavigation())

    result.current()

    expect(mockGoBack).toHaveBeenCalledOnce()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('falls back to landing when no back entry exists', () => {
    mockUseHistory.mockReturnValue(null)

    const { result } = renderHook(() => useLandingBackNavigation())

    result.current()

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/' })
    expect(mockGoBack).not.toHaveBeenCalled()
  })
})
