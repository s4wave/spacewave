import { useCallback, useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  resolvePath,
  Route,
  Router,
  Routes,
  type To,
} from '@s4wave/web/router/router.js'

import { SpaceObjectContainer } from './SpaceObjectContainer.js'

const wizardPath = '/u/1/so/space-1/-/wizard/device-1'

const h = vi.hoisted(() => ({
  navigateToRoot: vi.fn(),
  navigateToSubPath: vi.fn(),
  spaceContentsResource: {
    value: {},
    loading: false,
    error: null,
    retry: vi.fn(),
  },
  spaceContext: {
    spaceId: 'space-1',
    objectKey: 'wizard',
    objectPath: 'device-1',
    spaceState: {
      worldContents: {
        objects: [{ objectKey: 'wizard', objectType: 'spacewave/wizard' }],
      },
    },
    spaceWorldResource: { value: null, loading: false, error: null },
  },
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: ({
    onNavigate,
  }: {
    onNavigate?: (to: { path: string }) => void
  }) => (
    <button onClick={() => onNavigate?.({ path: '/login' })}>
      Sign in or create account
    </button>
  ),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContentsContext: {
    useContext: () => h.spaceContentsResource,
  },
  useSessionIndex: () => 1,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({
      ...h.spaceContext,
      navigateToRoot: h.navigateToRoot,
      navigateToSubPath: h.navigateToSubPath,
    }),
  },
}))

vi.mock('@s4wave/app/quickstart/session-handoff.js', () => ({
  getQuickstartInitialObjectHandoff: vi.fn(),
}))

function AppRouteHarness() {
  const [entries, setEntries] = useState([wizardPath])
  const [entryIndex, setEntryIndex] = useState(0)
  const path = entries[entryIndex] ?? wizardPath
  const navigate = useCallback(
    (to: To) => {
      const next = resolvePath(path, to)
      if (to.replace) {
        setEntries((current) => current.with(entryIndex, next))
        return
      }
      setEntries((current) => [...current.slice(0, entryIndex + 1), next])
      setEntryIndex((current) => current + 1)
    },
    [entryIndex, path],
  )
  const goBack = useCallback(() => {
    setEntryIndex((current) => Math.max(0, current - 1))
  }, [])

  return (
    <>
      <output data-testid="current-route">{path}</output>
      <Router path={path} onNavigate={navigate}>
        <Routes fullPath>
          <Route path="/u/:sessionIndex/so/:spaceId/-/*">
            <SpaceObjectContainer />
          </Route>
          <Route path="/login">
            <div>
              <p>Spacewave Cloud login</p>
              <button onClick={goBack}>Back to device wizard</button>
            </div>
          </Route>
        </Routes>
      </Router>
    </>
  )
}

describe('SpaceObjectContainer authentication navigation', () => {
  it('opens the app login route and keeps the device wizard as the back target', () => {
    render(<AppRouteHarness />)

    fireEvent.click(
      screen.getByRole('button', { name: 'Sign in or create account' }),
    )

    expect(screen.getByText('Spacewave Cloud login')).toBeTruthy()
    expect(screen.getByTestId('current-route').textContent).toBe('/login')
    expect(h.navigateToSubPath).not.toHaveBeenCalled()

    fireEvent.click(
      screen.getByRole('button', { name: 'Back to device wizard' }),
    )
    expect(
      screen.getByRole('button', { name: 'Sign in or create account' }),
    ).toBeTruthy()
    expect(screen.getByTestId('current-route').textContent).toBe(wizardPath)
  })
})
