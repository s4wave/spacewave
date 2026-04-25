import { useCallback, useMemo, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { mapAuthError } from '@s4wave/web/ui/credential/auth-utils.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { AccountEscalationIntent } from '@s4wave/sdk/account/account.pb.js'
import type { EntityCredential } from '@s4wave/core/session/session.pb.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { AuthUnlockWizard } from './AuthUnlockWizard.js'
import { useAccountDashboardState } from './AccountDashboardStateContext.js'
import {
  buildAccountEscalationStateResult,
  useAccountEscalationState,
} from './useAccountEscalationState.js'

// AuthCredential is the resolved signing credential.
export type AuthCredential =
  | { type: 'tracker' }
  | { type: 'password'; password: string }
  | { type: 'pem'; pemData: Uint8Array }

// buildEntityCredential converts a resolved UI auth credential into an
// EntityCredential request payload, or returns nil when tracker-backed
// unlocked signers should authorize the mutation instead.
export function buildEntityCredential(
  credential: AuthCredential,
): EntityCredential | undefined {
  if (credential.type === 'tracker') {
    return undefined
  }
  if (credential.type === 'password') {
    return { credential: { case: 'password', value: credential.password } }
  }
  return {
    credential: { case: 'pemPrivateKey', value: credential.pemData },
  }
}

export interface AuthConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel?: React.ReactNode
  // intent describes the protected account action being authorized.
  intent: AccountEscalationIntent
  // receives the resolved credential.
  onConfirm: (credential: AuthCredential) => Promise<void>
  // account enables multi-sig unlock when threshold > 0.
  account?: Resource<Account>
  // retainAfterClose keeps unlocked keypairs available until the surrounding
  // screen releases its shared retention handle.
  retainAfterClose?: boolean
}

// AuthConfirmDialog wraps the shared unlock shell and resolves protected account
// mutations through tracker-backed unlocked signers when an account resource is
// available.
export function AuthConfirmDialog({
  account,
  ...props
}: AuthConfirmDialogProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    const escalation = buildAccountEscalationStateResult(
      props.intent,
      state.accountInfo.value?.authThreshold ?? 0,
      state.authMethods.value?.authMethods ?? [],
      state.entityKeypairs.value?.keypairs ?? [],
      state.entityKeypairs.value?.unlockedCount ?? 0,
      state.accountInfo.loading ||
        state.authMethods.loading ||
        state.entityKeypairs.loading,
    )

    return (
      <AuthConfirmDialogContent
        {...props}
        account={account}
        requiredSigners={escalation.state.requirement?.requiredSigners ?? 1}
      />
    )
  }

  return <AuthConfirmDialogWithEscalation {...props} account={account} />
}

function AuthConfirmDialogWithEscalation(props: AuthConfirmDialogProps) {
  const noAccountFallback = useMemo<Resource<Account>>(
    () => ({ value: null, loading: false, error: null, retry: () => {} }),
    [],
  )
  const escalation = useAccountEscalationState(
    props.account ?? noAccountFallback,
    props.intent,
  )

  return (
    <AuthConfirmDialogContent
      {...props}
      requiredSigners={escalation.state.requirement?.requiredSigners ?? 1}
    />
  )
}

interface AuthConfirmDialogContentProps extends AuthConfirmDialogProps {
  requiredSigners: number
}

function AuthConfirmDialogContent({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = 'Confirm',
  onConfirm,
  account,
  retainAfterClose = false,
  requiredSigners,
}: AuthConfirmDialogContentProps) {
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const handleTrackerConfirm = useCallback(async () => {
    try {
      setError(null)
      await onConfirm({ type: 'tracker' })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      setError(mapAuthError(msg))
      throw err
    }
  }, [onConfirm])

  const handleFallbackConfirm = useCallback(async () => {
    setSubmitting(true)
    setError(null)
    try {
      await onConfirm({ type: 'tracker' })
      onOpenChange(false)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      setError(mapAuthError(msg))
    } finally {
      setSubmitting(false)
    }
  }, [onConfirm, onOpenChange])

  if (account) {
    return (
      <AuthUnlockWizard
        open={open}
        onClose={() => onOpenChange(false)}
        title={title}
        description={description}
        confirmLabel={confirmLabel}
        onConfirm={handleTrackerConfirm}
        threshold={requiredSigners - 1}
        account={account}
        retainAfterClose={retainAfterClose}
      />
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <p className="text-foreground-alt text-sm">
          Account escalation requires a mounted account resource.
        </p>
        {error && <p className="text-destructive text-xs">{error}</p>}

        <DialogFooter>
          <button
            onClick={() => onOpenChange(false)}
            disabled={submitting}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={() => void handleFallbackConfirm()}
            disabled={submitting}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 rounded-md border px-4 py-2 text-sm transition-all disabled:cursor-not-allowed disabled:opacity-50"
          >
            {submitting ? 'Confirming...' : confirmLabel}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
