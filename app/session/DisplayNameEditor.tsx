import { useCallback, useEffect, useReducer } from 'react'
import { LuSave, LuX } from 'react-icons/lu'

import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { Input } from '@s4wave/web/ui/input.js'

// DisplayNameEditState is the lifecycle of one display name edit. Every state
// past viewing carries the draft, so a failed save keeps what was typed.
export type DisplayNameEditState =
  | { kind: 'viewing' }
  | { kind: 'editing'; draft: string }
  | { kind: 'saving'; draft: string }
  | { kind: 'failed'; draft: string; error: string }

// DisplayNameEditEvent moves a display name edit between its states.
type DisplayNameEditEvent =
  | { type: 'start'; current: string }
  | { type: 'edit'; draft: string }
  | { type: 'cancel' }
  | { type: 'save' }
  | { type: 'failed'; error: string }
  | { type: 'watched'; current: string }

// normalizeDisplayName collapses whitespace as SetDisplayName stores it.
export function normalizeDisplayName(name: string): string {
  return name.trim().split(/\s+/).filter(Boolean).join(' ')
}

// displayNameEditReducer applies one event. A save stays pending until the
// watched name shows the written value.
export function displayNameEditReducer(
  state: DisplayNameEditState,
  event: DisplayNameEditEvent,
): DisplayNameEditState {
  switch (event.type) {
    case 'start':
      return state.kind === 'viewing'
        ? { kind: 'editing', draft: event.current }
        : state
    case 'edit':
      return state.kind === 'editing' || state.kind === 'failed'
        ? { kind: 'editing', draft: event.draft }
        : state
    case 'cancel':
      return state.kind === 'saving' ? state : { kind: 'viewing' }
    case 'save':
      return state.kind === 'editing' || state.kind === 'failed'
        ? { kind: 'saving', draft: state.draft }
        : state
    case 'failed':
      return state.kind === 'saving'
        ? { kind: 'failed', draft: state.draft, error: event.error }
        : state
    case 'watched':
      return state.kind === 'saving' &&
        normalizeDisplayName(state.draft) === event.current
        ? { kind: 'viewing' }
        : state
  }
}

// DisplayNameEditor is the state and actions of one display name edit.
export interface DisplayNameEditor {
  state: DisplayNameEditState
  // changed reports that the draft differs from the saved name.
  changed: boolean
  start: () => void
  edit: (draft: string) => void
  cancel: () => void
  save: () => void
}

// useDisplayNameEditor edits a local account's display name through setName,
// which writes it with SetDisplayName. currentDisplayName is the watched name.
export function useDisplayNameEditor(
  currentDisplayName: string,
  setName: (name: string) => Promise<void>,
): DisplayNameEditor {
  const [state, dispatch] = useReducer(displayNameEditReducer, {
    kind: 'viewing',
  })
  const draft = state.kind === 'viewing' ? '' : state.draft
  const changed =
    state.kind !== 'viewing' &&
    normalizeDisplayName(draft) !== currentDisplayName

  // Finish a pending save once the watched name shows it.
  useEffect(() => {
    dispatch({ type: 'watched', current: currentDisplayName })
  }, [currentDisplayName])

  const start = useCallback(
    () => dispatch({ type: 'start', current: currentDisplayName }),
    [currentDisplayName],
  )
  const edit = useCallback(
    (next: string) => dispatch({ type: 'edit', draft: next }),
    [],
  )
  const cancel = useCallback(() => dispatch({ type: 'cancel' }), [])
  const save = useCallback(() => {
    if (!changed || state.kind === 'saving') return
    const name = normalizeDisplayName(draft)
    dispatch({ type: 'save' })
    setName(name).catch((err: unknown) =>
      dispatch({
        type: 'failed',
        error: err instanceof Error ? err.message : 'Could not save the name',
      }),
    )
  }, [changed, draft, setName, state.kind])

  return { state, changed, start, edit, cancel, save }
}

// focusField moves focus into the name field when a person opens it.
function focusField(el: HTMLInputElement | null) {
  el?.focus()
}

// DisplayNameForm renders the name input with its save and cancel actions for
// an editor past the viewing state.
export function DisplayNameForm(props: {
  editor: DisplayNameEditor
  inputId?: string
  label: string
}) {
  const { editor } = props
  const state = editor.state
  if (state.kind === 'viewing') return null
  const saving = state.kind === 'saving'
  return (
    <div className="min-w-0 flex-1">
      <div className="flex items-center gap-2">
        <Input
          id={props.inputId}
          value={state.draft}
          ref={focusField}
          disabled={saving}
          onChange={(e) => editor.edit(e.target.value)}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            e.stopPropagation()
            if (e.nativeEvent.isComposing) return
            if (e.key === 'Enter') {
              e.preventDefault()
              editor.save()
            }
            if (e.key === 'Escape') {
              e.preventDefault()
              editor.cancel()
            }
          }}
          placeholder="Name this local account"
          aria-label={props.label}
          variant="displayName"
        />
        <DashboardButton
          icon={<LuSave aria-hidden className="size-3" />}
          onClick={(e) => {
            e.stopPropagation()
            editor.save()
          }}
          disabled={!editor.changed || saving}
        >
          {saving ? 'Saving…' : 'Save'}
        </DashboardButton>
        <DashboardButton
          icon={<LuX aria-hidden className="size-3" />}
          onClick={(e) => {
            e.stopPropagation()
            editor.cancel()
          }}
          disabled={saving}
        >
          Cancel
        </DashboardButton>
      </div>
      {state.kind === 'failed' && (
        <div className="text-destructive mt-1 text-xs" role="alert">
          {state.error}. Press Save to try again.
        </div>
      )}
    </div>
  )
}
