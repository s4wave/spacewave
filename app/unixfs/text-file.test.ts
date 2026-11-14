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
    { value: null, loading: false, error: null },
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
      { type: 'refused' },
      { type: 'back-read', content: theirs },
    )

    // The draft stays, not saving, shows their text, and a second plain
    // Save is refused.
    expect(state.draft?.saving).toBeNull()
    expect(state.draft?.text).toBe('mine')
    const view = viewOf(state)
    expect(view.state === 'editing' && view.changed?.text).toBe('theirs')
    expect(reduceTextFile(state, { type: 'save', over: false })).toBe(state)
  })

  it('offers Save again when a refusal brings only a new version', () => {
    // The store refuses a plain save; the read back holds the draft's base
    // text at another version.
    const same = read('original', 4)
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'refused' },
      { type: 'back-read', content: same },
    )

    // The draft moves onto that version, so plain Save expects it.
    expect(state.draft?.from).toBe(same)
    const view = viewOf(state)
    expect(view.state === 'editing' && view.presses).toContain('save')
  })

  it('keeps the version a save expects while it runs', () => {
    // A read of the base text at another version lands during the save, the
    // store refuses the save, and the read back shows that version again.
    const same = read('original', 4)
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'read', content: same },
      { type: 'refused' },
      { type: 'back-read', content: same },
    )

    // The refusal named the expected version, so the read back is not stale.
    expect(state.back.state).toBe('idle')
    expect(state.draft?.from).toBe(same)
  })

  it('starts the next draft from the version a save read back', () => {
    // A save lands; the read after it brings the new version.
    const after = read('mine', 3)
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine' },
      { type: 'back-read', content: after },
    )

    // The saved text shows, and the next draft expects its version.
    const view = viewOf(state)
    expect(view.state === 'shown' && view.text).toBe('mine')
    const next = reduceTextFile(state, { type: 'edit' })
    expect(next.draft?.from).toBe(after)
  })

  it('shows a landed save until a read shows its version', () => {
    // A save lands; the file's own handle still reads the version it replaced.
    const saved = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine' },
      { type: 'read', content: original },
    )

    // The saved text shows, and a draft of it waits to save.
    const view = viewOf(saved)
    expect(view.state === 'shown' && view.text).toBe('mine')
    const next = reduceTextFile(reduceTextFile(saved, { type: 'edit' }), {
      type: 'type',
      text: 'more',
    })
    expect(reduceTextFile(next, { type: 'save', over: false })).toBe(next)
    const waiting = viewOf(next)
    expect(waiting.state === 'editing' && waiting.presses).toContain('savewait')

    // The first read at another version with the saved text is its version.
    const after = read('mine', 3)
    const landed = reduceTextFile(next, { type: 'read', content: after })
    expect(landed.draft?.from).toBe(after)
  })

  it('leaves Read again when the read after a save shows the old version', () => {
    // A save lands; the read back still shows the version it replaced.
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine' },
      { type: 'back-read', content: original },
    )

    // The view keeps the saved text and offers the read again.
    const view = viewOf(state)
    expect(view.state === 'shown' && view.text).toBe('mine')
    expect(view.state === 'shown' && view.awaits?.back.state).toBe('failed')
  })

  it("takes another writer's text read after a landed save as theirs", () => {
    // A draft of a landed save; the first read after it holds other text.
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine' },
      { type: 'edit' },
      { type: 'type', text: 'more' },
      { type: 'read', content: theirs },
    )

    // It is another writer's version: Save over theirs, no plain Save.
    const view = viewOf(state)
    expect(view.state === 'editing' && view.changed?.text).toBe('theirs')
    expect(view.state === 'editing' && view.presses).toContain('saveover')
  })

  it('offers Save over theirs when the read after a refusal failed', () => {
    // The store refuses a plain save, and reading their version fails.
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'refused' },
      { type: 'back-failed', error: 'offline' },
    )

    // Save over theirs and Read again are offered; plain Save is refused,
    // even after a read that still shows the refused version.
    const view = viewOf(state)
    expect(view.state === 'editing' && view.presses).toContain('saveover')
    expect(view.state === 'editing' && view.awaits?.what).toBe('theirs')
    const reread = reduceTextFile(state, { type: 'read', content: original })
    expect(reduceTextFile(reread, { type: 'save', over: false })).toBe(reread)
  })

  it('never moves a draft onto a preview with the text it started from', () => {
    // Another writer grows the file past the read limit; the preview's text
    // is the draft's base.
    const cut = { ...read('original', 5), preview: 'large' as const }
    const state = replay(...drafted, { type: 'read', content: cut })

    // It is their version, so a plain Save is refused.
    expect(state.draft?.from).toBe(original)
    expect(reduceTextFile(state, { type: 'save', over: false })).toBe(state)
  })

  it('opens no draft of a preview', () => {
    const cut = { ...read('original', 5), preview: 'encoding' as const }
    const state = replay({ type: 'read', content: cut }, { type: 'edit' })
    expect(state.draft).toBeNull()
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
