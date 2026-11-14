// The text file viewer's draft and the view derived from it.
// UnixFSTextFileViewer.tla specifies the states and presses.

/** TextContent is one read of a text file. */
export interface TextContent {
  // text is the file as the editor holds it: a byte order mark stays as
  // U+FEFF, and lines end in LF whatever the file uses.
  text: string
  // crlf is set when every line of the file ends in CRLF; a save writes them
  // back so.
  crlf: boolean
  // preview says why the read is not the whole file as text: cut at the read
  // limit, not valid UTF-8 and decoded with replacements, or line endings the
  // editor cannot write back. A preview is never edited, and its text never
  // stands in for the file's contents.
  preview: TextPreview | null
  // version is the file version the read saw. A plain Save expects it, so the
  // store refuses the write when another writer saved since.
  version: Uint8Array
}

/** TextPreview is why a read cannot be edited. */
export type TextPreview = 'large' | 'encoding' | 'endings'

// decoder keeps a byte order mark in the text, so it saves back.
const decoder = new TextDecoder('utf-8', { ignoreBOM: true })
const strictDecoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })

/** TextForm is a file's text as the editor holds it. */
export type TextForm = Omit<TextContent, 'version'>

/**
 * decodeText decodes a file's bytes, or its first bytes when cut, into the
 * form the editor holds. encodeText of a whole, editable result returns the
 * same bytes.
 */
export function decodeText(data: Uint8Array, cut: boolean): TextForm {
  if (cut) return { text: decoder.decode(data), crlf: false, preview: 'large' }
  let text: string
  try {
    text = strictDecoder.decode(data)
  } catch {
    return { text: decoder.decode(data), crlf: false, preview: 'encoding' }
  }

  // The editor ends every line in LF, so only a file whose lines all end in
  // LF, or all in CRLF, saves back unchanged; any other CR makes a preview.
  if (!text.includes('\r')) return { text, crlf: false, preview: null }
  const lf = text.replaceAll('\r\n', '\n')
  if (lf.includes('\r') || lf.replaceAll('\n', '\r\n') !== text) {
    return { text, crlf: false, preview: 'endings' }
  }
  return { text: lf, crlf: true, preview: null }
}

/** encodeText returns the bytes a file of the editor's text holds. */
export function encodeText(
  text: string,
  crlf: boolean,
): Uint8Array<ArrayBuffer> {
  return new TextEncoder().encode(crlf ? text.replaceAll('\n', '\r\n') : text)
}

// sameVersion reports whether two versions are equal.
function sameVersion(x: Uint8Array, y: Uint8Array): boolean {
  return x.length === y.length && x.every((byte, i) => byte === y[i])
}

/**
 * TextChange is another writer's version under a draft: its text once a read
 * brought it, or null while the store's refusal is all the viewer knows.
 */
export interface TextChange {
  text: string | null
}

/**
 * changeOf returns another writer's version under the draft: a read at a
 * version the draft did not start from, or the store's refusal of a plain
 * save before a read shows what refused it.
 */
export function changeOf(
  draft: TextDraft,
  seen: TextContent | null,
): TextChange | null {
  if (seen && !sameVersion(seen.version, draft.from.version)) {
    return { text: seen.text }
  }
  return draft.refused ? { text: null } : null
}

/**
 * TextDraft is the person's edit of the file. The viewer holds it until
 * Discard edits or a save that lands; a read that runs again, fails, or
 * brings another writer's text never replaces it.
 */
export interface TextDraft {
  text: string
  // from is what the draft started from; a plain Save expects its version.
  from: TextContent
  // saving is the save in flight: a plain save, or Save over theirs.
  saving: 'save' | 'over' | null
  // error is the last save's failure.
  error: string
  // theirs shows the other writer's text in place of the draft.
  theirs: boolean
  // refused is set when the store refused a plain save on from's version.
  refused: boolean
  // retouched is set when a read after a refusal showed the draft's own text
  // at another version: another writer changed the file, such as its
  // permissions, but not its text, so a plain Save writes again.
  retouched: boolean
}

/**
 * BackRead is the viewer's read after the store refused a plain save: it
 * learns what refused it. Read again runs it once more. Each read carries the
 * number the viewer gave it, and only its own result lands: a save that lands
 * ends the read, so a result taken before that save never shows over it.
 */
