import { useCallback, useEffect, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { FcGoogle } from '@s4wave/web/images/icons.js'
import {
  LuFingerprint,
  LuGithub,
  LuLock,
  LuLockOpen,
} from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { EntityKeypairState } from '@s4wave/sdk/account/account.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'

import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import {
  mapAuthError,
  methodLabel,
  truncatePeerId,
} from '@s4wave/web/ui/credential/auth-utils.js'

import {
  recoverPasskeyEntityPem,
  recoverSSOEntityPem,
  resolveRecoveredEntityPem,
  type RecoveredEntityPem,
} from './accountEscalationUnlock.js'
import { startSSOPopupFlow, type SSOPopupFlow } from './sso-popup.js'
import { useAccountDashboardState } from './AccountDashboardStateContext.js'
import { useEntityKeypairs } from './useEntityKeypairs.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule3: {
    borderRadius: '3.40282e38px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule7: {
    width: 'var(--progress-width)',
    transition: 'width 300ms',
    height: '100%',
    backgroundColor: themeColors['--color-brand'],
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule9: {
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
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule10: {
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
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule11: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: { minWidth: 0 },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule15: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule17: { display: 'flex', justifyContent: 'flex-end' },
  rule18: {
    flexShrink: 0,
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
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule19: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule20: { minWidth: 0, flex: 1 },
  rule21: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule22: {
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule23: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule24: {
    display: 'inline-flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
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
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  credentialSpacing: {
    '--credential-proof-space': `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface AuthUnlockWizardProps {
  open: boolean
  onClose: () => void
  onConfirm: () => Promise<void>
  title: string
  description?: string
  confirmLabel?: React.ReactNode
  threshold: number
  account: Resource<Account>
  retainAfterClose?: boolean
}

// AuthUnlockWizard handles multi-sig unlock flows. It shows the list of entity
// keypairs with their lock/unlock status and lets the user unlock enough keypairs
// to meet the threshold before executing the confirmed mutation.
export function AuthUnlockWizard({ account, ...props }: AuthUnlockWizardProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    return (
      <AuthUnlockWizardContent
        {...props}
        account={account}
        keypairs={state.entityKeypairs.value?.keypairs ?? []}
        unlockedCount={state.entityKeypairs.value?.unlockedCount ?? 0}
        loading={state.entityKeypairs.loading}
      />
    )
  }

  return <AuthUnlockWizardWithKeypairs {...props} account={account} />
}

function AuthUnlockWizardWithKeypairs(props: AuthUnlockWizardProps) {
  const { keypairs, unlockedCount, loading } = useEntityKeypairs(props.account)

  return (
    <AuthUnlockWizardContent
      {...props}
      keypairs={keypairs}
      unlockedCount={unlockedCount}
      loading={loading}
    />
  )
}

interface AuthUnlockWizardContentProps extends AuthUnlockWizardProps {
  keypairs: EntityKeypairState[]
  unlockedCount: number
  loading: boolean
}

function AuthUnlockWizardContent({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = 'Confirm',
  threshold,
  account,
  retainAfterClose = false,
  keypairs,
  unlockedCount,
  loading,
}: AuthUnlockWizardContentProps) {
  const required = threshold + 1
  const canConfirm = unlockedCount >= required

  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleLockAll = useCallback(async () => {
    if (retainAfterClose) {
      return
    }
    if (!account.value) {
      return
    }
    try {
      await account.value.lockAllEntityKeypairs()
    } catch {
      // best-effort cleanup
    }
  }, [account.value, retainAfterClose])

  const handleConfirm = useCallback(async () => {
    if (!canConfirm) {
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      await onConfirm()
      await handleLockAll()
      onClose()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      setError(mapAuthError(msg))
    } finally {
      setSubmitting(false)
    }
  }, [canConfirm, handleLockAll, onClose, onConfirm])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setError(null)
        void handleLockAll()
        onClose()
      }
    },
    [handleLockAll, onClose],
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {description ??
              `Unlock ${required} of ${keypairs.length} keypairs to authorize this operation.`}
          </DialogDescription>
        </DialogHeader>

        {loading && <p sx={stylexStyles2.rule1}>Loading keypairs…</p>}

        {!loading && keypairs.length > 0 && (
          <div>
            <div sx={[nativeChildSpacingNextNext.s2, stylexStyles2.rule2]}>
              <span sx={stylexStyles2.rule1}>
                {unlockedCount} of {required} unlocked
              </span>
              <div
                sx={[
                  stylexStyles2.rule3,
                  canConfirm ? stylexStyles2.rule4 : stylexStyles2.rule5,
                ]}
              >
                {canConfirm ? 'Ready' : 'Unlock more'}
              </div>
            </div>

            <div sx={[nativeChildSpacingNextNext.s2, stylexStyles2.rule6]}>
              <div
                sx={stylexStyles2.rule7}
                style={{
                  '--progress-width': `${Math.min(100, (unlockedCount / required) * 100)}%`,
                }}
              />
            </div>

            <div sx={nativeChildSpacingNextNext.s2}>
              {keypairs.map((kp) => (
                <KeypairRow
                  sx={nativeChildSpacingNext.s1_5}
                  key={kp.keypair?.peerId ?? 'unknown'}
                  keypairState={kp}
                  account={account}
                  disabled={submitting}
                  onError={setError}
                />
              ))}
            </div>
          </div>
        )}

        {error && <p sx={stylexStyles2.rule8}>{error}</p>}

        <DialogFooter>
          <button
            type="button"
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
            sx={stylexStyles2.rule9}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleConfirm()}
            disabled={submitting || !canConfirm}
            sx={[stylexStyles2.rule10]}
          >
            {submitting ? 'Confirming…' : confirmLabel}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface KeypairRowProps {
  keypairState: EntityKeypairState
  account: Resource<Account>
  disabled: boolean
  onError: (msg: string | null) => void
}

// KeypairRow renders a single entity keypair with its lock status and the
// unlock controls its auth method needs.
function KeypairRow({
  sx,
  keypairState,
  account,
  disabled,
  onError,
}: KeypairRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const peerId = keypairState.keypair?.peerId ?? ''
  const method = keypairState.keypair?.authMethod ?? 'unknown'
  const props = {
    account,
    peerId,
    method,
    truncated: truncatePeerId(peerId),
    disabled,
    onError,
  }

  if (keypairState.unlocked) {
    return <UnlockedKeypairRow sx={sx} {...props} />
  }
  if (
    method === 'passkey' ||
    method === 'google_sso' ||
    method === 'github_sso'
  ) {
    return <BrowserKeypairRow sx={sx} {...props} />
  }
  return <CredentialKeypairRow sx={sx} {...props} />
}

interface KeypairRowStateProps {
  account: Resource<Account>
  peerId: string
  method: string
  truncated: string
  disabled: boolean
  onError: (msg: string | null) => void
}

// UnlockedKeypairRow renders an unlocked keypair with its lock control.
function UnlockedKeypairRow({
  sx,
  account,
  peerId,
  method,
  truncated,
  disabled,
  onError,
}: KeypairRowStateProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const handleLock = async () => {
    if (!account.value || !peerId) {
      return
    }
    try {
      await account.value.lockEntityKeypair(peerId)
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Lock failed')
    }
  }

  return (
    <div sx={[stylexStyles2.rule11, sx]}>
      <div sx={stylexStyles2.rule12}>
        <LuLockOpen sx={stylexStyles.rule1} />
        <div sx={stylexStyles2.rule13}>
          <p sx={stylexStyles2.rule14}>{methodLabel(method)}</p>
          <p sx={stylexStyles2.rule15}>{truncated}</p>
        </div>
      </div>
      <button
        type="button"
        onClick={() => void handleLock()}
        disabled={disabled}
        sx={stylexStyles2.rule16}
      >
        Lock
      </button>
    </div>
  )
}

// CredentialKeypairRow unlocks a password or backup-key keypair.
function CredentialKeypairRow({
  sx,
  account,
  peerId,
  method,
  truncated,
  disabled,
  onError,
}: KeypairRowStateProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const runner = useUnlockRunner(onError)
  const { credential, unlockWithCredential } = useCredentialUnlock(
    account,
    peerId,
    runner,
  )

  return (
    <CredentialUnlockCard
      sx={sx}
      method={method}
      truncated={truncated}
      credential={credential}
      disabled={disabled || runner.unlocking}
      unlocking={runner.unlocking}
      showPassword={method === 'password'}
      onUnlock={() => void unlockWithCredential()}
    />
  )
}

// BrowserKeypairRow unlocks a passkey or SSO keypair through a browser or
// desktop ceremony, resuming with a PIN when the recovered key is PIN wrapped.
function BrowserKeypairRow({
  sx,
  account,
  peerId,
  method,
  truncated,
  disabled,
  onError,
}: KeypairRowStateProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const runner = useUnlockRunner(onError)
  const recovered = useRecoveredUnlock(account, peerId, runner)
  const flow = useBrowserFlow()
  const { unlockWithPasskey, unlockWithSSO } = useBrowserUnlock({
    peerId,
    method,
    runner,
    flow,
    finishRecovery: recovered.finishRecovery,
  })

  const handleCancel = () => {
    flow.cancel()
    runner.setUnlocking(false)
  }

  return (
    <BrowserUnlockCard
      sx={sx}
      method={method}
      truncated={truncated}
      pin={recovered.pin}
      onPinChange={recovered.setPin}
      needsPin={recovered.pendingRecovered?.case === 'pin'}
      waiting={runner.unlocking}
      disabled={disabled}
      flowActive={flow.active}
      desktopRelayActive={flow.desktopRelayActive}
      onStart={() => {
        if (method === 'passkey') {
          void unlockWithPasskey()
          return
        }
        void unlockWithSSO()
      }}
      onCancel={flow.active ? handleCancel : undefined}
      onUnlockPin={() => void recovered.unlockWithPin()}
    />
  )
}

interface UnlockRunner {
  onError: (msg: string | null) => void
  unlocking: boolean
  setUnlocking: (unlocking: boolean) => void
  runUnlock: (
    work: () => Promise<void>,
    opts?: { ignoreCanceled?: boolean; cleanup?: () => void },
  ) => Promise<void>
}

// useUnlockRunner tracks the in-flight unlock and reports its failure through
// onError. ignoreCanceled drops a user-canceled ceremony silently, and cleanup
// runs once the unlock settles.
function useUnlockRunner(onError: (msg: string | null) => void): UnlockRunner {
  const [unlocking, setUnlocking] = useState(false)

  const runUnlock: UnlockRunner['runUnlock'] = async (work, opts = {}) => {
    setUnlocking(true)
    onError(null)
    try {
      await work()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      if (!opts.ignoreCanceled || !msg.includes('canceled')) {
        onError(mapAuthError(msg))
      }
    } finally {
      opts.cleanup?.()
      setUnlocking(false)
    }
  }

  return { onError, unlocking, setUnlocking, runUnlock }
}

// useCredentialUnlock unlocks a keypair with the password or backup key held
// by the credential proof.
function useCredentialUnlock(
  account: Resource<Account>,
  peerId: string,
  runner: UnlockRunner,
) {
  const credential = useCredentialProof()

  const unlockWithCredential = async () => {
    const mounted = account.value
    const proof = credential.credential
    if (!mounted || !peerId || !proof) {
      return
    }
    await runner.runUnlock(async () => {
      await mounted.unlockEntityKeypair(peerId, proof)
      credential.reset()
    })
  }

  return { credential, unlockWithCredential }
}

// useRecoveredUnlock unlocks a keypair from a recovered PEM, holding a
// PIN-wrapped recovery until the user enters the PIN.
function useRecoveredUnlock(
  account: Resource<Account>,
  peerId: string,
  runner: UnlockRunner,
) {
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [pin, setPin] = useState('')
  const [pendingRecovered, setPendingRecovered] =
    useState<RecoveredEntityPem | null>(null)

  const unlockWithPem = async (pemPrivateKey: Uint8Array) => {
    if (!account.value || !peerId) {
      return
    }
    await account.value.unlockEntityKeypair(peerId, {
      credential: {
        case: 'pemPrivateKey',
        value: pemPrivateKey,
      },
    })
    setPendingRecovered(null)
    setPin('')
  }

  const finishRecovery = async (recovered: RecoveredEntityPem) => {
    if (recovered.case === 'pin') {
      setPendingRecovered(recovered)
      return
    }
    await unlockWithPem(recovered.pemPrivateKey)
  }

  const unlockWithPin = async () => {
    if (!pendingRecovered) {
      return
    }
    if (!root) {
      runner.onError('Provider is not ready')
      return
    }
    await runner.runUnlock(async () => {
      await unlockWithPem(
        await resolveRecoveredEntityPem(root, pendingRecovered, pin),
      )
    })
  }

  return { pin, setPin, pendingRecovered, finishRecovery, unlockWithPin }
}

// useBrowserFlow tracks the SSO popup and the desktop relays that a browser
// ceremony has in flight, and cancels whatever is active on unmount.
function useBrowserFlow() {
  const [ssoFlow, setSSOFlow] = useState<SSOPopupFlow | null>(null)
  const [desktopSSOAbort, setDesktopSSOAbort] =
    useState<AbortController | null>(null)
  const [desktopPasskeyAbort, setDesktopPasskeyAbort] =
    useState<AbortController | null>(null)

  useEffect(() => {
    return () => {
      ssoFlow?.cancel()
      desktopSSOAbort?.abort()
      desktopPasskeyAbort?.abort()
    }
  }, [desktopPasskeyAbort, desktopSSOAbort, ssoFlow])

  const cancel = () => {
    ssoFlow?.cancel()
    desktopSSOAbort?.abort()
    desktopPasskeyAbort?.abort()
    setDesktopSSOAbort(null)
    setDesktopPasskeyAbort(null)
    setSSOFlow(null)
  }

  return {
    active: !!ssoFlow || !!desktopSSOAbort || !!desktopPasskeyAbort,
    desktopRelayActive: !!desktopSSOAbort || !!desktopPasskeyAbort,
    setSSOFlow,
    setDesktopSSOAbort,
    setDesktopPasskeyAbort,
    cancel,
  }
}

type BrowserFlow = ReturnType<typeof useBrowserFlow>

// recoverPasskeyForSigner recovers the signer's PEM with a passkey. Desktop
// relays the ceremony through the session and reports its abort controller to
// onAbort.
async function recoverPasskeyForSigner(
  root: Root,
  session: Session | null,
  peerId: string,
  onAbort: (controller: AbortController) => void,
): Promise<RecoveredEntityPem> {
  if (!isDesktop) {
    return await recoverPasskeyEntityPem(root)
  }
  if (!session) {
    throw new Error('Session is not ready')
  }
  if (!peerId) {
    throw new Error('Signer is not available')
  }
  const controller = new AbortController()
  onAbort(controller)
  return await recoverPasskeyEntityPem(root, {
    desktopSession: session.spacewave,
    targetPeerId: peerId,
    abortSignal: controller.signal,
  })
}

// requestSSOCode runs the SSO ceremony for provider and resolves the
// authorization code. Desktop relays through the session; the browser opens a
// popup.
async function requestSSOCode(
  session: Session | null,
  config: ReturnType<typeof useCloudProviderConfig>,
  provider: string,
  flow: BrowserFlow,
): Promise<string> {
  if (isDesktop) {
    if (!session) {
      throw new Error('Session is not ready')
    }
    const controller = new AbortController()
    flow.setDesktopSSOAbort(controller)
    const resp = await session.spacewave.startDesktopSSOLink(
      { ssoProvider: provider },
      controller.signal,
    )
    if (!resp.code) {
      throw new Error('Desktop SSO unlock did not return an authorization code')
    }
    return resp.code
  }
  const ssoBaseUrl = config?.ssoBaseUrl ?? ''
  if (!ssoBaseUrl) {
    throw new Error('SSO is not configured')
  }
  const popup = startSSOPopupFlow({
    provider,
    ssoBaseUrl,
    origin: window.location.origin,
    mode: 'unlock',
  })
  flow.setSSOFlow(popup)
  return await popup.waitForResult
}

interface BrowserUnlockOptions {
  peerId: string
  method: string
  runner: UnlockRunner
  flow: BrowserFlow
  finishRecovery: (recovered: RecoveredEntityPem) => Promise<void>
}

// useBrowserUnlock starts the passkey or SSO ceremony that recovers a
// keypair's PEM.
function useBrowserUnlock({
  peerId,
  method,
  runner,
  flow,
  finishRecovery,
}: BrowserUnlockOptions) {
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const cloudProviderConfig = useCloudProviderConfig()

  const unlockWithPasskey = async () => {
    if (!root) {
      runner.onError('Not connected to server')
      return
    }
    await runner.runUnlock(
      async () => {
        await finishRecovery(
          await recoverPasskeyForSigner(
            root,
            session ?? null,
            peerId,
            flow.setDesktopPasskeyAbort,
          ),
        )
      },
      {
        ignoreCanceled: true,
        cleanup: () => flow.setDesktopPasskeyAbort(null),
      },
    )
  }

  const unlockWithSSO = async () => {
    const accountBaseUrl = cloudProviderConfig?.accountBaseUrl ?? ''
    if (!root || !accountBaseUrl) {
      runner.onError('SSO is not configured')
      return
    }
    const provider = method === 'google_sso' ? 'google' : 'github'
    await runner.runUnlock(
      async () => {
        const code = await requestSSOCode(
          session ?? null,
          cloudProviderConfig,
          provider,
          flow,
        )
        await finishRecovery(
          await recoverSSOEntityPem(
            root,
            provider,
            code,
            `${accountBaseUrl}/auth/sso/callback`,
          ),
        )
      },
      {
        ignoreCanceled: true,
        cleanup: () => {
          flow.setDesktopSSOAbort(null)
          flow.setSSOFlow(null)
        },
      },
    )
  }

  return { unlockWithPasskey, unlockWithSSO }
}

interface CredentialUnlockCardProps {
  method: string
  truncated: string
  credential: ReturnType<typeof useCredentialProof>
  disabled: boolean
  unlocking: boolean
  showPassword: boolean
  onUnlock: () => void
}

// CredentialUnlockCard renders the shared password / backup-key unlock form
// used by the settings escalation shell.
function CredentialUnlockCard({
  sx,
  method,
  truncated,
  credential,
  disabled,
  unlocking,
  showPassword,
  onUnlock,
}: CredentialUnlockCardProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const canUnlock = showPassword ? !!credential.password : !!credential.pemData

  return (
    <div sx={[stylexStyles3.rule1, sx]}>
      <UnlockCardHeader
        sx={nativeChildSpacing.s3}
        method={method}
        truncated={truncated}
      />

      <CredentialProofInput
        password={credential.password}
        onPasswordChange={credential.setPassword}
        pemFileName={credential.pemFileName}
        onFileChange={credential.handleFileChange}
        fileInputRef={credential.fileInputRef}
        showPassword={showPassword}
        showPem={!showPassword}
        passwordLabel="Password"
        passwordPlaceholder="Enter your password"
        pemLabel="Backup key (.pem)"
        disabled={disabled}
        sx={[nativeChildSpacing.s3, stylexStyles.credentialSpacing]}
        onPasswordKeyDown={(e) => {
          if (e.key === 'Enter' && canUnlock) {
            onUnlock()
          }
        }}
      />

      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule17]}>
        <button
          type="button"
          onClick={onUnlock}
          disabled={disabled || !canUnlock}
          sx={[stylexStyles2.rule18]}
        >
          {unlocking ? '...' : 'Unlock'}
        </button>
      </div>
    </div>
  )
}

interface BrowserUnlockCardProps {
  method: string
  truncated: string
  pin: string
  onPinChange: (pin: string) => void
  needsPin: boolean
  waiting: boolean
  disabled: boolean
  flowActive: boolean
  desktopRelayActive: boolean
  onStart: () => void
  onCancel?: () => void
  onUnlockPin: () => void
}

// describeBrowserUnlock explains what the shared passkey / SSO prompt expects
// from the user next.
function describeBrowserUnlock(
  method: string,
  needsPin: boolean,
  flowActive: boolean,
  desktopRelayActive: boolean,
): string {
  const isPasskey = method === 'passkey'
  if (needsPin) {
    return 'Enter the PIN for the recovered key to finish unlocking this signer.'
  }
  if (flowActive && desktopRelayActive && isPasskey) {
    return 'Complete passkey verification in your browser, then return here.'
  }
  if (flowActive) {
    return `Complete ${methodLabel(method)} in the browser, then return here.`
  }
  if (isPasskey) {
    return 'Use your passkey to unlock this signer in the shared escalation prompt.'
  }
  return `Use ${methodLabel(method)} to unlock this signer in the shared escalation prompt.`
}

// UnlockCardHeader renders the lock icon, method label, and signer id shared
// by the unlock cards.
function UnlockCardHeader({
  sx,
  method,
  truncated,
}: {
  method: string
  truncated: string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule19, sx]}>
      <LuLock sx={stylexStyles.rule2} />
      <div sx={stylexStyles2.rule20}>
        <p sx={stylexStyles2.rule14}>{methodLabel(method)}</p>
        <p sx={stylexStyles2.rule15}>{truncated}</p>
      </div>
    </div>
  )
}

// MethodIcon renders the icon for a passkey or SSO method.
function MethodIcon({ method }: { method: string }) {
  if (method === 'passkey') {
    return <LuFingerprint sx={stylexStyles.rule3} />
  }
  if (method === 'google_sso') {
    return <FcGoogle sx={stylexStyles.rule3} />
  }
  return <LuGithub sx={stylexStyles.rule3} />
}

// PinField renders the PIN input for resuming a PIN-wrapped recovery. Enter
// submits a non-empty PIN.
function PinField({
  sx,
  pin,
  onChange,
  onSubmit,
  locked,
}: {
  pin: string
  onChange: (pin: string) => void
  onSubmit: () => void
  locked: boolean
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <input
      type="password"
      value={pin}
      onChange={(e) => onChange(e.target.value)}
      aria-label="PIN"
      placeholder="Enter PIN"
      disabled={locked}
      readOnly={locked}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && pin.length > 0) {
          onSubmit()
        }
      }}
      sx={[[stylexStyles2.rule21], sx]}
    />
  )
}

// BrowserUnlockCard renders the shared passkey / SSO unlock states for browser
// ceremonies, including waiting, PIN resume, cancel, and retry.
function BrowserUnlockCard({
  sx,
  method,
  truncated,
  pin,
  onPinChange,
  needsPin,
  waiting,
  disabled,
  flowActive,
  desktopRelayActive,
  onStart,
  onCancel,
  onUnlockPin,
}: BrowserUnlockCardProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const helperText = describeBrowserUnlock(
    method,
    needsPin,
    flowActive,
    desktopRelayActive,
  )

  return (
    <div sx={[stylexStyles3.rule1, sx]}>
      <UnlockCardHeader
        sx={nativeChildSpacing.s3}
        method={method}
        truncated={truncated}
      />

      <p sx={[nativeChildSpacing.s3, stylexStyles2.rule1]}>{helperText}</p>

      {needsPin && (
        <PinField
          sx={nativeChildSpacing.s3}
          pin={pin}
          onChange={onPinChange}
          onSubmit={onUnlockPin}
          locked={disabled || waiting}
        />
      )}

      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule22]}>
        {needsPin && (
          <button
            type="button"
            onClick={onStart}
            disabled={disabled || waiting}
            sx={stylexStyles2.rule23}
          >
            Retry {methodLabel(method)}
          </button>
        )}
        {flowActive && onCancel && (
          <button
            type="button"
            onClick={onCancel}
            disabled={disabled}
            sx={stylexStyles2.rule23}
          >
            Cancel
          </button>
        )}
        {needsPin ? (
          <button
            type="button"
            onClick={onUnlockPin}
            disabled={disabled || waiting || pin.length === 0}
            sx={[stylexStyles2.rule18]}
          >
            {waiting ? '...' : 'Unlock'}
          </button>
        ) : (
          <button
            type="button"
            onClick={onStart}
            disabled={disabled || waiting}
            sx={[stylexStyles2.rule24]}
          >
            <MethodIcon method={method} />
            {waiting
              ? 'Waiting…'
              : method === 'passkey'
                ? 'Use passkey'
                : `Use ${methodLabel(method)}`}
          </button>
        )}
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
