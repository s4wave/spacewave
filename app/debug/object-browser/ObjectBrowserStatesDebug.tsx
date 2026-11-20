import type { ReactNode } from 'react'

import {
  initialBrowserState,
  type BrowserScreen,
  type BrowserState,
} from '@s4wave/app/space/browser/object-browser.js'
import {
  nameTypeWith,
  ObjectBrowserView,
  type BrowserData,
} from '@s4wave/app/space/browser/ObjectBrowser.js'
import { useParams } from '@s4wave/web/router/router.js'

// noop stands in for every press the fixtures do not follow.
const noop = () => {}

// typesState is the browser on its first screen.
const typesState: BrowserState = initialBrowserState

// objectState is the browser on an object opened from a type's list.
const objectState: BrowserState = {
  nav: {
    view: 'object',
    objectKey: 'goals/dock',
    list: { view: 'objects', typeId: 'glados/goal' },
  },
  rel: 'graph',
}

// Fixture is one state of ObjectBrowser.tla the live scenario cannot reach on
// demand.
interface Fixture {
  title: string
  state: BrowserState
  screen: BrowserScreen
  data: BrowserData | null
  error?: string
}

// fixtures are the states a scenario Space does not hold still for: the
// connection down, a failed read, a read in flight, and a Space with no
// objects.
const fixtures: Record<string, Fixture> = {
  offline: {
    title: 'Cannot load: offline',
    state: objectState,
    screen: {
      view: 'object',
      load: 'loading',
      conn: 'down',
      target: 'present',
      count: 12,
    },
    data: null,
    error: 'connection to the Space closed',
  },
  failed: {
    title: 'Cannot load: the read failed',
    state: typesState,
    screen: {
      view: 'types',
      load: 'failed',
      conn: 'up',
      target: 'present',
      count: 12,
    },
    data: null,
    error: 'world: transaction discarded',
  },
  loading: {
    title: 'Loading',
    state: typesState,
    screen: {
      view: 'types',
      load: 'loading',
      conn: 'up',
      target: 'present',
      count: 0,
    },
    data: null,
  },
  empty: {
    title: 'No objects',
    state: typesState,
    screen: {
      view: 'types',
      load: 'ready',
      conn: 'up',
      target: 'present',
      count: 0,
    },
    data: { view: 'types', types: [] },
  },
}

// nameType names the fixture types without the registry.
const nameType = nameTypeWith()

/**
 * ObjectBrowserStatesDebug draws the object browser in one state named by
 * the route, or every state when none is named.
 */
export function ObjectBrowserStatesDebug() {
  const { state } = useParams()
  const names = state && fixtures[state] ? [state] : Object.keys(fixtures)
  return (
    <div className="flex flex-col gap-6 overflow-auto p-4">
      {names.map((name) => (
        <FixtureFrame key={name} title={fixtures[name].title}>
          <ObjectBrowserView
            state={fixtures[name].state}
            screen={fixtures[name].screen}
            data={fixtures[name].data}
            error={fixtures[name].error}
            nameType={nameType}
            onEvent={noop}
            onRetry={noop}
            onOpen={noop}
            onMore={noop}
          />
        </FixtureFrame>
      ))}
    </div>
  )
}

function FixtureFrame({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section
      aria-label={title}
      className="border-foreground/10 max-w-xl rounded-md border p-3"
    >
      <h2 className="text-foreground-alt mb-2 text-xs">{title}</h2>
      {children}
    </section>
  )
}
