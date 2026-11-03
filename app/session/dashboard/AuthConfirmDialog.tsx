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

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  layout as themeLayout,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

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
        <p sx={stylexStyles.rule1}>
          Account escalation requires a mounted account resource.
        </p>
        {error && <p sx={stylexStyles.rule2}>{error}</p>}

        <DialogFooter>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
            sx={stylexStyles.rule3}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleFallbackConfirm()}
            disabled={submitting}
            sx={stylexStyles.rule4}
          >
            {submitting ? 'Confirming…' : confirmLabel}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
