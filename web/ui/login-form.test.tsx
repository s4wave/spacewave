import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { LoginForm } from './login-form.js'

const mockIsDesktop = vi.hoisted(() => ({ value: false }))

vi.mock('@aptre/bldr', () => ({
  get isDesktop() {
    return mockIsDesktop.value
  },
}))

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/ui/turnstile.js', () => ({
  Turnstile: () => null,
}))

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DialogDescription: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DialogFooter: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DialogTitle: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

describe('LoginForm SSO provider visibility', () => {
  beforeEach(() => {
    mockIsDesktop.value = false
  })

  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('shows only configured SSO providers', () => {
    render(
      <LoginForm
        cloudProviderConfig={{
          googleSsoEnabled: true,
          githubSsoEnabled: false,
          turnstileSiteKey: '',
        }}
        onSignInWithSSO={vi.fn()}
      />,
    )

    expect(screen.getByText('Google')).toBeDefined()
    expect(screen.queryByText('GitHub')).toBeNull()
  })

  it('calls onSignInWithSSO directly on desktop without showing browser modal', () => {
    mockIsDesktop.value = true
    const onSignInWithSSO = vi.fn()

    render(
      <LoginForm
        cloudProviderConfig={{
          googleSsoEnabled: true,
          githubSsoEnabled: false,
          turnstileSiteKey: '',
        }}
        onSignInWithSSO={onSignInWithSSO}
      />,
    )

    screen.getByText('Google').click()
    expect(onSignInWithSSO).toHaveBeenCalledWith('google')
    expect(screen.queryByText(/Continue sign-in in your browser/)).toBeNull()
  })

  it('prefills the username field when provided', () => {
    render(
      <LoginForm
        initialUsername="casey"
        cloudProviderConfig={{ turnstileSiteKey: '' }}
      />,
    )

    expect(screen.getByDisplayValue('casey')).toBeDefined()
  })
})

describe('LoginForm clickwrap consent', () => {
  beforeEach(() => {
    mockIsDesktop.value = true
  })

  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('does not show or require clickwrap consent during sign-in', async () => {
    const user = userEvent.setup()
    const onLoginWithPassword = vi.fn().mockResolvedValue({
      type: 'error',
      errorCode: 'wrong_password',
    })

    render(
      <LoginForm
        cloudProviderConfig={{ turnstileSiteKey: '' }}
        onLoginWithPassword={onLoginWithPassword}
      />,
    )

    expect(screen.queryByRole('checkbox')).toBeNull()

    await user.type(screen.getByPlaceholderText('alice'), 'alice')
    await user.type(
      screen.getByPlaceholderText('Enter password'),
      'password123',
    )
    await user.click(
      screen.getByRole('button', { name: 'Continue with password' }),
    )

    await waitFor(() => {
      expect(onLoginWithPassword).toHaveBeenCalledWith(
        'alice',
        'password123',
        '',
      )
    })
  })

  it('shows and requires clickwrap consent during account creation', async () => {
    const user = userEvent.setup()
    const onLoginWithPassword = vi.fn().mockResolvedValue({
      type: 'new_account',
    })
    const onCreateAccountWithPassword = vi.fn().mockResolvedValue({
      sessionIndex: 1,
    })

    render(
      <LoginForm
        cloudProviderConfig={{ turnstileSiteKey: '' }}
        onLoginWithPassword={onLoginWithPassword}
        onCreateAccountWithPassword={onCreateAccountWithPassword}
      />,
    )

    await user.type(screen.getByPlaceholderText('alice'), 'alice')
    await user.type(
      screen.getByPlaceholderText('Enter password'),
      'password123',
    )
    await user.click(
      screen.getByRole('button', { name: 'Continue with password' }),
    )

    await screen.findByPlaceholderText('Confirm password')
    expect(
      screen.getByRole('button', { name: 'Confirm and create account' }),
    ).toHaveProperty('disabled', true)

    await user.type(
      screen.getByPlaceholderText('Confirm password'),
      'password123',
    )
    expect(
      screen.getByRole('button', { name: 'Confirm and create account' }),
    ).toHaveProperty('disabled', true)

    await user.click(screen.getByRole('checkbox'))

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Confirm and create account' }),
      ).toHaveProperty('disabled', false)
    })

    await user.click(
      screen.getByRole('button', { name: 'Confirm and create account' }),
    )

    await waitFor(() => {
      expect(onCreateAccountWithPassword).toHaveBeenCalledWith(
        'alice',
        'password123',
        '',
      )
    })
  })
})
