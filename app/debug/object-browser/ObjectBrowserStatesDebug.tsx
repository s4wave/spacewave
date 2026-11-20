import type { ReactNode } from 'react'

import {
  groupRelations,
  initialBrowserState,
  type BrowserScreen,
  type BrowserState,
} from '@s4wave/app/space/browser/object-browser.js'
import {
  nameTypeWith,
  ObjectBrowserView,
  type BrowserData,
} from '@s4wave/app/space/browser/ObjectBrowser.js'
import type { ObjectTypeMetadata } from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type {
  ObjectDeclaration,
  ObjectMetadata,
} from '@s4wave/sdk/world/world.pb.js'
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

// sessionKey is the LLM session of the mock-up's example.
const sessionKey = 'sessions/01JD8/llm-session'

// sessionState is the browser on the session, opened from the sessions list.
const sessionState = (rel: BrowserState['rel']): BrowserState => ({
  nav: {
    view: 'object',
    objectKey: sessionKey,
    list: { view: 'objects', typeId: 'glados/llm-session' },
  },
  rel,
})

// sessionScreen is the session's screen with every read done.
const sessionScreen: BrowserScreen = {
  view: 'object',
  load: 'ready',
  conn: 'up',
  target: 'present',
  count: 49,
}

// sessionObject is the mock-up's session example as the host reads it: the
// objects, relations and declarations GLaDOS declares for its session, Goal,
// Agent and receipts (title, state, time and tokens), a Forge job's state as
// the host declares it, and an untitled Device. It goes through the same
// grouping as a live read.
function sessionObject(): BrowserData {
  const ago = (h: number) => new Date(Date.now() - h * 3600_000)
  const objects: [
    key: string,
    typeId: string,
    declaration: ObjectDeclaration | undefined,
    link?: [predicate: string, direction: 'out' | 'in'],
  ][] = [
    [
      sessionKey,
      'glados/llm-session',
      { title: 'Fix replay stall on thumper', state: 'active', time: ago(2) },
    ],
    [
      'goals/replay-stall',
      'glados/goal',
      { title: 'Replay waits at an operation it cannot read', state: 'open' },
      ['<glados/llm-session/goal>', 'out'],
    ],
    [
      'agents/boss',
      'glados/agent',
      { title: 'Boss', time: ago(300) },
      ['<glados/llm-session/agent>', 'out'],
    ],
    [
      'devices/thumper',
      'spacewave/device',
      undefined,
      ['<glados/runs-on>', 'out'],
    ],
    [
      'jobs/build-glados-thumper',
      'forge/job',
      { state: 'running', time: ago(1) },
      ['<glados/agent-turn-job-session>', 'in'],
    ],
    [
      'sessions/01JD9/llm-session',
      'glados/llm-session',
      { title: 'World regeneration rehearsal', state: 'closed', time: ago(5) },
      ['<glados/llm-session/parent>', 'in'],
    ],
    ...Array.from({ length: 42 }, (_, i): (typeof objects)[number] => [
      'usage/' + String(i + 1).padStart(2, '0'),
      'glados/llm-usage-event',
      {
        title: 'claude-opus-5-5',
        time: ago(2 - i / 30),
        measures: [{ name: 'tokens', value: i < 38 ? 45_000 : 47_500 }],
      },
      ['<glados/llm-usage-event/session>', 'in'],
    ]),
  ]
  const metadata = new Map<string, ObjectMetadata>(
    objects.map(([objectKey, typeId, declaration]) => [
      objectKey,
      { objectKey, typeId, declaration },
    ]),
  )
  const self = keyToIRI(sessionKey)
  const quads = objects.flatMap(([key, , , link]) =>
    link ? [{ key, predicate: link[0], direction: link[1] }] : [],
  )
  const bucket = {
    outgoing: quads
      .filter((q) => q.direction === 'out')
      .map((q) => ({
        subject: self,
        predicate: q.predicate,
        obj: keyToIRI(q.key),
      })),
    incoming: quads
      .filter((q) => q.direction === 'in')
      .map((q) => ({
        subject: keyToIRI(q.key),
        predicate: q.predicate,
        obj: self,
      })),
  }
  const declared = metadata.get(sessionKey)?.declaration
  return {
    view: 'object',
    object: {
      objectKey: sessionKey,
      typeId: 'glados/llm-session',
      title: declared?.title ?? '',
      state: declared?.state ?? '',
      time: declared?.time,
      groups: groupRelations(bucket, metadata),
      truncated: false,
    },
  }
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
// objects; and the mock-up's session example as a graph and as a list, whose
// GLaDOS types no scenario Space can declare without the GLaDOS plugin.
const fixtures: Record<string, Fixture> = {
  session: {
    title: 'Session example',
    state: sessionState('graph'),
    screen: sessionScreen,
    data: sessionObject(),
  },
  'session-list': {
    title: 'Session example, list',
    state: sessionState('list'),
    screen: sessionScreen,
    data: sessionObject(),
  },
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

// nameType names the fixture types as the mock-up does. GLaDOS registers no
// display names yet, so a live browser names its types from their IDs.
const nameType = nameTypeWith(
  new Map<string, ObjectTypeMetadata>(
    Object.entries({
      'glados/llm-session': 'LLM session',
      'glados/goal': 'Goal',
      'glados/agent': 'Agent',
      'glados/llm-usage-event': 'Inference receipt',
      'spacewave/device': 'Device',
      'forge/job': 'Forge job',
    }).map(([typeId, displayName]) => [typeId, { displayName }]),
  ),
)

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
