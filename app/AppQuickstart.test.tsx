import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

interface MockRouteParams {
  quickstartId?: string
}

const mockUseParams = vi.hoisted(() => vi.fn<() => MockRouteParams>())
const mockNavigate = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => mockUseParams(),
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: ({ to }: { to: string }) => (
    <div data-testid="navigate">{to}</div>
  ),
}))

vi.mock('@s4wave/app/quickstart/Quickstart.js', () => ({
  Quickstart: ({ quickstartId }: { quickstartId: string }) => (
    <div data-testid="quickstart">{quickstartId}</div>
  ),
}))

import { AppQuickstart } from './AppQuickstart.js'

describe('AppQuickstart', () => {
  beforeEach(() => {
    mockUseParams.mockReset()
    mockNavigate.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('renders static quickstart ids', () => {
    mockUseParams.mockReturnValue({ quickstartId: 'drive' })

    render(<AppQuickstart />)

    expect(screen.getByTestId('quickstart').textContent).toBe('drive')
  })

  it('shows a not-available fallback for unavailable public quickstart routes', () => {
    mockUseParams.mockReturnValue({ quickstartId: 'cdn' })

    render(<AppQuickstart />)

    expect(
      screen.getByRole('heading', { name: 'Quickstart not available' }),
    ).toBeTruthy()
    expect(
      screen.getByText(
        'The "cdn" quickstart is not part of the current public Spacewave catalog. Choose an available quickstart from the home page.',
      ),
    ).toBeTruthy()
    const backButton = screen.getByRole('button', { name: 'Back to home' })
    expect(backButton).toBeTruthy()
    expect(screen.queryByTestId('navigate')).toBeNull()
    expect(screen.queryByTestId('quickstart')).toBeNull()

    fireEvent.click(backButton)

    expect(mockNavigate).toHaveBeenCalledWith({ path: '/' })
  })
})
