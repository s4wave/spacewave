import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AuthRoutes } from './AuthRoutes.js'

vi.mock('@s4wave/web/router/router.js', () => ({
  Route: ({ path, children }: { path: string; children: React.ReactNode }) => (
    <div data-testid="route" data-path={path}>
      {children}
    </div>
  ),
}))

vi.mock('@s4wave/app/session/SessionSelector.js', () => ({
  SessionSelector: () => null,
}))

vi.mock('@s4wave/app/session/RecoveryPage.js', () => ({
  RecoveryPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/SSOFinishPage.js', () => ({
  SSOFinishPage: () => <div data-testid="sso-finish-page" />,
}))

vi.mock('@s4wave/app/provider/spacewave/SSOLinkFinishPage.js', () => ({
  SSOLinkFinishPage: () => <div data-testid="sso-link-finish-page" />,
}))

vi.mock('@s4wave/app/provider/spacewave/SSOWaitPage.js', () => ({
  SSOWaitPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/SSOConfirmPage.js', () => ({
  SSOConfirmPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/PasskeyPage.js', () => ({
  PasskeyPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/PasskeyWaitPage.js', () => ({
  PasskeyWaitPage: () => <div data-testid="passkey-wait-page" />,
}))

vi.mock('@s4wave/app/provider/spacewave/PasskeyConfirmPage.js', () => ({
  PasskeyConfirmPage: () => <div data-testid="passkey-confirm-page" />,
}))

vi.mock('@s4wave/app/auth/HandoffPage.js', () => ({
  HandoffPage: () => null,
}))

vi.mock('@s4wave/app/auth/LaunchLoginPage.js', () => ({
  LaunchLoginPage: () => <div data-testid="launch-login-page" />,
}))

vi.mock('../AppLogin.js', () => ({
  AppLogin: () => null,
}))

vi.mock('../AppSignup.js', () => ({
  AppSignup: () => null,
}))

describe('AuthRoutes', () => {
  afterEach(() => {
    cleanup()
  })

  it('keeps the browser SSO finish route mounted', () => {
    render(<>{AuthRoutes}</>)

    const routes = screen.getAllByTestId('route')
    expect(
      routes.some(
        (route) =>
          route.getAttribute('data-path') === '/auth/sso/finish/:nonce',
      ),
    ).toBe(true)
    expect(screen.getByTestId('sso-finish-page')).toBeDefined()
  })

  it('mounts the settings SSO link finish route', () => {
    render(<>{AuthRoutes}</>)

    const routes = screen.getAllByTestId('route')
    expect(
      routes.some(
        (route) =>
          route.getAttribute('data-path') === '/auth/sso/link/:provider/finish',
      ),
    ).toBe(true)
    expect(screen.getByTestId('sso-link-finish-page')).toBeDefined()
  })

  it('mounts the web passkey routes and removes the desktop ceremony route', () => {
    render(<>{AuthRoutes}</>)

    const routes = screen.getAllByTestId('route')
    expect(
      routes.some(
        (route) => route.getAttribute('data-path') === '/auth/passkey/wait',
      ),
    ).toBe(true)
    expect(
      routes.some(
        (route) => route.getAttribute('data-path') === '/auth/passkey/confirm',
      ),
    ).toBe(true)
    expect(
      routes.some(
        (route) => route.getAttribute('data-path') === '/auth/passkey/desktop',
      ),
    ).toBe(false)
    expect(
      routes.some(
        (route) => route.getAttribute('data-path') === '/auth/passkey',
      ),
    ).toBe(true)
    expect(screen.getByTestId('passkey-wait-page')).toBeDefined()
    expect(screen.getByTestId('passkey-confirm-page')).toBeDefined()
  })

  it('mounts the auth launch route for prefilled app handoff', () => {
    render(<>{AuthRoutes}</>)

    const routes = screen.getAllByTestId('route')
    expect(
      routes.some(
        (route) => route.getAttribute('data-path') === '/auth/launch/:username',
      ),
    ).toBe(true)
    expect(screen.getByTestId('launch-login-page')).toBeDefined()
  })
})