export type BackRead =
  | { state: 'idle' }
  | { state: 'reading'; read: number }
  | { state: 'failed'; error: string }

/** TextFileState is what the viewer holds beside the file's read. */
export interface TextFileState {
  draft: TextDraft | null
  // seen is the latest the viewer knows: a read, or a landed save. It is
  // kept through a failed read, so a draft still knows whether another writer
  // saved.
  seen: TextContent | null
  // replaced lists the versions the last landed save replaced, until a read
  // is taken. The file's own handle can still read them; such a read is from
  // before the save.
  replaced: Uint8Array[]
  back: BackRead
}

export type TextFileEvent =
  | { type: 'read'; content: TextContent }
  | { type: 'edit' }
  | { type: 'type'; text: string }
  | { type: 'save'; over: boolean }
  | { type: 'saved'; text: string; crlf: boolean; version: Uint8Array }
  | { type: 'save-failed'; error: string }
  | { type: 'refused'; read: number }
  | { type: 'back-reading'; read: number }
  | { type: 'back-read'; read: number; content: TextContent }
  | { type: 'back-failed'; read: number; error: string }
  | { type: 'discard' }
  | { type: 'theirs'; show: boolean }

export const initialTextFileState: TextFileState = {
  draft: null,
  seen: null,
  replaced: [],
  back: { state: 'idle' },
}

// stale reports whether a read is from before what the viewer knows: at a
// version a landed save replaced, or at the version the store refused.
function stale(state: TextFileState, content: TextContent): boolean {
  const { draft } = state
  if (state.replaced.some((v) => sameVersion(v, content.version))) return true
  const refused = draft?.refused ? draft.from.version : null
  return !!refused && sameVersion(refused, content.version)
}

// awaits reports whether the viewer waits for the read back numbered read.
function awaits(state: TextFileState, read: number): boolean {
  return state.back.state === 'reading' && state.back.read === read
}

// land takes a read that is not stale. A whole read of the text the draft
// started from changes nothing the draft holds, so the draft moves onto it and
// takes the read's line endings, which the draft's LF text does not carry; a
// preview's text is not the file's, so it never moves a draft. A save in
// flight keeps the version it expected, so a refusal names that version. A
// draft the store refused that moves so was refused for a change that left
// its text alone. A read taken is past the last save, so the versions that
// save replaced stop being stale; another writer may bring one back.
function land(prev: TextFileState, content: TextContent): TextFileState {
  const state = { ...prev, replaced: [] }
  const { draft } = state
  const same = !content.preview && content.text === draft?.from.text
  if (draft && !draft.saving && same) {
    const moved = {
      ...draft,
      from: content,
      refused: false,
      retouched: draft.refused || draft.retouched,
    }
    return { ...state, seen: content, draft: moved }
  }
  return { ...state, seen: content }
}

/** reduceTextFile applies one event to the viewer's state. */
export function reduceTextFile(
  state: TextFileState,
  event: TextFileEvent,
): TextFileState {
  const draft = state.draft
  switch (event.type) {
    case 'read':
      if (stale(state, event.content)) return state
      return land(state, event.content)
    case 'edit':
      // Only a whole read, or text this viewer saved, is edited.
      if (draft || !state.seen || state.seen.preview) return state
      return {
        ...state,
        draft: {
          text: state.seen.text,
          from: state.seen,
          saving: null,
          error: '',
          theirs: false,
          refused: false,
          retouched: false,
        },
      }
    case 'type':
      if (!draft || draft.saving || draft.theirs) return state
      return { ...state, draft: { ...draft, text: event.text } }
    case 'save':
      // A plain Save never writes over a version another writer saved.
      if (!draft || draft.saving) return state
      if (!event.over && changeOf(draft, state.seen)) return state
      return {
        ...state,
        draft: {
          ...draft,
          saving: event.over ? 'over' : 'save',
          error: '',
          theirs: false,
          retouched: false,
        },
      }
    case 'saved': {
      // The file holds the saved text at the version the upload returned.
      if (!draft?.saving) return state
      const replaced = [draft.from.version]
      if (state.seen) replaced.push(state.seen.version)
      const seen: TextContent = {
        text: event.text,
        crlf: event.crlf,
        preview: null,
        version: event.version,
      }
      return { draft: null, seen, replaced, back: { state: 'idle' } }
    }
    case 'save-failed':
      if (!draft?.saving) return state
      return { ...state, draft: { ...draft, saving: null, error: event.error } }
    case 'refused':
      // The store refused a plain save on another writer's version; the
      // viewer reads theirs back, and Save over theirs is offered meanwhile.
      if (draft?.saving !== 'save') return state
      return {
        ...state,
        draft: { ...draft, saving: null, refused: true },
        back: { state: 'reading', read: event.read },
      }
    case 'back-reading':
      return { ...state, back: { state: 'reading', read: event.read } }
    case 'back-read':
      // Only the read the viewer waits for lands, and a read at the version
      // the store refused is not it.
      if (!awaits(state, event.read)) return state
      if (stale(state, event.content)) {
        const error = 'the file still reads as the version your save expected'
        return { ...state, back: { state: 'failed', error } }
      }
      return { ...land(state, event.content), back: { state: 'idle' } }
    case 'back-failed':
      if (!awaits(state, event.read)) return state
      return { ...state, back: { state: 'failed', error: event.error } }
    case 'discard':
      if (!draft || draft.saving) return state
      return { ...state, draft: null }
    case 'theirs':
      if (!draft || draft.saving) return state
      return { ...state, draft: { ...draft, theirs: event.show } }
  }
}

