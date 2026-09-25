import { useCallback, useState } from 'react'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  resolvePath,
  Route,
  Router,
  Routes,
  type To,
} from '@s4wave/web/router/router.js'
import { worldObjectNavigation } from '@s4wave/web/router/HistoryRouter.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'

import { SpaceObjectContainer } from './SpaceObjectContainer.js'

const wizardPath = '/u/1/so/space-1/-/wizard/device-1'

afterEach(cleanup)

const h = vi.hoisted(() => ({
  viewer: vi.fn(),
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
    spaceWorldResource: {
      value: null as null | {
        openNestedWorld: (key: string, signal: AbortSignal) => Promise<unknown>
        getSeqno: () => Promise<{ seqno: bigint }>
        getObject: (key: string) => Promise<{
          getRootRef: () => Promise<{ rev: bigint }>
          release: () => void
        }>
        waitSeqno: (
          seqno: bigint,
          signal: AbortSignal,
        ) => Promise<{ seqno: bigint }>
      },
      loading: false,
      error: null,
      retry: vi.fn(),
    },
  },
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: ({
    onNavigate,
    ...props
  }: {
    onNavigate?: (to: { path: string }) => void
    objectInfo?: { info?: { value?: { objectKey?: string } } }
    path?: string
    worldState?: { value: unknown }
  }) => {
    h.viewer({ onNavigate, ...props })
    return (
      <button onClick={() => onNavigate?.({ path: '/login' })}>
        Sign in or create account
      </button>
    )
  },
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

function makeOuterWorld(
  openNestedWorld: (key: string, signal: AbortSignal) => Promise<unknown>,
) {
  let rev = 1n
  let seqno = 1n
  const waiters: Array<(value: { seqno: bigint }) => void> = []
  const getObject = vi.fn(async () => ({
    getRootRef: async () => ({ rev }),
    release: vi.fn(),
  }))
  const world = {
    openNestedWorld,
    getSeqno: async () => ({ seqno }),
    getObject,
    waitSeqno: vi.fn(
      (target: bigint, signal: AbortSignal) =>
        new Promise<{ seqno: bigint }>((resolve, reject) => {
          if (seqno >= target) {
            resolve({ seqno })
            return
          }
          const onAbort = () => {
            const index = waiters.indexOf(onWrite)
            if (index !== -1) waiters.splice(index, 1)
            reject(new DOMException('Aborted', 'AbortError'))
          }
          const onWrite = (value: { seqno: bigint }) => {
            signal.removeEventListener('abort', onAbort)
            resolve(value)
          }
          waiters.push(onWrite)
          signal.addEventListener('abort', onAbort, { once: true })
        }),
    ),
  }
  const write = (nextRev: bigint) => {
    rev = nextRev
    seqno++
    for (const resolve of waiters.splice(0)) resolve({ seqno })
  }
  return { world, getObject, write }
}

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

describe('nested World route', () => {
  it('replaces a mounted nested viewer only when its outer root changes', async () => {
    const firstRelease = vi.fn()
    const secondRelease = vi.fn()
    const first = { [Symbol.dispose]: firstRelease, result: 'before' }
    const second = { [Symbol.dispose]: secondRelease, result: 'after' }
    const openNestedWorld = vi
      .fn()
      .mockResolvedValueOnce(first)
      .mockResolvedValueOnce(second)
    const outer = makeOuterWorld(openNestedWorld)
    h.spaceContext.spaceWorldResource.value = outer.world
    h.spaceContext.objectKey = 'projection/logbook'
    h.spaceContext.objectPath = 'world/-/orient/plan/first'
    h.viewer.mockClear()

    const view = render(
      <Router path={wizardPath} onNavigate={vi.fn()}>
        <SpaceObjectContainer />
      </Router>,
    )
    await waitFor(() =>
      expect(h.viewer.mock.lastCall?.[0].worldState.value).toBe(first),
    )

    // A Space or Goal write advances the World without replacing this root.
    await act(async () => outer.write(1n))
    await waitFor(() => expect(outer.getObject).toHaveBeenCalledTimes(2))
    expect(openNestedWorld).toHaveBeenCalledTimes(1)
    expect(firstRelease).not.toHaveBeenCalled()

    // Publish a new root under the same key without changing the route or parent.
    await act(async () => outer.write(2n))
    await waitFor(() =>
      expect(h.viewer.mock.lastCall?.[0].worldState.value).toBe(second),
    )
    expect(h.viewer.mock.lastCall?.[0].worldState.value.result).toBe('after')
    expect(openNestedWorld).toHaveBeenCalledTimes(2)
    expect(firstRelease).toHaveBeenCalledTimes(1)
    view.unmount()
    expect(secondRelease).toHaveBeenCalledTimes(1)
  })

  it('decodes the outer key and nested viewer subpath from a deep link', () => {
    const outer = parseObjectUri(
      'projection/logbook/-/world/-/orient/plan/first/-/details',
    )
    const inner = parseObjectUri(outer.path.slice('world/-/'.length))

    expect(outer.objectKey).toBe('projection/logbook')
    expect(inner).toEqual({
      objectKey: 'orient/plan/first',
      path: 'details',
    })
  })

  it('opens the outer object, navigates within the inner viewer, and releases on route change', async () => {
    const release = vi.fn()
    const nestedWorld = { [Symbol.dispose]: release, getReadOnly: () => true }
    const openNestedWorld = vi.fn().mockResolvedValue(nestedWorld)
    h.spaceContext.spaceWorldResource.value =
      makeOuterWorld(openNestedWorld).world
    h.spaceContext.objectKey = 'projection/logbook'
    h.spaceContext.objectPath = 'world/-/orient/plan/first/-/details'
    h.viewer.mockClear()

    const view = render(
      <Router path={wizardPath} onNavigate={vi.fn()}>
        <SpaceObjectContainer />
      </Router>,
    )
    await waitFor(() => expect(h.viewer).toHaveBeenCalled())
    const props = h.viewer.mock.lastCall?.[0]
    expect(openNestedWorld).toHaveBeenCalledWith(
      'projection/logbook',
      expect.any(AbortSignal),
    )
    expect(props.objectInfo.info.value.objectKey).toBe('orient/plan/first')
    expect(props.worldState.value).toBe(nestedWorld)
    expect(props.path).toBe('/details')
    props.onNavigate({ path: 'history' })
    expect(h.navigateToSubPath).toHaveBeenCalledWith(
      'projection/logbook/-/world/-/orient/plan/first/-/details/history',
    )
    props.onNavigate(worldObjectNavigation('plans/second.org'))
    expect(h.navigateToSubPath).toHaveBeenLastCalledWith(
      'projection/logbook/-/world/-/plans/second.org',
    )
    props.onNavigate(worldObjectNavigation('orient/repo', 'plans'))
    expect(h.navigateToSubPath).toHaveBeenLastCalledWith(
      'projection/logbook/-/world/-/orient/repo/-/plans',
    )

    h.spaceContext.objectPath = ''
    view.rerender(
      <Router path={wizardPath} onNavigate={vi.fn()}>
        <SpaceObjectContainer />
      </Router>,
    )
    await waitFor(() => expect(release).toHaveBeenCalledTimes(1))

    h.spaceContext.objectPath = 'world/-/orient/issue/second'
    view.rerender(
      <Router path={wizardPath} onNavigate={vi.fn()}>
        <SpaceObjectContainer />
      </Router>,
    )
    await waitFor(() => expect(openNestedWorld).toHaveBeenCalledTimes(2))
    view.unmount()
    expect(release).toHaveBeenCalledTimes(2)
  })

  it('shows an empty route without opening a World', () => {
    const openNestedWorld = vi.fn()
    h.spaceContext.spaceWorldResource.value =
      makeOuterWorld(openNestedWorld).world
    h.spaceContext.objectKey = 'projection/logbook'
    h.spaceContext.objectPath = 'world/-/'

    render(
      <Router path={wizardPath} onNavigate={vi.fn()}>
        <SpaceObjectContainer />
      </Router>,
    )
    expect(screen.getByRole('status').textContent).toContain('No nested object')
    expect(openNestedWorld).not.toHaveBeenCalled()
  })

  it('shows an open error with retry instead of an indefinite loading state', async () => {
    const openNestedWorld = vi.fn().mockRejectedValue(new Error('Unavailable'))
    h.spaceContext.spaceWorldResource.value =
      makeOuterWorld(openNestedWorld).world
    h.spaceContext.objectKey = 'projection/logbook'
    h.spaceContext.objectPath = 'world/-/orient/plan/first'

    render(
      <Router path={wizardPath} onNavigate={vi.fn()}>
        <SpaceObjectContainer />
      </Router>,
    )
    expect((await screen.findByRole('alert')).textContent).toContain(
      'Unavailable',
    )
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy()
  })
})
