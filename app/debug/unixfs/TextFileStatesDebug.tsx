import type { ReactNode } from 'react'

import { Toolbar } from '@s4wave/web/editors/file-browser/Toolbar.js'
import { useParams } from '@s4wave/web/router/router.js'

import { TextFileFrame } from '@s4wave/app/unixfs/UnixFSTextFileViewer.js'
import {
  deriveTextFileView,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
  type TextFileEvent,
  type TextFileRead,
} from '@s4wave/app/unixfs/text-file.js'

// noop stands in for every press; a fixture holds one state.
const noop = () => {}

const notes: TextContent = {
  preview: null,
  version: new Uint8Array([1]),
  text: Array.from(
    { length: 40 },
    (_, i) =>
      `${i + 1}. Field note: the rover crossed ridge ${i + 1} at dusk and logged the soil sample.`,
  ).join('\n'),
}
const theirs: TextContent = {
  preview: null,
  version: new Uint8Array([2]),
  text: notes.text.replace('ridge 1 ', 'the first ridge '),
}
const mine = notes.text + '\n41. Mine: a draft line not yet saved.'

const shown = (content: TextContent): TextFileRead => ({
  value: content,
  loading: false,
  error: null,
})
const waiting: TextFileRead = { value: null, loading: true, error: null }
const failed = (message: string): TextFileRead => ({
  value: null,
  loading: false,
  error: new Error(message),
})

interface Fixture {
  // reads are the reads in order; each landed one is dispatched as the
  // viewer's effect does, and the last is the read the view derives from.
  reads: TextFileRead[]
  // events are the presses and save results after the first read.
  events?: TextFileEvent[]
  readOnly?: string
  // nav draws the presses in the file browser's navigator bar.
  nav?: boolean
}

const edit: TextFileEvent = { type: 'edit' }
const type: TextFileEvent = { type: 'type', text: mine }

// fixtures are every state the viewer derives, built from its own events.
const fixtures: Record<string, Fixture> = {
  waiting: { reads: [waiting] },
  failed: { reads: [failed('stream closed')] },
  shown: { reads: [shown(notes)] },
  preview: { reads: [shown({ ...notes, preview: 'large' })] },
  encoding: {
    reads: [
      shown({
        ...notes,
        text: 'Rover log \uFFFD\uFFFD 0x7f',
        preview: 'encoding',
      }),
    ],
  },
  empty: { reads: [shown({ ...notes, text: '' })] },
  readonly: {
    reads: [shown(notes)],
    readOnly: 'Read-only: a file in a commit',
  },
  rereading: { reads: [shown(notes), { ...shown(notes), loading: true }] },
  editing: { reads: [shown(notes)], events: [edit] },
  dirty: { reads: [shown(notes)], events: [edit, type] },
  saving: {
    reads: [shown(notes)],
    events: [edit, type, { type: 'save', over: false }],
  },
  'save-failed': {
    reads: [shown(notes)],
    events: [
      edit,
      type,
      { type: 'save', over: false },
      { type: 'save-failed', error: 'the Space refused the write' },
    ],
  },
  'just-saved': {
    reads: [shown(notes)],
    events: [
      edit,
      type,
      { type: 'save', over: false },
      { type: 'saved', text: mine },
      {
        type: 'back-read',
        content: { text: mine, preview: null, version: new Uint8Array([3]) },
      },
    ],
  },
  confirming: {
    reads: [shown(notes)],
    events: [
      edit,
      type,
      { type: 'save', over: false },
      { type: 'saved', text: mine },
    ],
  },
  refused: {
    reads: [shown(notes)],
    events: [
      edit,
      type,
      { type: 'save', over: false },
      { type: 'refused' },
      { type: 'back-failed', error: 'stream closed' },
    ],
  },
  'read-failed': {
    reads: [shown(notes), failed('stream closed')],
    events: [edit, type],
  },
  changed: { reads: [shown(notes), shown(theirs)], events: [edit, type] },
  theirs: {
    reads: [shown(notes), shown(theirs)],
    events: [edit, type, { type: 'theirs', show: true }],
  },
  nav: { reads: [shown(notes)], nav: true },
  'nav-dirty': { reads: [shown(notes)], events: [edit, type], nav: true },
}

const path = '/Field notes/notes.md'

// fixtureView replays a fixture's reads and presses through the viewer's
// reducer, as its read effect and presses would, and derives the view.
function fixtureView(fixture: Fixture) {
  // The first read lands before any press.
  const [first, ...later] = fixture.reads
  let state = initialTextFileState
  if (first.value) {
    state = reduceTextFile(state, { type: 'read', content: first.value })
  }

  // The presses, then any later reads, land in order.
  for (const event of fixture.events ?? []) {
    state = reduceTextFile(state, event)
  }
  for (const read of later) {
    if (read.value) {
      state = reduceTextFile(state, { type: 'read', content: read.value })
    }
  }

  // The view derives from the last read.
  const read = fixture.reads[fixture.reads.length - 1]
  return deriveTextFileView(read, state, !fixture.readOnly)
}

// renderNavigator draws the presses in the file browser's navigator.
function renderNavigator(presses: ReactNode) {
  return <Toolbar currentPath={path} canGoUp actions={presses} />
}

/**
 * TextFileStatesDebug renders the text file viewer in one fixture state, with
 * the viewport as the pane, for the viewer sweep. Each state is built through
 * the viewer's reducer and derivation: waiting, failed, shown, preview,
 * encoding, empty, readonly, rereading, editing, dirty, saving, save-failed,
 * just-saved, confirming, refused, read-failed, changed and theirs; nav and nav-dirty draw the
 * presses in the file browser's navigator. An unknown or missing state shows
 * shown.
 */
export function TextFileStatesDebug() {
  // Read the fixture state from the route.
  const { state = 'shown' } = useParams()
  const fixture = fixtures[state] ?? fixtures.shown

  return (
    <div className="viewer-pane bg-file-back flex h-full w-full flex-col">
      <TextFileFrame
        view={fixtureView(fixture)}
        path={path}
        readOnly={fixture.readOnly}
        description="Markdown, 3.4 KB"
        renderBar={fixture.nav ? renderNavigator : undefined}
        dispatch={noop}
        onSave={noop}
        retryRead={noop}
        readBack={noop}
      />
    </div>
  )
}
