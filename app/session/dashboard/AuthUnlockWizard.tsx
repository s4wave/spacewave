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
          <p className="text-foreground-alt text-xs">Loading keypairs...</p>
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
                  canConfirm ?
                    'bg-brand/10 text-brand'
                  : 'bg-foreground/5 text-foreground-alt',
                )}
              >
                {canConfirm ? 'Ready' : 'Unlock more'}
              </div>
            </div>

            <div className="bg-foreground/5 h-1.5 w-full overflow-hidden rounded-full">
              <div
                className="bg-brand h-full transition-all duration-300"
                style={{
                  width: `${Math.min(100, (unlockedCount / required) * 100)}%`,
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
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={() => void handleConfirm()}
            disabled={submitting || !canConfirm}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {submitting ? 'Confirming...' : confirmLabel}
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

// KeypairRow renders a single entity keypair with its lock status and unlock controls.
function KeypairRow({
  keypairState,
  account,
  disabled,
  onError,
}: KeypairRowProps) {
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const cloudProviderConfig = useCloudProviderConfig()
  const peerId = keypairState.keypair?.peerId ?? ''
  const method = keypairState.keypair?.authMethod ?? 'unknown'
  const unlocked = keypairState.unlocked ?? false
  const truncated = truncatePeerId(peerId)

  const cred = useCredentialProof()
  const [unlocking, setUnlocking] = useState(false)
  const [pin, setPin] = useState('')
  const [pendingRecovered, setPendingRecovered] =
    useState<RecoveredEntityPem | null>(null)
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

  const needsPassword = method === 'password'
  const needsPasskey = method === 'passkey'
  const needsSSO = method === 'google_sso' || method === 'github_sso'
  const needsPin = pendingRecovered?.case === 'pin'

  const unlockWithCredential = useCallback(async () => {
    if (!account.value || !peerId || !cred.credential) {
      return
    }
    setUnlocking(true)
    onError(null)
    try {
      await account.value.unlockEntityKeypair(peerId, cred.credential)
      cred.reset()
      setPendingRecovered(null)
      setPin('')
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      onError(mapAuthError(msg))
    } finally {
      setUnlocking(false)
    }
  }, [account.value, cred, onError, peerId])

  const unlockWithPem = useCallback(
    async (pemPrivateKey: Uint8Array) => {
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
    },
    [account.value, peerId],
  )

  const handlePasskeyUnlock = useCallback(async () => {
    if (!root) {
      onError('Not connected to server')
      return
    }
    setUnlocking(true)
    onError(null)
    try {
      let recovered: RecoveredEntityPem
      if (isDesktop) {
        if (!session) {
          throw new Error('Session is not ready')
        }
        if (!peerId) {
          throw new Error('Signer is not available')
        }
        const controller = new AbortController()
        setDesktopPasskeyAbort(controller)
        recovered = await recoverPasskeyEntityPem(root, {
          desktopSession: session.spacewave,
          targetPeerId: peerId,
          abortSignal: controller.signal,
        })
      } else {
        recovered = await recoverPasskeyEntityPem(root)
      }
      if (recovered.case === 'pin') {
        setPendingRecovered(recovered)
        return
      }
      await unlockWithPem(recovered.pemPrivateKey)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      if (msg.includes('canceled')) {
        return
      }
      onError(mapAuthError(msg))
    } finally {
      setDesktopPasskeyAbort(null)
      setUnlocking(false)
    }
  }, [onError, peerId, root, session, unlockWithPem])

  const handleSSOUnlock = useCallback(async () => {
    const accountBaseUrl = cloudProviderConfig?.accountBaseUrl ?? ''
    if (!root || !accountBaseUrl) {
      onError('SSO is not configured')
      return
    }
    setUnlocking(true)
    onError(null)
    try {
      let code: string
      const provider = method === 'google_sso' ? 'google' : 'github'
      if (isDesktop) {
        if (!session) {
          throw new Error('Session is not ready')
        }
        const controller = new AbortController()
        setDesktopSSOAbort(controller)
        const resp = await session.spacewave.startDesktopSSOLink(
          { ssoProvider: provider },
          controller.signal,
        )
        code = resp.code ?? ''
        if (!code) {
          throw new Error(
            'Desktop SSO unlock did not return an authorization code',
          )
        }
      } else {
        const ssoBaseUrl = cloudProviderConfig?.ssoBaseUrl ?? ''
        if (!ssoBaseUrl) {
          throw new Error('SSO is not configured')
        }
        const flow = startSSOPopupFlow({
          provider,
          ssoBaseUrl,
          origin: window.location.origin,
          mode: 'unlock',
        })
        setSSOFlow(flow)
        code = await flow.waitForResult
      }
      const recovered = await recoverSSOEntityPem(
        root,
        provider,
        code,
        `${accountBaseUrl}/auth/sso/callback`,
      )
      if (recovered.case === 'pin') {
        setPendingRecovered(recovered)
        return
      }
      await unlockWithPem(recovered.pemPrivateKey)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      if (msg.includes('canceled')) {
        return
      }
      onError(mapAuthError(msg))
    } finally {
      setDesktopSSOAbort(null)
      setSSOFlow(null)
      setUnlocking(false)
    }
  }, [cloudProviderConfig, method, onError, root, session, unlockWithPem])

  const handlePinUnlock = useCallback(async () => {
    if (!pendingRecovered) {
      return
    }
    if (!root) {
      onError('Provider is not ready')
      return
    }
    setUnlocking(true)
    onError(null)
    try {
      const pemPrivateKey = await resolveRecoveredEntityPem(
        root,
        pendingRecovered,
        pin,
      )
      await unlockWithPem(pemPrivateKey)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      onError(mapAuthError(msg))
    } finally {
      setUnlocking(false)
    }
  }, [onError, pendingRecovered, pin, root, unlockWithPem])

  const handleLock = useCallback(async () => {
    if (!account.value || !peerId) {
      return
    }
    try {
      await account.value.lockEntityKeypair(peerId)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Lock failed'
      onError(msg)
    }
  }, [account.value, onError, peerId])

  if (unlocked) {
    return (
      <div className="border-foreground/10 flex items-center justify-between gap-2 rounded-md border px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <LuLockOpen className="text-brand h-3.5 w-3.5 shrink-0" />
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
          onClick={() => void handleLock()}
          disabled={disabled}
          className="text-foreground-alt hover:text-foreground text-xs transition-colors disabled:opacity-50"
        >
          Lock
        </button>
      </div>
    )
  }

  if (!needsPasskey && !needsSSO && !needsPin) {
    return (
      <CredentialUnlockCard
        method={method}
        truncated={truncated}
        credential={cred}
        disabled={disabled || unlocking}
        unlocking={unlocking}
        showPassword={needsPassword}
        onUnlock={() => void unlockWithCredential()}
      />
    )
  }

  const handleCancelBrowserFlow = () => {
    ssoFlow?.cancel()
    desktopSSOAbort?.abort()
    desktopPasskeyAbort?.abort()
    setDesktopSSOAbort(null)
    setDesktopPasskeyAbort(null)
    setSSOFlow(null)
    setUnlocking(false)
  }

  return (
    <BrowserUnlockCard
      method={method}
      truncated={truncated}
      pin={pin}
      onPinChange={setPin}
      needsPin={needsPin}
      waiting={unlocking}
      disabled={disabled}
      flowActive={!!ssoFlow || !!desktopSSOAbort || !!desktopPasskeyAbort}
      desktopRelayActive={!!desktopSSOAbort || !!desktopPasskeyAbort}
      onStart={() => {
        if (needsPasskey) {
          void handlePasskeyUnlock()
          return
        }
        void handleSSOUnlock()
      }}
      onCancel={
        ssoFlow || desktopSSOAbort || desktopPasskeyAbort ?
          handleCancelBrowserFlow
        : undefined
      }
      onUnlockPin={() => void handlePinUnlock()}
    />
  )
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
    <div className="border-foreground/10 space-y-3 rounded-md border px-3 py-3">
      <div className="flex items-center gap-2">
        <LuLock className="text-foreground-alt h-3.5 w-3.5 shrink-0" />
        <div className="min-w-0 flex-1">
          <p className="text-foreground text-sm font-medium">
            {methodLabel(method)}
          </p>
          <p className="text-foreground-alt truncate font-mono text-xs">
            {truncated}
          </p>
        </div>
      </div>

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
  const isPasskey = method === 'passkey'
  const actionLabel = isPasskey ? 'Use passkey' : `Use ${methodLabel(method)}`
  const helperText =
    needsPin ?
      'Enter the PIN for the recovered key to finish unlocking this signer.'
    : flowActive && desktopRelayActive && isPasskey ?
      'Complete passkey verification in your browser, then return here.'
    : flowActive ?
      `Complete ${methodLabel(method)} in the browser, then return here.`
    : isPasskey ?
      'Use your passkey to unlock this signer in the shared escalation prompt.'
    : `Use ${methodLabel(method)} to unlock this signer in the shared escalation prompt.`

  return (
    <div className="border-foreground/10 space-y-3 rounded-md border px-3 py-3">
      <div className="flex items-center gap-2">
        <LuLock className="text-foreground-alt h-3.5 w-3.5 shrink-0" />
        <div className="min-w-0 flex-1">
          <p className="text-foreground text-sm font-medium">
            {methodLabel(method)}
          </p>
          <p className="text-foreground-alt truncate font-mono text-xs">
            {truncated}
          </p>
        </div>
      </div>

      <p className="text-foreground-alt text-xs">{helperText}</p>

      {needsPin && (
        <input
          type="password"
          value={pin}
          onChange={(e) => onPinChange(e.target.value)}
          placeholder="Enter PIN"
          disabled={disabled || waiting}
          readOnly={disabled || waiting}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && pin.length > 0) {
              onUnlockPin()
            }
          }}
          className={cn(
            'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-2.5 py-1.5 text-xs transition-colors outline-none',
            'focus:border-brand/50 disabled:opacity-50',
          )}
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
        <button
          onClick={needsPin ? onUnlockPin : onStart}
          disabled={
            needsPin ?
              disabled || waiting || pin.length === 0
            : disabled || waiting
          }
          className={cn(
            'shrink-0 rounded-md border px-3 py-1.5 text-xs transition-all',
            'border-brand/30 bg-brand/10 hover:bg-brand/20',
            'disabled:cursor-not-allowed disabled:opacity-50',
            !needsPin && 'inline-flex items-center gap-1.5',
          )}
        >
          {!needsPin && isPasskey && <LuFingerprint className="h-3 w-3" />}
          {!needsPin &&
            !isPasskey &&
            (method === 'google_sso' ?
              <FcGoogle className="h-3 w-3" />
            : <LuGithub className="h-3 w-3" />)}
          {needsPin ?
            waiting ?
              '...'
            : 'Unlock'
          : waiting ?
            'Waiting...'
          : actionLabel}
        </button>
      </div>
    </div>
  )
}
