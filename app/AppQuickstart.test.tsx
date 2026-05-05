import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

interface MockRouteParams {
  quickstartId?: string
}

const mockUseParams = vi.hoisted(() => vi.fn<() => MockRouteParams>())

vi.mock('@s4wave/web/router/router.js', () => ({
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
  })

  afterEach(() => {
    cleanup()
  })

  it('renders static quickstart ids', () => {
    mockUseParams.mockReturnValue({ quickstartId: 'drive' })

    render(<AppQuickstart />)

    expect(screen.getByTestId('quickstart').textContent).toBe('drive')
  })

  it('redirects unknown dynamic quickstart ids deterministically', () => {
    mockUseParams.mockReturnValue({ quickstartId: 'glados-workspace' })

    render(<AppQuickstart />)

    expect(screen.getByTestId('navigate').textContent).toBe('/')
  })
})
