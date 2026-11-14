// The text file viewer's draft and the view derived from it.
// UnixFSTextFileViewer.tla specifies the states and presses.

/** TextContent is one read of a text file. */
export interface TextContent {
  text: string
  // preview says why the read is not the whole file as text: cut at the read
  // limit, or not valid UTF-8 and decoded with replacements. A preview is
  // never edited, and its text never stands in for the file's contents.
  preview: TextPreview | null
  // version is the file version the read saw. A plain Save expects it, so the
  // store refuses the write when another writer saved since.
  version: Uint8Array
}

/** TextPreview is why a read cannot be edited. */
export type TextPreview = 'large' | 'encoding'

/**
 * SavedText is a save that landed before any read showed its version. The
 * upload result carries no version, so the viewer shows the text it wrote and
 * waits for a read; a read at a version the save replaced comes from before
 * the save and is not that read.
 */
export interface SavedText {
  text: string
  preview: null
  version: null
  // replaced lists the versions the viewer knew before the save.
  replaced: Uint8Array[]
}

/** TextBase is what the viewer knows is on disk: a read, or a landed save. */
export type TextBase = TextContent | SavedText

// sameVersion reports whether two versions are equal.
function sameVersion(x: Uint8Array, y: Uint8Array): boolean {
  return x.length === y.length && x.every((byte, i) => byte === y[i])
}

// versionsOf lists the versions a base stands for.
function versionsOf(base: TextBase | null | undefined): Uint8Array[] {
  if (!base) return []
  return base.version ? [base.version] : base.replaced
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
  seen: TextBase | null,
): TextChange | null {
  if (seen?.version) {
    const from = draft.from.version
    if (!from || !sameVersion(seen.version, from)) return { text: seen.text }
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
  // from is what the draft started from; a plain Save expects its version,
  // and waits for a read when it is a save whose version no read showed.
  from: TextBase
  // saving is the save in flight: a plain save, or Save over theirs.
  saving: 'save' | 'over' | null
  // error is the last save's failure.
  error: string
  // theirs shows the other writer's text in place of the draft.
  theirs: boolean
  // refused is set when the store refused a plain save on from's version.
  refused: boolean
}

/**
 * BackRead is the viewer's read after a save: it learns a landed save's
 * version or what refused a plain save. Read again runs it once more.
 */
export type BackRead =
  | { state: 'idle' }
  | { state: 'reading' }
  | { state: 'failed'; error: string }

/** TextFileState is what the viewer holds beside the file's read. */
export interface TextFileState {
  draft: TextDraft | null
  // seen is the latest the viewer knows: a read, or a landed save. It is
  // kept through a failed read, so a draft still knows whether another writer
  // saved.
  seen: TextBase | null
  back: BackRead
}

export type TextFileEvent =
  | { type: 'read'; content: TextContent }
  | { type: 'edit' }
  | { type: 'type'; text: string }
  | { type: 'save'; over: boolean }
  | { type: 'saved'; text: string }
  | { type: 'save-failed'; error: string }
  | { type: 'refused' }
  | { type: 'back-reading' }
  | { type: 'back-read'; content: TextContent }
  | { type: 'back-failed'; error: string }
  | { type: 'discard' }
  | { type: 'theirs'; show: boolean }

export const initialTextFileState: TextFileState = {
  draft: null,
  seen: null,
  back: { state: 'idle' },
}

// stale reports whether a read is from before what the viewer knows: at a
// version a landed save replaced, or at the version the store refused.
function stale(state: TextFileState, content: TextContent): boolean {
  const { draft } = state
  const before = (base: TextBase | null | undefined) =>
    base?.version === null &&
    base.replaced.some((v) => sameVersion(v, content.version))
  if (before(state.seen) || before(draft?.from)) return true
  const refused = draft?.refused ? draft.from.version : null
  return !!refused && sameVersion(refused, content.version)
}

// land takes a read that is not stale. A whole read with the text the draft
// started from changes nothing the draft holds, so the draft moves onto it; a
// preview's text is not the file's, so it never moves a draft. A save in
// flight keeps the version it expected, so a refusal names that version.
function land(state: TextFileState, content: TextContent): TextFileState {
  const { draft } = state
  const whole = !content.preview
  if (draft && !draft.saving && whole && content.text === draft.from.text) {
    const moved = { ...draft, from: content, refused: false }
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
        },
      }
    case 'type':
      if (!draft || draft.saving || draft.theirs) return state
      return { ...state, draft: { ...draft, text: event.text } }
    case 'save':
      // A plain Save never writes over a version another writer saved, and
      // waits for the version of a save no read has shown.
      if (!draft || draft.saving) return state
      if (!event.over) {
        if (changeOf(draft, state.seen) || !draft.from.version) return state
      }
      return {
        ...state,
        draft: {
          ...draft,
          saving: event.over ? 'over' : 'save',
          error: '',
          theirs: false,
        },
      }
    case 'saved': {
      // The file holds the saved text at a version no read showed yet; the
      // viewer reads it back.
      if (!draft?.saving) return state
      const replaced = [...versionsOf(state.seen), ...versionsOf(draft.from)]
      const seen: SavedText = {
        text: event.text,
        preview: null,
        version: null,
        replaced,
      }
      return { draft: null, seen, back: { state: 'reading' } }
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
        back: { state: 'reading' },
      }
    case 'back-reading':
      return { ...state, back: { state: 'reading' } }
    case 'back-read':
      // A read from before the save is not the read the viewer waits for.
      if (stale(state, event.content)) {
        const error = 'the file still reads as it was before the save'
        return { ...state, back: { state: 'failed', error } }
      }
      return { ...land(state, event.content), back: { state: 'idle' } }
    case 'back-failed':
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
  | 'savewait'
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
 * TextFileAwait is a read the view waits for: the version of a landed save,
 * or the other writer's version the store refused a plain save on.
 */
export interface TextFileAwait {
  what: 'saved' | 'theirs'
  back: BackRead
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
      awaits: TextFileAwait | null
      presses: TextFilePress[]
    }
  | {
      state: 'editing'
      draft: TextDraft
      // changed is another writer's version under the draft.
      changed: TextChange | null
      readError: Error | null
      awaits: TextFileAwait | null
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
    let awaits: TextFileAwait | null = null
    if (changed && changed.text === null) awaits = { what: 'theirs', back }
    if (!changed && !draft.from.version) awaits = { what: 'saved', back }
    return {
      state: 'editing',
      draft,
      changed,
      readError: read.error,
      awaits,
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
    awaits: shown.version ? null : { what: 'saved', back },
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

  // Otherwise a save when dirty: over theirs when changed, waiting while the
  // saved version is unread, else Save. Then the theirs toggle once their
  // text is read, and Discard edits.
  const presses: TextFilePress[] = []
  if (draft.text !== draft.from.text) {
    if (changed) presses.push('saveover')
    else presses.push(draft.from.version ? 'save' : 'savewait')
  }
  if (changed?.text != null) presses.push(draft.theirs ? 'mine' : 'theirs')
  presses.push('discard')
  return presses
}
