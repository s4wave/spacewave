import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RouterProvider } from '@s4wave/web/router/router.js'

import { AppRoutes } from './AppRoutes.js'

vi.mock('@s4wave/app/session/SessionSelector.js', () => ({
  SessionSelector: () => (
    <div data-testid="session-selector-route">Session selector route</div>
  ),
}))

vi.mock('@s4wave/app/session/RecoveryPage.js', () => ({
  RecoveryPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/SSOFinishPage.js', () => ({
  SSOFinishPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/SSOLinkFinishPage.js', () => ({
  SSOLinkFinishPage: () => null,
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
  PasskeyWaitPage: () => null,
}))

vi.mock('@s4wave/app/provider/spacewave/PasskeyConfirmPage.js', () => ({
  PasskeyConfirmPage: () => null,
}))

vi.mock('@s4wave/app/auth/HandoffPage.js', () => ({
  HandoffPage: () => null,
}))

vi.mock('@s4wave/app/auth/LaunchLoginPage.js', () => ({
  LaunchLoginPage: () => null,
}))

vi.mock('../AppLogin.js', () => ({
  AppLogin: () => null,
}))

vi.mock('../AppSignup.js', () => ({
  AppSignup: () => null,
}))

vi.mock('@s4wave/web/debug/CanvasGraphLinksDebug.js', () => ({
  CanvasGraphLinksDebug: () => null,
}))

vi.mock('@s4wave/web/debug/DebugDbBench.js', () => ({
  DebugDbBench: () => null,
}))

vi.mock('@s4wave/web/debug/ForgeViewerDebug.js', () => ({
  ForgeViewerDebug: () => null,
}))

vi.mock('@s4wave/web/debug/HDRDebug.js', () => ({
  HDRDebug: () => null,
}))

vi.mock('@s4wave/web/debug/LayoutDebug.js', () => ({
  LayoutDebug: () => null,
}))

vi.mock('@s4wave/web/debug/LayoutColorsDebug.js', () => ({
  LayoutColorsDebug: () => null,
}))

vi.mock('@s4wave/web/debug/LoadingDebug.js', () => ({
  LoadingDebug: () => null,
}))

vi.mock('@s4wave/web/debug/SessionSettingsDebug.js', () => ({
  SessionSettingsDebug: () => null,
}))

vi.mock('@s4wave/web/debug/UnixFSBrowserDebug.js', () => ({
  UnixFSBrowserDebug: () => null,
}))

const debugIndexTargets = [
  '#/debug/db/bench',
  '#/debug/hdr',
  '#/debug/ui/layout',
  '#/debug/ui/layout/colors',
  '#/debug/ui/canvas-graph-links',
  '#/debug/ui/session-settings',
  '#/debug/ui/loading',
  '#/debug/ui/forge-viewer',
  '#/debug/ui/unixfs-browser',
]

function renderAppRoute(path: string) {
  const navigate = vi.fn()
  render(
    <RouterProvider path={path} onNavigate={navigate}>
      <AppRoutes />
    </RouterProvider>,
  )
  return navigate
}

describe('AppRoutes direct index routes', () => {
  afterEach(() => {
    cleanup()
  })

  it('resolves /sessions through the app and auth route tables to the session selector', async () => {
    const navigate = renderAppRoute('/sessions')

    expect(await screen.findByTestId('session-selector-route')).toBeDefined()
    expect(screen.getByText('Session selector route')).toBeDefined()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('resolves /debug through the app and debug route tables to the debug index', async () => {
    renderAppRoute('/debug')

    const heading = await screen.findByRole('heading', { name: 'Debug tools' })
    expect(heading).toBeDefined()
    const linkTargets = screen
      .getAllByRole('link')
      .map((link) => link.getAttribute('href'))
    expect(linkTargets).toEqual(expect.arrayContaining(debugIndexTargets))
  })
})
