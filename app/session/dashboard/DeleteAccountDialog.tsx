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

import { TypedConfirmField } from './TypedConfirmField.js'

export interface DeleteAccountDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isCloud: boolean
  onConfirm: () => Promise<void>
}

type Step = 'warning' | 'confirm' | 'final'

// DeleteAccountDialog is a multi-step confirmation dialog for account deletion.
export function DeleteAccountDialog({
  open,
  onOpenChange,
  isCloud,
  onConfirm,
}: DeleteAccountDialogProps) {
  const [step, setStep] = useState<Step>('warning')
  const [typedName, setTypedName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setStep('warning')
        setTypedName('')
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

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isCloud ? 'Delete Account' : 'Delete Local Data'}
          </DialogTitle>
          {step === 'warning' && (
            <DialogDescription>
              {isCloud
                ? 'Deleting your cloud account requires extra verification. You will confirm by email code or from account.spacewave.app, billing will finalize immediately, and a 24-hour undo window will begin.'
                : 'This will permanently delete your local account and all its data.'}
            </DialogDescription>
          )}
        </DialogHeader>

        {step === 'warning' && (
          <DialogFooter>
            <button
              type="button"
              onClick={() => handleOpenChange(false)}
              className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => setStep('confirm')}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
              )}
            >
              Review deletion
            </button>
          </DialogFooter>
        )}

        {step === 'confirm' && (
          <>
            <TypedConfirmField
              label={
                <>
                  Type{' '}
                  <span className="text-destructive font-medium">DELETE</span>{' '}
                  to confirm
                </>
              }
              value={typedName}
              onChange={setTypedName}
              placeholder="DELETE"
              canSubmit={typedName === 'DELETE'}
              onSubmit={() => setStep('final')}
            />
            <DialogFooter>
              <button
                type="button"
                onClick={() => {
                  setStep('warning')
                  setTypedName('')
                }}
                className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
              >
                Back
              </button>
              <button
                type="button"
                disabled={typedName !== 'DELETE'}
                onClick={() => setStep('final')}
                className={cn(
                  'rounded-md border px-4 py-2 text-sm transition-all',
                  'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                Confirm
              </button>
            </DialogFooter>
          </>
        )}

        {step === 'final' && (
          <>
            <p className="text-destructive text-sm">
              {isCloud
                ? 'Continue to the delete confirmation screen to send the email code and finalize the 24-hour delete countdown.'
                : 'This action cannot be undone. All local data will be permanently deleted.'}
            </p>

            {error && <p className="text-destructive text-xs">{error}</p>}

            <DialogFooter>
              <button
                type="button"
                onClick={() => setStep('confirm')}
                disabled={submitting}
                className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
              >
                Back
              </button>
              <button
                type="button"
                onClick={() => void handleDelete()}
                disabled={submitting}
                className={cn(
                  'rounded-md border px-4 py-2 text-sm transition-all',
                  'border-destructive bg-destructive/20 text-destructive hover:bg-destructive/30',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                {submitting
                  ? 'Deleting…'
                  : isCloud
                    ? 'Continue to Delete Flow'
                    : 'Delete Everything'}
              </button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
