import { describe, expect, it } from 'vitest'

import {
  deriveTextFileView,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
  type TextFileEvent,
  type TextFileState,
} from './text-file.js'

const original: TextContent = { text: 'original', preview: null }
const theirs: TextContent = { text: 'theirs', preview: null }

// replay applies events in order from the initial state.
function replay(...events: TextFileEvent[]): TextFileState {
  return events.reduce(reduceTextFile, initialTextFileState)
}

// shownText derives the text the viewer shows without a draft.
function shownText(state: TextFileState, read: TextContent): string {
  const view = deriveTextFileView(
    { value: read, loading: false, error: null },
    state,
    true,
  )
  return view.state === 'shown' ? view.text : view.state
}

describe('reduceTextFile', () => {
  it('refuses a plain Save once another writer saved', () => {
    // A draft of the original; another writer's read lands.
    const state = replay(
      { type: 'read', content: original },
      { type: 'edit', text: 'original' },
      { type: 'type', text: 'mine' },
      { type: 'read', content: theirs },
    )

    // Plain Save is refused; Save over theirs starts.
    expect(reduceTextFile(state, { type: 'save', over: false })).toBe(state)
    const over = reduceTextFile(state, { type: 'save', over: true })
    expect(over.draft?.saving).toBe('over')
  })

  it('stops a plain Save on the read at the press', () => {
    // A plain Save starts; the read at the press finds their version.
    const state = replay(
      { type: 'read', content: original },
      { type: 'edit', text: 'original' },
      { type: 'type', text: 'mine' },
      { type: 'save', over: false },
      { type: 'refused', content: theirs },
    )

    // The draft stays, not saving, and a second plain Save is refused.
    expect(state.draft?.saving).toBeNull()
    expect(state.draft?.text).toBe('mine')
    expect(reduceTextFile(state, { type: 'save', over: false })).toBe(state)
  })

  it('shows the saved text, then a read that landed during the save', () => {
    // A save is pressed against the original read.
    const pressed = replay(
      { type: 'read', content: original },
      { type: 'edit', text: 'original' },
      { type: 'type', text: 'mine' },
      { type: 'save', over: false },
    )

    // Landing with no later read shows the saved text at once.
    const saved = reduceTextFile(pressed, { type: 'saved', text: 'mine' })
    expect(shownText(saved, original)).toBe('mine')

    // Another writer's commit read while the save ran shows when it lands.
    const midRead = reduceTextFile(pressed, { type: 'read', content: theirs })
    const after = reduceTextFile(midRead, { type: 'saved', text: 'mine' })
    expect(shownText(after, theirs)).toBe('theirs')

    // A read begun before the press lands during the save with the old text;
    // the saved text still shows.
    const stale: TextContent = { text: 'original', preview: null }
    const oldRead = reduceTextFile(pressed, { type: 'read', content: stale })
    const kept = reduceTextFile(oldRead, { type: 'saved', text: 'mine' })
    expect(shownText(kept, stale)).toBe('mine')
  })
})
