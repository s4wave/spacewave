import { describe, expect, it } from 'vitest'

import {
  decodeText,
  deriveTextFileView,
  encodeText,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
  type TextFileEvent,
  type TextFileState,
} from './text-file.js'

// v is the version numbered n.
const v = (n: number) => new Uint8Array([n])

// read is a whole text read at version v.
function read(text: string, v: number): TextContent {
  return { text, crlf: false, preview: null, version: new Uint8Array([v]) }
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
      { type: 'refused', read: 1 },
      { type: 'back-read', read: 1, content: theirs },
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
      { type: 'refused', read: 1 },
      { type: 'back-read', read: 1, content: same },
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
      { type: 'refused', read: 1 },
      { type: 'back-read', read: 1, content: same },
    )

    // The refusal named the expected version, so the read back is not stale.
    expect(state.back.state).toBe('idle')
    expect(state.draft?.from).toBe(same)
  })

  it('starts the next draft from the version a save wrote', () => {
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine', crlf: false, version: v(3) },
    )

    // The saved text shows, and the next draft expects its version.
    const view = viewOf(state)
    expect(view.state === 'shown' && view.text).toBe('mine')
    const next = reduceTextFile(state, { type: 'edit' })
    expect(next.draft?.from.version).toEqual(v(3))
  })

  it('keeps a landed save over a read of the version it replaced', () => {
    // The file's own handle still reads the version the save replaced.
    const saved: TextFileEvent[] = [
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine', crlf: false, version: v(3) },
      { type: 'read', content: original },
    ]
    const view = viewOf(replay(...saved))
    expect(view.state === 'shown' && view.text).toBe('mine')

    // A draft of the saved text offers a plain Save at once.
    const next = viewOf(
      replay(...saved, { type: 'edit' }, { type: 'type', text: 'more' }),
    )
    expect(next.state === 'editing' && next.presses).toEqual([
      'save',
      'discard',
    ])
  })

  it("takes another writer's text read after a landed save as theirs", () => {
    // A draft of a landed save; the first read after it holds other text.
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'saved', text: 'mine', crlf: false, version: v(3) },
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
      { type: 'refused', read: 1 },
      { type: 'back-failed', read: 1, error: 'offline' },
    )

    // Save over theirs and Read again are offered; plain Save is refused,
    // even after a read that still shows the refused version.
    const view = viewOf(state)
    expect(view.state === 'editing' && view.presses).toContain('saveover')
    expect(view.state === 'editing' && view.theirsRead?.state).toBe('failed')
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

  it('moves a draft onto the same text in other line endings', () => {
    // Another writer rewrites the draft's base with CRLF line endings.
    const crlf = { ...read('a\nb\n', 4), crlf: true }
    const state = replay(
      { type: 'read', content: read('a\nb\n', 1) },
      { type: 'edit' },
      { type: 'type', text: 'a\nbX\n' },
      { type: 'read', content: crlf },
    )

    // It is no change; a plain Save expects their version in their endings.
    const view = viewOf(state)
    expect(view.state === 'editing' && view.changed).toBeNull()
    expect(state.draft?.from).toBe(crlf)
  })

  it('says the text did not change when a refusal read back shows the same text', () => {
    // Another writer changes only the file's permissions: a new version with
    // the draft's text refuses the save.
    const chmod = read('original', 4)
    const state = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'refused', read: 1 },
      { type: 'back-read', read: 1, content: chmod },
    )

    // No change is claimed; plain Save writes again over the new version.
    const view = viewOf(state)
    expect(view.state === 'editing' && view.changed).toBeNull()
    expect(view.state === 'editing' && view.presses).toContain('save')
    expect(state.draft).toMatchObject({ from: chmod, retouched: true })
    expect(
      reduceTextFile(state, { type: 'save', over: false }).draft?.retouched,
    ).toBe(false)
  })

  it('never shows a read back taken before a save over that landed', () => {
    // The store refuses a plain save; while their version is read back, the
    // person saves over theirs and the save lands.
    const saved = replay(
      ...drafted,
      { type: 'save', over: false },
      { type: 'refused', read: 1 },
      { type: 'save', over: true },
      { type: 'saved', text: 'mine', crlf: false, version: v(4) },
    )

    // The read back, taken before the save, returns their version late; the
    // saved text stays, as does the read's failure.
    const late = reduceTextFile(saved, {
      type: 'back-read',
      read: 1,
      content: read('theirs', 3),
    })
    expect(late).toBe(saved)
    expect(late.seen).toMatchObject({ text: 'mine', version: v(4) })
    expect(
      reduceTextFile(saved, { type: 'back-failed', read: 1, error: 'gone' }),
    ).toBe(saved)

    // Nor does it land in a later refusal's read back.
    const next: TextFileEvent[] = [
      { type: 'edit' },
      { type: 'type', text: 'mine again' },
      { type: 'save', over: false },
      { type: 'refused', read: 2 },
      { type: 'back-read', read: 1, content: read('theirs', 3) },
    ]
    const again = next.reduce(reduceTextFile, saved)
    expect(again.back).toEqual({ state: 'reading', read: 2 })
    expect(again.seen?.version).toEqual(v(4))
  })
})

describe('decodeText', () => {
  const bytes = (text: string) => new TextEncoder().encode(text)

  it('keeps a byte order mark through a save', () => {
    const file = bytes('\uFEFFnotes\n')
    const form = decodeText(file, false)
    expect(form).toEqual({ text: '\uFEFFnotes\n', crlf: false, preview: null })
    expect(encodeText(form.text + 'more\n', form.crlf)).toEqual(
      bytes('\uFEFFnotes\nmore\n'),
    )
  })

  it('writes an edit of CRLF lines back with CRLF', () => {
    // The editor holds LF lines; the save writes CRLF again.
    const form = decodeText(bytes('one\r\ntwo\r\n'), false)
    expect(form).toEqual({ text: 'one\ntwo\n', crlf: true, preview: null })
    expect(encodeText(form.text.replace('two', 'TWO'), form.crlf)).toEqual(
      bytes('one\r\nTWO\r\n'),
    )
  })

  it('saves an unedited file byte for byte', () => {
    const files = [
      '',
      'no final newline',
      'lf\nlines\n',
      '\uFEFFmark\r\ncrlf\r\nno final newline',
      '\r\n\r\n',
      'wide \u{1F680} text\n',
      '\uFEFF\uFEFFtwo marks\n',
    ]
    for (const file of files) {
      const form = decodeText(bytes(file), false)
      expect(form.preview).toBeNull()
      expect(encodeText(form.text, form.crlf)).toEqual(bytes(file))
    }
  })

  it('opens no draft of a file with mixed line endings', () => {
    // The editor would write every line back in one form.
    for (const file of ['a\r\nb\n', 'a\rb', 'a\nb\r\n']) {
      expect(decodeText(bytes(file), false).preview).toBe('endings')
    }
  })
})