/** TextFilePress names one press the viewer offers. */
export type TextFilePress =
  | 'edit'
  | 'save'
  | 'saveover'
  | 'saving'
  | 'discard'
  | 'theirs'
  | 'mine'

/** TextFileRead is the file's read as the viewer's Resource reports it. */
export interface TextFileRead {
  value: TextContent | null
  loading: boolean
  error: Error | null
}

/**
 * TextFileView is what the viewer draws. Without a draft it follows the read:
 * waiting, failed or shown. With a draft it is editing whatever the read does,
 * with the read's failure and another writer's text as notices beside it.
 */
export type TextFileView =
  | { state: 'waiting' }
  | { state: 'failed'; error: Error }
  | {
      state: 'shown'
      text: string
      preview: TextPreview | null
      rereading: boolean
      presses: TextFilePress[]
    }
  | {
      state: 'editing'
      draft: TextDraft
      // changed is another writer's version under the draft.
      changed: TextChange | null
      readError: Error | null
      // theirsRead is the read of the version the store refused a plain save
      // on, while no read has shown it.
      theirsRead: BackRead | null
      presses: TextFilePress[]
    }

/**
 * deriveTextFileView derives the view from the read and the viewer's state.
 * writable is false where the file cannot be written, such as a commit's
 * files; Edit then never shows.
 */
export function deriveTextFileView(
  read: TextFileRead,
  state: TextFileState,
  writable: boolean,
): TextFileView {
  // A draft is drawn whatever the read does; only a read after the draft
  // began can bring another writer's text.
  const { draft, back } = state
  if (draft) {
    const changed = changeOf(draft, state.seen)
    return {
      state: 'editing',
      draft,
      changed,
      readError: read.error,
      theirsRead: changed && changed.text === null ? back : null,
      presses: draftPresses(draft, changed),
    }
  }

  // Without a draft the read decides: failed, waiting, or shown. The latest
  // read is the reducer's, newer than the Resource's after a save.
  const shown = state.seen ?? read.value
  if (read.error) return { state: 'failed', error: read.error }
  if (!shown) return { state: 'waiting' }

  // Edit needs a writable file read whole and not being read again.
  const rereading = read.loading
  const editable = writable && !shown.preview && !rereading
  return {
    state: 'shown',
    text: shown.text,
    preview: shown.preview,
    rereading,
    presses: editable ? ['edit'] : [],
  }
}

// draftPresses lists a draft's presses: a running save holds its press alone;
// a changed file offers Save over theirs in place of Save.
function draftPresses(
  draft: TextDraft,
  changed: TextChange | null,
): TextFilePress[] {
  // A running save holds its press alone.
  if (draft.saving) return ['saving']

  // Otherwise a save when dirty: over theirs when changed, else Save. Then
  // the theirs toggle once their text is read, and Discard edits.
  const presses: TextFilePress[] = []
  if (draft.text !== draft.from.text)
    presses.push(changed ? 'saveover' : 'save')
  if (changed?.text != null) presses.push(draft.theirs ? 'mine' : 'theirs')
  presses.push('discard')
  return presses
}
