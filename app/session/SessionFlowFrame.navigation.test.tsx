import { useState, type ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { Redirect } from '@s4wave/web/router/Redirect.js'
import { HistoryRouter } from '@s4wave/web/router/HistoryRouter.js'
import {
  resolvePath,
  Route,
  Routes,
  useNavigate,
  type To,
} from '@s4wave/web/router/router.js'

import { SessionFlowFrame } from './SessionFlowFrame.js'

vi.mock('./SessionFrame.js', () => ({
  SessionFrame: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

function EntryPage() {
  const navigate = useNavigate()
  return (
    <button onClick={() => navigate({ path: '/u/7/plan' })}>
      Finish setting up your account
    </button>
  )
}

function PlanPage() {
  const navigate = useNavigate()
  return (
    <SessionFlowFrame fallbackPath="/u/7">
      <button onClick={() => navigate({ path: '/u/7/plan/upgrade' })}>
        Start with Cloud
      </button>
    </SessionFlowFrame>
  )
}

function LoginPage() {
  const navigate = useNavigate()
  return (
    <SessionFlowFrame fallbackPath="/u/7">
      <h1>Create a Cloud Account</h1>
      <button onClick={() => navigate({ path: '../../' })}>
        Back to plan selection
      </button>
    </SessionFlowFrame>
  )
}

function FlowRoutes() {
  return (
    <Routes fullPath>
      <Route path="/u/7/context">
        <EntryPage />
      </Route>
      <Route path="/u/7/plan">
        <PlanPage />
      </Route>
      <Route path="/u/7/plan/upgrade">
        <SessionFlowFrame fallbackPath="/u/7">
          <Redirect to="login" />
        </SessionFlowFrame>
      </Route>
      <Route path="/u/7/plan/upgrade/login">
        <LoginPage />
      </Route>
      <Route path="/u/7">
        <div>session root fallback</div>
      </Route>
    </Routes>
  )
}

function AccountSetupNavigationHarness() {
  const [path, setPath] = useState('/u/7/context')
  const handleNavigate = (to: To) => setPath(resolvePath(path, to))

  return (
    <HistoryRouter path={path} onNavigate={handleNavigate}>
      <output>{path}</output>
      <FlowRoutes />
    </HistoryRouter>
  )
}

describe('SessionFlowFrame account setup navigation', () => {
  it('returns to the pre-flow context after the login round trip', async () => {
    render(<AccountSetupNavigationHarness />)

    fireEvent.click(screen.getByText('Finish setting up your account'))
    fireEvent.click(screen.getByText('Start with Cloud'))

    await screen.findByText('Create a Cloud Account')
    fireEvent.click(screen.getByText('Back to plan selection'))

    await waitFor(() => {
      expect(screen.getByText('Start with Cloud')).toBeDefined()
    })
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(screen.getByText('Finish setting up your account')).toBeDefined()
    expect(screen.queryByText('Create a Cloud Account')).toBeNull()
  })
})
