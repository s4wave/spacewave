import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import { AppSignup } from './AppSignup.js'

const mockNavigate = vi.fn()

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: ({ to, replace }: { to: string; replace?: boolean }) => (
    <div data-testid="navigate" data-to={to} data-replace={String(replace)} />
  ),
}))

describe('AppSignup', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('redirects to /login', () => {
    const { getByTestId } = render(<AppSignup />)
    const nav = getByTestId('navigate')
    expect(nav.getAttribute('data-to')).toBe('/login')
    expect(nav.getAttribute('data-replace')).toBe('true')
  })
})
