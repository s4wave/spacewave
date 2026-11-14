// The text file viewer's draft and the view derived from it.
// UnixFSTextFileViewer.tla specifies the states and presses.

/** TextContent is one read of a text file. */
export interface TextContent {
  text: string
  // preview says why the read is not the whole file as text: cut at the read
  // limit, or not valid UTF-8 and decoded with replacements. A preview is
  // never edited.
  preview: TextPreview | null
  // version is the file version the read saw. A plain Save expects it, so the
  // store refuses the write when another writer saved since.
  version: Uint8Array
}

/** TextPreview is why a read cannot be edited. */
export type TextPreview = 'large' | 'encoding'

// sameVersion reports whether two reads saw the same file version.
function sameVersion(a: TextContent, b: TextContent): boolean {
  const x = a.version
  const y = b.version
  return x.length === y.length && x.every((byte, i) => byte === y[i])
}

/**
 * theirText returns the text another writer saved: the latest read, when it
 * saw a version other than the one the draft started from; otherwise null.
 */
export function theirText(
  draft: TextDraft,
  latest: TextContent | null,
): string | null {
  if (!latest || sameVersion(latest, draft.from)) return null
  return latest.text
}

/**
 * TextDraft is the person's edit of the file. The viewer holds it until
 * Discard edits or a save that lands; a read that runs again, fails, or
 * brings another writer's text never replaces it.
 */
export interface TextDraft {
  text: string
  // from is the read the draft started from; a plain Save expects its
  // version.
  from: TextContent
  // saving is the save in flight: a plain save, or Save over theirs.
  saving: 'save' | 'over' | null
  // error is the last save's failure.
  error: string
  // theirs shows the other writer's text in place of the draft.
  theirs: boolean
}

/** TextFileState is what the viewer holds beside the file's read. */
export interface TextFileState {
  draft: TextDraft | null
  // seen is the latest read: the viewer's own read, or the read after a
  // save. It is kept through a failed read, so a draft still knows whether
  // another writer saved.
  seen: TextContent | null
}

export type TextFileEvent =
  | { type: 'read'; content: TextContent }
  | { type: 'edit' }
  | { type: 'type'; text: string }
  | { type: 'save'; over: boolean }
  | { type: 'saved'; content: TextContent | null }
  | { type: 'save-failed'; error: string }
  | { type: 'refused'; content: TextContent }
  | { type: 'discard' }
  | { type: 'theirs'; show: boolean }

export const initialTextFileState: TextFileState = { draft: null, seen: null }

/** reduceTextFile applies one event to the viewer's state. */
export function reduceTextFile(
  state: TextFileState,
  event: TextFileEvent,
): TextFileState {
  const draft = state.draft
  switch (event.type) {
    case 'read':
      // A new version with the text the draft started from changes nothing
      // the draft holds, so the draft moves onto it.
      if (draft && event.content.text === draft.from.text) {
        return { seen: event.content, draft: { ...draft, from: event.content } }
      }
      return { ...state, seen: event.content }
    case 'edit':
      if (draft || !state.seen) return state
      return {
        ...state,
        draft: {
          text: state.seen.text,
          from: state.seen,
          saving: null,
          error: '',
          theirs: false,
        },
      }
    case 'type':
      if (!draft || draft.saving || draft.theirs) return state
      return { ...state, draft: { ...draft, text: event.text } }
    case 'save':
      // A plain Save never writes over a version another writer saved.
      if (!draft || draft.saving) return state
      if (!event.over && theirText(draft, state.seen) !== null) return state
      return {
        ...state,
        draft: {
          ...draft,
          saving: event.over ? 'over' : 'save',
          error: '',
          theirs: false,
        },
      }
    case 'saved':
      // The read after the write shows the saved file and its new version;
      // without it the viewer reads the file again.
      if (!draft?.saving) return state
      return { draft: null, seen: event.content }
    case 'save-failed':
      if (!draft?.saving) return state
      return { ...state, draft: { ...draft, saving: null, error: event.error } }
    case 'refused':
      // The store refused a plain save on another writer's version; the read
      // after it lands as a read does, so the draft shows the changed state.
      if (draft?.saving !== 'save') return state
      return reduceTextFile(
        { ...state, draft: { ...draft, saving: null } },
        { type: 'read', content: event.content },
      )
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
      // changed is the other writer's text when a read brought a version the
      // draft did not start from.
      changed: string | null
      readError: Error | null
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
  const { draft } = state
  if (draft) {
    const changed = theirText(draft, state.seen)
    return {
      state: 'editing',
      draft,
      changed,
      readError: read.error,
      presses: draftPresses(draft, changed !== null),
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
function draftPresses(draft: TextDraft, changed: boolean): TextFilePress[] {
  // A running save holds its press alone.
  if (draft.saving) return ['saving']

  // Otherwise Save or Save over theirs when dirty, the theirs toggle when
  // changed, and Discard edits.
  const presses: TextFilePress[] = []
  if (draft.text !== draft.from.text) {
    presses.push(changed ? 'saveover' : 'save')
  }
  if (changed) presses.push(draft.theirs ? 'mine' : 'theirs')
  presses.push('discard')
  return presses
}
