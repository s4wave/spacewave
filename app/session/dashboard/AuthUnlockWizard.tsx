import { useCallback, useEffect, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { FcGoogle } from 'react-icons/fc'
import { LuFingerprint, LuGithub, LuLock, LuLockOpen } from 'react-icons/lu'

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
import { cn } from '@s4wave/web/style/utils.js'
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

        {loading && (
          <p className="text-foreground-alt text-xs">Loading keypairs…</p>
        )}

        {!loading && keypairs.length > 0 && (
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-foreground-alt text-xs">
                {unlockedCount} of {required} unlocked
              </span>
              <div
                className={cn(
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  canConfirm
                    ? 'bg-brand/10 text-brand'
                    : 'bg-foreground/5 text-foreground-alt',
                )}
              >
                {canConfirm ? 'Ready' : 'Unlock more'}
              </div>
            </div>

            <div className="bg-foreground/5 h-1.5 w-full overflow-hidden rounded-full">
              <div
                className="bg-brand progress-width progress-width-transition-medium h-full"
                style={{
                  '--progress-width': `${Math.min(100, (unlockedCount / required) * 100)}%`,
                }}
              />
            </div>

            <div className="space-y-1.5">
              {keypairs.map((kp) => (
                <KeypairRow
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

        {error && <p className="text-destructive text-xs">{error}</p>}

        <DialogFooter>
          <button
            type="button"
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleConfirm()}
            disabled={submitting || !canConfirm}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
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
  keypairState,
  account,
  disabled,
  onError,
}: KeypairRowProps) {
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
    return <UnlockedKeypairRow {...props} />
  }
  if (
    method === 'passkey' ||
    method === 'google_sso' ||
    method === 'github_sso'
  ) {
    return <BrowserKeypairRow {...props} />
  }
  return <CredentialKeypairRow {...props} />
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
  account,
  peerId,
  method,
  truncated,
  disabled,
  onError,
}: KeypairRowStateProps) {
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
    <div className="border-foreground/10 flex items-center justify-between gap-2 rounded-md border px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        <LuLockOpen className="text-brand size-3.5 shrink-0" />
        <div className="min-w-0">
          <p className="text-foreground text-sm font-medium">
            {methodLabel(method)}
          </p>
          <p className="text-foreground-alt truncate font-mono text-xs">
            {truncated}
          </p>
        </div>
      </div>
      <button
        type="button"
        onClick={() => void handleLock()}
        disabled={disabled}
        className="text-foreground-alt hover:text-foreground text-xs transition-colors disabled:opacity-50"
      >
        Lock
      </button>
    </div>
  )
}

// CredentialKeypairRow unlocks a password or backup-key keypair.
function CredentialKeypairRow({
  account,
  peerId,
  method,
  truncated,
  disabled,
  onError,
}: KeypairRowStateProps) {
  const runner = useUnlockRunner(onError)
  const { credential, unlockWithCredential } = useCredentialUnlock(
    account,
    peerId,
    runner,
  )

  return (
    <CredentialUnlockCard
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
  account,
  peerId,
  method,
  truncated,
  disabled,
  onError,
}: KeypairRowStateProps) {
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
  method,
  truncated,
  credential,
  disabled,
  unlocking,
  showPassword,
  onUnlock,
}: CredentialUnlockCardProps) {
  const canUnlock = showPassword ? !!credential.password : !!credential.pemData

  return (
    <div className="border-foreground/10 space-y-3 rounded-md border p-3">
      <UnlockCardHeader method={method} truncated={truncated} />

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
        className="space-y-2"
        onPasswordKeyDown={(e) => {
          if (e.key === 'Enter' && canUnlock) {
            onUnlock()
          }
        }}
      />

      <div className="flex justify-end">
        <button
          type="button"
          onClick={onUnlock}
          disabled={disabled || !canUnlock}
          className={cn(
            'shrink-0 rounded-md border px-3 py-1.5 text-xs transition-all',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
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
  method,
  truncated,
}: {
  method: string
  truncated: string
}) {
  return (
    <div className="flex items-center gap-2">
      <LuLock className="text-foreground-alt size-3.5 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="text-foreground text-sm font-medium">
          {methodLabel(method)}
        </p>
        <p className="text-foreground-alt truncate font-mono text-xs">
          {truncated}
        </p>
      </div>
    </div>
  )
}

// MethodIcon renders the icon for a passkey or SSO method.
function MethodIcon({ method }: { method: string }) {
  if (method === 'passkey') {
    return <LuFingerprint className="size-3" />
  }
  if (method === 'google_sso') {
    return <FcGoogle className="size-3" />
  }
  return <LuGithub className="size-3" />
}

// PinField renders the PIN input for resuming a PIN-wrapped recovery. Enter
// submits a non-empty PIN.
function PinField({
  pin,
  onChange,
  onSubmit,
  locked,
}: {
  pin: string
  onChange: (pin: string) => void
  onSubmit: () => void
  locked: boolean
}) {
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
      className={cn(
        'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-2.5 py-1.5 text-xs transition-colors outline-none',
        'focus:border-brand/50 disabled:opacity-50',
      )}
    />
  )
}

// BrowserUnlockCard renders the shared passkey / SSO unlock states for browser
// ceremonies, including waiting, PIN resume, cancel, and retry.
function BrowserUnlockCard({
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
}: BrowserUnlockCardProps) {
  const helperText = describeBrowserUnlock(
    method,
    needsPin,
    flowActive,
    desktopRelayActive,
  )

  return (
    <div className="border-foreground/10 space-y-3 rounded-md border p-3">
      <UnlockCardHeader method={method} truncated={truncated} />

      <p className="text-foreground-alt text-xs">{helperText}</p>

      {needsPin && (
        <PinField
          pin={pin}
          onChange={onPinChange}
          onSubmit={onUnlockPin}
          locked={disabled || waiting}
        />
      )}

      <div className="flex flex-wrap justify-end gap-2">
        {needsPin && (
          <button
            type="button"
            onClick={onStart}
            disabled={disabled || waiting}
            className="text-foreground-alt hover:text-foreground rounded-md px-2 py-1 text-xs transition-colors disabled:opacity-50"
          >
            Retry {methodLabel(method)}
          </button>
        )}
        {flowActive && onCancel && (
          <button
            type="button"
            onClick={onCancel}
            disabled={disabled}
            className="text-foreground-alt hover:text-foreground rounded-md px-2 py-1 text-xs transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
        )}
        {needsPin ? (
          <button
            type="button"
            onClick={onUnlockPin}
            disabled={disabled || waiting || pin.length === 0}
            className={cn(
              'shrink-0 rounded-md border px-3 py-1.5 text-xs transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {waiting ? '...' : 'Unlock'}
          </button>
        ) : (
          <button
            type="button"
            onClick={onStart}
            disabled={disabled || waiting}
            className={cn(
              'shrink-0 rounded-md border px-3 py-1.5 text-xs transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
              'inline-flex items-center gap-1.5',
            )}
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
