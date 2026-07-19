import type { ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { SessionFlowFrame } from './SessionFlowFrame.js'

const mockNavigate = vi.hoisted(() => vi.fn())
const mockUseHistory = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  useHistory: mockUseHistory,
}))

vi.mock('./SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: ReactNode }) => (
    <div data-testid="session-frame">{children}</div>
  ),
}))

describe('SessionFlowFrame', () => {
  beforeEach(() => {
    cleanup()
    mockNavigate.mockClear()
    mockUseHistory.mockReset()
  })

  it('renders children inside the session frame with a back button', () => {
    mockUseHistory.mockReturnValue(null)

    render(
      <SessionFlowFrame fallbackPath="/u/7">
        <div data-testid="setup-page" />
      </SessionFlowFrame>,
    )

    expect(screen.getByTestId('session-frame')).toBeDefined()
    expect(screen.getByTestId('setup-page')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Back' })).toBeDefined()
  })

  it('records and exits the flow through the history owner', () => {
    const enterFlow = vi.fn()
    const exitFlow = vi.fn()
    mockUseHistory.mockReturnValue({ enterFlow, exitFlow })

    render(
      <SessionFlowFrame fallbackPath="/u/7">
        <div />
      </SessionFlowFrame>,
    )

    expect(enterFlow).toHaveBeenCalledWith('/u/7')
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(exitFlow).toHaveBeenCalledWith('/u/7')
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('navigates to the fallback without a history owner', () => {
    mockUseHistory.mockReturnValue(null)

    render(
      <SessionFlowFrame fallbackPath="/u/7">
        <div />
      </SessionFlowFrame>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/u/7' })
  })
})
