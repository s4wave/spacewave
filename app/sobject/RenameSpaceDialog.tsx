import { useCallback, useEffect, useState } from 'react'

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

// RenameSpaceDialog prompts the user for a new display name for the space.
export function RenameSpaceDialog({
  open,
  onOpenChange,
  spaceName,
  onConfirm,
}: RenameSpaceDialogProps) {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()
  const [value, setValue] = useState(spaceName)

  useEffect(() => {
    if (open) {
      setValue(spaceName)
      setError(undefined)
      setSubmitting(false)
    }
  }, [open, spaceName])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setError(undefined)
        setSubmitting(false)
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const trimmed = value.trim()
  const canSubmit = trimmed.length > 0 && trimmed !== spaceName && !submitting

  const handleSubmit = useCallback(async () => {
    if (!canSubmit) return
    setSubmitting(true)
    setError(undefined)
    try {
      await onConfirm(trimmed)
      handleOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Rename failed')
      setSubmitting(false)
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
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            Space name
          </label>
          <input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
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

        {error && <p className="text-destructive text-xs">{error}</p>}

        <DialogFooter>
          <button
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
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
            {submitting ? 'Saving...' : 'Save'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
