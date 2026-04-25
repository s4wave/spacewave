import { useCallback, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'

export interface DeleteSpaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceName: string
  onConfirm: () => Promise<void>
}

// DeleteSpaceDialog prompts the user to type the space name to confirm deletion.
export function DeleteSpaceDialog({
  open,
  onOpenChange,
  spaceName,
  onConfirm,
}: DeleteSpaceDialogProps) {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()
  const [confirmText, setConfirmText] = useState('')

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setConfirmText('')
        setError(undefined)
        setSubmitting(false)
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const handleDelete = useCallback(async () => {
    setSubmitting(true)
    setError(undefined)
    try {
      await onConfirm()
      handleOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setSubmitting(false)
    }
  }, [onConfirm, handleOpenChange])

  const inputClass = cn(
    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm outline-none transition-colors',
    'focus:border-destructive/50',
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Space</DialogTitle>
          <DialogDescription>
            This will permanently delete &ldquo;{spaceName}&rdquo; and all its
            data. Type the space name to confirm.
          </DialogDescription>
        </DialogHeader>

        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            Space name
          </label>
          <input
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            placeholder={spaceName}
            className={inputClass}
            onKeyDown={(e) => {
              if (
                e.key === 'Enter' &&
                confirmText === spaceName &&
                !submitting
              ) {
                void handleDelete()
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
            onClick={() => void handleDelete()}
            disabled={confirmText !== spaceName || submitting}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {submitting ? 'Deleting...' : 'Delete Space'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
