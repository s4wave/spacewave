import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

const mockUseRootResource = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())
const mockNavigate = vi.hoisted(() => vi.fn())
const mockRetry = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: mockUseRootResource,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: mockUseResource,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/router/Redirect.js', () => ({
  Redirect: ({ to }: { to: string }) => <div>redirect:{to}</div>,
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: ({ to }: { to: string }) => <div>navigate:{to}</div>,
}))

import { Quickstart } from './Quickstart.js'

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('Quickstart', () => {
  beforeEach(() => {
    mockUseRootResource.mockReturnValue({})
    mockRetry.mockReset()
    mockNavigate.mockReset()
  })

  it('shows a back button for local quickstart setup failures', () => {
    mockUseResource.mockImplementation(
      (
        _root: unknown,
        _factory: unknown,
        _deps: unknown,
        opts?: { enabled?: boolean },
      ) =>
        opts?.enabled ?
          {
            error: new Error('local setup failed'),
            loading: false,
            retry: mockRetry,
            value: null,
          }
        : {
            error: null,
            loading: false,
            retry: mockRetry,
            value: null,
          },
    )

    render(<Quickstart quickstartId="local" />)

    fireEvent.click(screen.getByRole('button', { name: 'Back to home' }))

    expect(screen.getByText('local setup failed')).toBeDefined()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '../../' })
  })

  it('shows a back button for space quickstart setup failures', () => {
    mockUseResource.mockImplementation(
      (
        _root: unknown,
        _factory: unknown,
        _deps: unknown,
        opts?: { enabled?: boolean },
      ) =>
        opts?.enabled ?
          {
            error: new Error('space setup failed'),
            loading: false,
            retry: mockRetry,
            value: null,
          }
        : {
            error: null,
            loading: false,
            retry: mockRetry,
            value: null,
          },
    )

    render(<Quickstart quickstartId="v86" />)

    fireEvent.click(screen.getByRole('button', { name: 'Back to home' }))

    expect(screen.getByText('space setup failed')).toBeDefined()
    expect(mockNavigate).toHaveBeenCalledWith({ path: '../../' })
  })
})
