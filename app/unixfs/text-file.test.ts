import { describe, expect, it } from 'vitest'

import {
  deriveTextFileView,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
  type TextFileEvent,
  type TextFileState,
} from './text-file.js'

// read is a whole text read at version v.
function read(text: string, v: number): TextContent {
  return { text, preview: null, version: new Uint8Array([v]) }
}

const original = read('original', 1)
const theirs = read('theirs', 2)

// replay applies events in order from the initial state.
function replay(...events: TextFileEvent[]): TextFileState {
  return events.reduce(reduceTextFile, initialTextFileState)
}

// drafted is a dirty draft of the original read.
const drafted: TextFileEvent[] = [
  { type: 'read', content: original },
  { type: 'edit' },
  { type: 'type', text: 'mine' },
]

// viewOf derives the view over a landed read of the state's latest version.
function viewOf(state: TextFileState) {
  return deriveTextFileView(
    { value: state.seen, loading: false, error: null },
    state,
    true,
  )
}

describe('reduceTextFile', () => {
  it('refuses a plain Save once another writer saved', () => {
    // A draft of the original; another writer's version lands.
    const state = replay(...drafted, { type: 'read', content: theirs })

    // Plain Save is refused; Save over theirs starts.
    expect(reduceTextFile(state, { type: 'save', over: false })).toBe(state)
    const over = reduceTextFile(state, { type: 'save', over: true })
    expect(over.draft?.saving).toBe('over')
  })

  it('shows the change when the store refuses a plain Save', () => {
    // A plain Save starts; the store refuses it and the read back lands.
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'refused', content: theirs },
    )

    // The draft stays, not saving, shows their text, and a second plain
    // Save is refused.
    expect(state.draft?.saving).toBeNull()
    expect(state.draft?.text).toBe('mine')
    const view = viewOf(state)
    expect(view.state === 'editing' && view.changed).toBe('theirs')
    expect(reduceTextFile(state, { type: 'save', over: false })).toBe(state)
  })

  it('offers Save again when a refusal brings only a new version', () => {
    // The store refuses a plain save; the read back holds the draft's base
    // text at another version.
    const same = read('original', 4)
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'refused', content: same },
    )

    // The draft moves onto that version, so plain Save expects it.
    expect(state.draft?.from).toBe(same)
    const view = viewOf(state)
    expect(view.state === 'editing' && view.presses).toContain('save')
  })

  it('starts the next draft from the version a save read back', () => {
    // A save lands; the read after it brings the new version.
    const after = read('mine', 3)
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', content: after },
    )

    // The saved text shows, and the next draft expects its version.
    const view = viewOf(state)
    expect(view.state === 'shown' && view.text).toBe('mine')
    const next = reduceTextFile(state, { type: 'edit' })
    expect(next.draft?.from).toBe(after)
  })

  it('waits for a read when the read after a save failed', () => {
    // A save lands but its read back failed.
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', content: null },
    )

    // Nothing stale shows and Edit waits for the viewer's own read.
    const view = deriveTextFileView(
      { value: null, loading: true, error: null },
      state,
      true,
    )
    expect(view.state).toBe('waiting')
  })

  it('moves a draft onto a new version with the text it started from', () => {
    // Another version with the same text lands under the draft.
    const same = read('original', 4)
    const state = replay(...drafted, { type: 'read', content: same })

    // It is no change, and a plain Save expects the new version.
    const view = viewOf(state)
    expect(view.state === 'editing' && view.changed).toBeNull()
    expect(state.draft?.from).toBe(same)
  })
})
