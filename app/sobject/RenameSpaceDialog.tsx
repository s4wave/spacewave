import { useCallback, useId, useReducer } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'

export interface RenameSpaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceName: string
  onConfirm: (newName: string) => Promise<void>
}

interface RenameState {
  submitting: boolean
  error?: string
  source: string
  value: string
}

type RenameAction =
  | { type: 'reset'; value: string }
  | { type: 'set-value'; source: string; value: string }
  | { type: 'submit' }
  | { type: 'fail'; error: string }

function renameReducer(state: RenameState, action: RenameAction): RenameState {
  switch (action.type) {
    case 'reset':
      return { submitting: false, source: action.value, value: action.value }
    case 'set-value':
      return { ...state, source: action.source, value: action.value }
    case 'submit':
      return { ...state, submitting: true, error: undefined }
    case 'fail':
      return { ...state, submitting: false, error: action.error }
  }
}

// RenameSpaceDialog prompts the user for a new display name for the space.
export function RenameSpaceDialog({
  open,
  onOpenChange,
  spaceName,
  onConfirm,
}: RenameSpaceDialogProps) {
  const [state, dispatch] = useReducer(renameReducer, {
    submitting: false,
    source: spaceName,
    value: spaceName,
  })
  const spaceNameId = useId()
  const value = state.source === spaceName ? state.value : spaceName

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        dispatch({ type: 'reset', value: spaceName })
      }
      onOpenChange(next)
    },
    [onOpenChange, spaceName],
  )

  const trimmed = value.trim()
  const canSubmit =
    trimmed.length > 0 && trimmed !== spaceName && !state.submitting
  const handleInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleSubmit = useCallback(async () => {
    if (!canSubmit) return
    dispatch({ type: 'submit' })
    try {
      await onConfirm(trimmed)
      handleOpenChange(false)
    } catch (err) {
      dispatch({
        type: 'fail',
        error: err instanceof Error ? err.message : 'Rename failed',
      })
    }
  }, [canSubmit, onConfirm, trimmed, handleOpenChange])

  const inputClass = cn(
    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm outline-none transition-colors',
    'focus:border-brand/50',
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Rename Space</DialogTitle>
          <DialogDescription>
            Enter a new display name for &ldquo;{spaceName}&rdquo;.
          </DialogDescription>
        </DialogHeader>

        <div>
          <label
            className="text-foreground-alt mb-1.5 block text-xs select-none"
            htmlFor={spaceNameId}
          >
            Space name
          </label>
          <input
            id={spaceNameId}
            ref={handleInputRef}
            value={value}
            onChange={(e) =>
              dispatch({
                type: 'set-value',
                source: spaceName,
                value: e.target.value,
              })
            }
            placeholder={spaceName}
            className={inputClass}
            aria-label="New space name"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canSubmit) {
                e.preventDefault()
                void handleSubmit()
              }
            }}
          />
        </div>

        {state.error && (
          <p className="text-destructive text-xs">{state.error}</p>
        )}

        <DialogFooter>
          <button
            onClick={() => handleOpenChange(false)}
            disabled={state.submitting}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={() => void handleSubmit()}
            disabled={!canSubmit}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 text-brand hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {state.submitting ? 'Saving…' : 'Save'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
