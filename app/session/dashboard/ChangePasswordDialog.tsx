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
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

export interface ChangePasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  account: Resource<Account>
}

// ChangePasswordDialog prompts for current and new password, then calls ChangePassword RPC.
export function ChangePasswordDialog({
  open,
  onOpenChange,
  account,
}: ChangePasswordDialogProps) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const passwordsMatch = newPassword === confirmPassword
  const canSubmit =
    !!currentPassword &&
    !!newPassword &&
    !!confirmPassword &&
    passwordsMatch &&
    !submitting

  const handleSubmit = useCallback(async () => {
    if (!canSubmit || !account.value) return
    setSubmitting(true)
    setError(null)
    try {
      await account.value.changePassword({
        oldPassword: currentPassword,
        newPassword,
      })
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      onOpenChange(false)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      if (msg.includes('unknown_keypair')) {
        setError('Incorrect current password. Please try again.')
      } else {
        setError(msg)
      }
    } finally {
      setSubmitting(false)
    }
  }, [canSubmit, account.value, currentPassword, newPassword, onOpenChange])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setCurrentPassword('')
        setNewPassword('')
        setConfirmPassword('')
        setError(null)
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const inputClass = cn(
    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm outline-none transition-colors',
    'focus:border-brand/50',
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change password</DialogTitle>
          <DialogDescription>
            Enter your current password and choose a new one.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div>
            <label className="text-foreground-alt mb-1.5 block text-xs select-none">
              Current password
            </label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="Enter current password"
              className={inputClass}
            />
          </div>
          <div>
            <label className="text-foreground-alt mb-1.5 block text-xs select-none">
              New password
            </label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="Enter new password"
              className={inputClass}
            />
          </div>
          <div>
            <label className="text-foreground-alt mb-1.5 block text-xs select-none">
              Confirm new password
            </label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Confirm new password"
              onKeyDown={(e) => {
                if (e.key === 'Enter') void handleSubmit()
              }}
              className={inputClass}
            />
            {confirmPassword && !passwordsMatch && (
              <p className="text-destructive mt-1 text-xs">
                Passwords do not match.
              </p>
            )}
          </div>
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
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {submitting ? 'Changing...' : 'Change password'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
