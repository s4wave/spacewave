// The text file viewer's draft and the view derived from it.
// UnixFSTextFileViewer.tla specifies the states and presses.

/** TextContent is one read of a text file. */
export interface TextContent {
  text: string
  // complete is false for a preview cut at the read limit, or text that is
  // not valid UTF-8; a preview is never edited.
  complete: boolean
}

/**
 * TextDraft is the person's edit of the file. The viewer holds it until
 * Discard edits or a save that lands; a read that runs again, fails, or
 * brings another writer's text never replaces it.
 */
export interface TextDraft {
  text: string
  // base is the file text the draft started from.
  base: string
  // from is the read the draft started from; base differs from its text when
  // the draft started from a landed save no read has brought yet.
  from: TextContent | null
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
  // seen is the last read that landed, kept through a failed read so a draft
  // still knows whether another writer saved.
  seen: TextContent | null
  // saved is the text a landed save wrote, shown until a read after it lands.
  saved: { text: string; over: TextContent | null } | null
}

export type TextFileEvent =
  | { type: 'read'; content: TextContent }
  | { type: 'edit'; text: string }
  | { type: 'type'; text: string }
  | { type: 'save'; over: boolean }
  | { type: 'saved'; text: string }
  | { type: 'save-failed'; error: string }
  | { type: 'discard' }
  | { type: 'theirs'; show: boolean }

export const initialTextFileState: TextFileState = {
  draft: null,
  seen: null,
  saved: null,
}

/** reduceTextFile applies one event to the viewer's state. */
export function reduceTextFile(
  state: TextFileState,
  event: TextFileEvent,
): TextFileState {
  const draft = state.draft
  switch (event.type) {
    case 'read':
      return { ...state, seen: event.content }
    case 'edit':
      if (draft) return state
      return {
        ...state,
        draft: {
          text: event.text,
          base: event.text,
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
      if (!draft || draft.saving) return state
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
      if (!draft?.saving) return state
      return {
        ...state,
        draft: null,
        saved: { text: event.text, over: state.seen },
      }
    case 'save-failed':
      if (!draft?.saving) return state
      return { ...state, draft: { ...draft, saving: null, error: event.error } }
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
      complete: boolean
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
  const latest = read.value ?? state.seen
  if (draft) {
    const changed =
      latest &&
      latest !== draft.from &&
      latest.text !== draft.base &&
      latest.text !== draft.text
        ? latest.text
        : null
    return {
      state: 'editing',
      draft,
      changed,
      readError: read.error,
      presses: draftPresses(draft, changed !== null),
    }
  }

  // Without a draft the read decides: failed, waiting, or shown.
  if (read.error) return { state: 'failed', error: read.error }
  if (!read.value) return { state: 'waiting' }

  // A landed save shows until a read after it lands; Edit needs a writable
  // file read whole and not being read again.
  const saved = state.saved
  const text = saved && saved.over === state.seen ? saved.text : read.value.text
  const rereading = read.loading
  const editable = writable && read.value.complete && !rereading
  return {
    state: 'shown',
    text,
    complete: read.value.complete,
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
  if (draft.text !== draft.base) presses.push(changed ? 'saveover' : 'save')
  if (changed) presses.push(draft.theirs ? 'mine' : 'theirs')
  presses.push('discard')
  return presses
}
