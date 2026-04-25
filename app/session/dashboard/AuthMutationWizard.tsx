import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCheck,
  LuDownload,
  LuKey,
  LuLock,
  LuLockOpen,
  LuShieldCheck,
  LuTrash2,
  LuUpload,
} from 'react-icons/lu'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { downloadPemFile } from '@s4wave/web/download.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import {
  mapAuthError,
  methodLabel,
  truncatePeerId,
} from '@s4wave/web/ui/credential/auth-utils.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type {
  EntityKeypairState,
  WatchAccountInfoResponse,
} from '@s4wave/sdk/account/account.pb.js'

import { useAccountDashboardState } from './AccountDashboardStateContext.js'
import { useEntityKeypairs } from './useEntityKeypairs.js'

// WizardMode determines the operation type.
export type WizardMode = 'add' | 'remove'

// AddMethodType identifies the method type to add.
export type AddMethodType = 'pem' | 'passkey'

export interface AuthMutationWizardProps {
  open: boolean
  onClose: () => void
  mode: WizardMode
  account: Resource<Account>
  retainAfterClose?: boolean
  // For remove: the peer ID of the method being removed.
  removePeerId?: string
  removeMethodLabel?: string
  // For add: the method type being added.
  addMethodType?: AddMethodType
}

// stepLabels returns the step labels for the given mode.
function stepLabels(mode: WizardMode): string[] {
  if (mode === 'add') {
    return ['Provide credential', 'Unlock keypairs', 'Confirm']
  }
  return ['Confirm removal', 'Unlock keypairs', 'Execute']
}

// AuthMutationWizard provides a sequential step-through wizard for add/remove
// auth method operations. Each step must be completed before advancing. When
// auth_threshold > 0, the unlock step requires enough keypairs to be unlocked.
// For threshold=0 (single-sig), the unlock step is condensed.
export function AuthMutationWizard({
  account,
  ...props
}: AuthMutationWizardProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    return (
      <AuthMutationWizardContent
        {...props}
        account={account}
        accountInfoResource={state.accountInfo}
      />
    )
  }

  return <AuthMutationWizardWithWatch {...props} account={account} />
}

function AuthMutationWizardWithWatch(props: AuthMutationWizardProps) {
  const accountInfoResource = useStreamingResource(
    props.account,
    (acc, signal) => acc.watchAccountInfo({}, signal),
    [],
  )

  return (
    <AuthMutationWizardContent
      {...props}
      accountInfoResource={accountInfoResource}
    />
  )
}

interface AuthMutationWizardContentProps extends AuthMutationWizardProps {
  accountInfoResource: Resource<WatchAccountInfoResponse>
}

function AuthMutationWizardContent({
  open,
  onClose,
  mode,
  account,
  retainAfterClose = false,
  removePeerId,
  removeMethodLabel,
  addMethodType,
  accountInfoResource,
}: AuthMutationWizardContentProps) {
  const [step, setStep] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [executing, setExecuting] = useState(false)
  const [complete, setComplete] = useState(false)

  const cred = useCredentialProof()

  const threshold = accountInfoResource.value?.authThreshold ?? 0

  const {
    keypairs,
    unlockedCount,
    loading: keypairsLoading,
  } = useEntityKeypairs(account)
  const required = threshold + 1
  const canExecute = unlockedCount >= required

  const labels = useMemo(() => stepLabels(mode), [mode])

  const handleExecute = useCallback(async () => {
    if (!account.value || !canExecute) return
    setExecuting(true)
    setError(null)
    try {
      if (mode === 'add' && addMethodType === 'pem') {
        const credential = cred.credential
        if (!credential) return
        const resp = await account.value.generateBackupKey({ credential })
        const data = resp.pemData
        if (data && data.length > 0) {
          downloadPemFile(data)
        }
      } else if (mode === 'remove' && removePeerId) {
        // In multi-sig mode the server uses unlocked keypairs to sign
        // internally, so pass an empty credential.
        await account.value.removeAuthMethod({
          peerId: removePeerId,
          credential: cred.credential ?? {
            credential: { case: 'password' as const, value: '' },
          },
        })
      }
      setComplete(true)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      setError(mapAuthError(msg))
    } finally {
      setExecuting(false)
    }
  }, [
    account.value,
    canExecute,
    mode,
    addMethodType,
    removePeerId,
    cred.credential,
  ])

  const handleLockAll = useCallback(async () => {
    if (retainAfterClose) return
    if (!account.value) return
    try {
      await account.value.lockAllEntityKeypairs()
    } catch {
      // best-effort cleanup
    }
  }, [account.value, retainAfterClose])

  const resetState = useCallback(() => {
    setStep(0)
    setError(null)
    setExecuting(false)
    setComplete(false)
    cred.reset()
  }, [cred])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        void handleLockAll()
        resetState()
        onClose()
      }
    },
    [onClose, handleLockAll, resetState],
  )

  const handleDone = useCallback(() => {
    void handleLockAll()
    resetState()
    onClose()
  }, [onClose, handleLockAll, resetState])

  // Step 0 readiness depends on mode.
  const step0Ready = mode === 'add' ? cred.hasCredential : !!removePeerId

  const title =
    mode === 'add' ?
      addMethodType === 'pem' ?
        'Add backup key'
      : 'Add auth method'
    : 'Remove auth method'

  const description =
    mode === 'add' ?
      'Step through the wizard to securely add a new auth method.'
    : `Remove "${removeMethodLabel ?? 'auth method'}" from your account.`

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        {/* Step indicator */}
        <StepIndicator labels={labels} current={step} complete={complete} />

        {/* Step content */}
        <div className="min-h-[120px]">
          {complete ?
            <CompleteView mode={mode} addMethodType={addMethodType} />
          : step === 0 ?
            <Step0Content
              mode={mode}
              removePeerId={removePeerId}
              removeMethodLabel={removeMethodLabel}
              addMethodType={addMethodType}
              cred={cred}
              keypairs={keypairs}
            />
          : step === 1 ?
            <Step1Unlock
              keypairs={keypairs}
              unlockedCount={unlockedCount}
              required={required}
              loading={keypairsLoading}
              account={account}
              onError={setError}
            />
          : <Step2Confirm
              mode={mode}
              addMethodType={addMethodType}
              removeMethodLabel={removeMethodLabel}
              keypairs={keypairs}
              unlockedCount={unlockedCount}
              required={required}
            />
          }

          {error && <p className="text-destructive mt-2 text-xs">{error}</p>}
        </div>

        <DialogFooter>
          {complete ?
            <button
              onClick={handleDone}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-brand/30 bg-brand/10 hover:bg-brand/20',
              )}
            >
              Done
            </button>
          : <>
              {step > 0 && (
                <button
                  onClick={() => {
                    setStep(step - 1)
                    setError(null)
                  }}
                  disabled={executing}
                  className="text-foreground-alt hover:text-foreground flex items-center gap-1 rounded-md px-3 py-2 text-sm transition-colors"
                >
                  <LuArrowLeft className="h-3 w-3" />
                  Back
                </button>
              )}
              <div className="flex-1" />
              <button
                onClick={() => handleOpenChange(false)}
                disabled={executing}
                className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
              >
                Cancel
              </button>
              {step < 2 ?
                <button
                  onClick={() => {
                    setError(null)
                    setStep(step + 1)
                  }}
                  disabled={step === 0 && !step0Ready}
                  className={cn(
                    'flex items-center gap-1 rounded-md border px-4 py-2 text-sm transition-all',
                    'border-brand/30 bg-brand/10 hover:bg-brand/20',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  Next
                  <LuArrowRight className="h-3 w-3" />
                </button>
              : <button
                  onClick={() => void handleExecute()}
                  disabled={executing || !canExecute}
                  className={cn(
                    'flex items-center gap-1 rounded-md border px-4 py-2 text-sm transition-all',
                    mode === 'remove' ?
                      'border-destructive/30 bg-destructive/10 hover:bg-destructive/20 text-destructive'
                    : 'border-brand/30 bg-brand/10 hover:bg-brand/20',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  {executing ?
                    <>
                      <Spinner size="sm" />
                      Executing...
                    </>
                  : mode === 'remove' ?
                    <>
                      <LuTrash2 className="h-3 w-3" />
                      Remove
                    </>
                  : <>
                      <LuDownload className="h-3 w-3" />
                      Generate and download
                    </>
                  }
                </button>
              }
            </>
          }
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// StepIndicator renders the horizontal step bar.
function StepIndicator({
  labels,
  current,
  complete,
}: {
  labels: string[]
  current: number
  complete: boolean
}) {
  return (
    <div className="flex items-center gap-1">
      {labels.map((label, i) => {
        const done = complete || i < current
        const active = !complete && i === current
        return (
          <div key={label} className="flex flex-1 flex-col items-center gap-1">
            <div
              className={cn(
                'h-1 w-full rounded-full transition-colors',
                done ? 'bg-brand'
                : active ? 'bg-brand/40'
                : 'bg-foreground/10',
              )}
            />
            <span
              className={cn(
                'text-[10px] leading-tight',
                done ? 'text-brand'
                : active ? 'text-foreground'
                : 'text-foreground-alt/50',
              )}
            >
              {label}
            </span>
          </div>
        )
      })}
    </div>
  )
}

// Step0Content renders the first step content (credential for add, confirmation for remove).
function Step0Content({
  mode,
  removePeerId,
  removeMethodLabel,
  addMethodType,
  cred,
  keypairs,
}: {
  mode: WizardMode
  removePeerId?: string
  removeMethodLabel?: string
  addMethodType?: AddMethodType
  cred: ReturnType<typeof useCredentialProof>
  keypairs: EntityKeypairState[]
}) {
  if (mode === 'remove') {
    const truncated = truncatePeerId(removePeerId ?? '')
    return (
      <div className="space-y-3">
        <div className="border-destructive/20 bg-destructive/5 rounded-md border p-3">
          <p className="text-foreground text-sm font-medium">
            Remove {removeMethodLabel ?? 'auth method'}
          </p>
          <p className="text-foreground-alt mt-1 font-mono text-xs">
            {truncated}
          </p>
        </div>
        <p className="text-foreground-alt text-xs">
          This action cannot be undone. You will need to unlock enough keypairs
          in the next step to authorize this removal.
        </p>
      </div>
    )
  }

  // Add mode: collect credential to prove identity.
  const hasPassword = keypairs.some(
    (kp) => kp.keypair?.authMethod === 'password',
  )

  return (
    <div className="space-y-3">
      {addMethodType === 'pem' && (
        <div className="border-brand/20 bg-brand/5 flex items-start gap-2 rounded-md border p-3">
          <LuKey className="text-brand mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <p className="text-foreground text-sm font-medium">
              Backup key (.pem)
            </p>
            <p className="text-foreground-alt mt-0.5 text-xs">
              A new Ed25519 keypair will be generated and the private key
              downloaded as a .pem file for offline recovery.
            </p>
          </div>
        </div>
      )}

      <p className="text-foreground-alt text-xs">
        Confirm your identity to proceed. Enter your password or upload a backup
        key file.
      </p>

      <CredentialProofInput
        password={cred.password}
        onPasswordChange={cred.setPassword}
        pemFileName={cred.pemFileName}
        onFileChange={cred.handleFileChange}
        fileInputRef={cred.fileInputRef}
        showPassword={hasPassword}
        showPem={!hasPassword}
        pemLabel="Select your backup key file"
      />
    </div>
  )
}

// Step1Unlock renders the keypair unlock step.
function Step1Unlock({
  keypairs,
  unlockedCount,
  required,
  loading,
  account,
  onError,
}: {
  keypairs: EntityKeypairState[]
  unlockedCount: number
  required: number
  loading: boolean
  account: Resource<Account>
  onError: (msg: string | null) => void
}) {
  const ready = unlockedCount >= required

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-foreground-alt text-xs">
          {unlockedCount} of {required} unlocked
        </span>
        <div
          className={cn(
            'rounded-full px-2 py-0.5 text-xs font-medium',
            ready ?
              'bg-brand/10 text-brand'
            : 'bg-foreground/5 text-foreground-alt',
          )}
        >
          {ready ? 'Ready' : 'Unlock more'}
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

      {loading && (
        <p className="text-foreground-alt text-xs">Loading keypairs...</p>
      )}

      {!loading && keypairs.length > 0 && (
        <div className="space-y-1.5">
          {keypairs.map((kp) => (
            <UnlockRow
              key={kp.keypair?.peerId ?? 'unknown'}
              keypairState={kp}
              account={account}
              onError={onError}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// UnlockRow renders a single entity keypair with unlock controls.
function UnlockRow({
  keypairState,
  account,
  onError,
}: {
  keypairState: EntityKeypairState
  account: Resource<Account>
  onError: (msg: string | null) => void
}) {
  const peerId = keypairState.keypair?.peerId ?? ''
  const method = keypairState.keypair?.authMethod ?? 'unknown'
  const unlocked = keypairState.unlocked ?? false
  const truncated = truncatePeerId(peerId)

  const cred = useCredentialProof()
  const [unlocking, setUnlocking] = useState(false)

  const needsPassword = method === 'password'

  const handleUnlock = useCallback(async () => {
    if (!account.value || !peerId || !cred.credential) return

    setUnlocking(true)
    onError(null)
    try {
      await account.value.unlockEntityKeypair(peerId, cred.credential)
      cred.reset()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      onError(mapAuthError(msg))
    } finally {
      setUnlocking(false)
    }
  }, [account.value, peerId, cred, onError])

  const handleLock = useCallback(async () => {
    if (!account.value || !peerId) return
    try {
      await account.value.lockEntityKeypair(peerId)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Lock failed'
      onError(msg)
    }
  }, [account.value, peerId, onError])

  const canUnlock = needsPassword ? !!cred.password : !!cred.pemData

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
          className="text-foreground-alt hover:text-foreground text-xs transition-colors"
        >
          Lock
        </button>
      </div>
    )
  }

  return (
    <div className="border-foreground/10 space-y-2 rounded-md border px-3 py-2">
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

      <div className="flex items-center gap-2">
        {needsPassword ?
          <input
            type="password"
            value={cred.password}
            onChange={(e) => cred.setPassword(e.target.value)}
            placeholder="Enter password"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canUnlock) void handleUnlock()
            }}
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-2.5 py-1.5 text-xs transition-colors outline-none',
              'focus:border-brand/50',
            )}
          />
        : <>
            <button
              type="button"
              onClick={() => cred.fileInputRef.current?.click()}
              className={cn(
                'border-foreground/20 bg-background/30 text-foreground w-full rounded-md border px-2.5 py-1.5 text-left text-xs transition-colors outline-none',
                'hover:border-foreground/30 focus:border-brand/50',
                'flex items-center gap-1.5',
              )}
            >
              <LuUpload className="text-foreground-alt h-3 w-3 shrink-0" />
              <span
                className={cn(!cred.pemFileName && 'text-foreground-alt/50')}
              >
                {cred.pemFileName ?? 'Choose .pem file'}
              </span>
            </button>
            <input
              ref={cred.fileInputRef}
              type="file"
              accept=".pem"
              onChange={cred.handleFileChange}
              className="hidden"
            />
          </>
        }
        <button
          onClick={() => void handleUnlock()}
          disabled={unlocking || !canUnlock}
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

// Step2Confirm renders the confirmation summary before execution.
function Step2Confirm({
  mode,
  addMethodType,
  removeMethodLabel,
  keypairs,
  unlockedCount,
  required,
}: {
  mode: WizardMode
  addMethodType?: AddMethodType
  removeMethodLabel?: string
  keypairs: EntityKeypairState[]
  unlockedCount: number
  required: number
}) {
  const ready = unlockedCount >= required
  const signers = keypairs.filter((kp) => kp.unlocked)

  return (
    <div className="space-y-3">
      <div
        className={cn(
          'rounded-md border p-3',
          ready ?
            'border-brand/20 bg-brand/5'
          : 'border-yellow-500/20 bg-yellow-500/5',
        )}
      >
        <div className="flex items-center gap-2">
          {ready ?
            <LuShieldCheck className="text-brand h-4 w-4 shrink-0" />
          : <LuLock className="h-4 w-4 shrink-0 text-yellow-500" />}
          <p className="text-foreground text-sm font-medium">
            {ready ?
              'Ready to execute'
            : `Need ${required - unlockedCount} more unlock${required - unlockedCount === 1 ? '' : 's'}`
            }
          </p>
        </div>

        {!ready && (
          <p className="text-foreground-alt mt-1 text-xs">
            Go back and unlock more keypairs to meet the threshold.
          </p>
        )}
      </div>

      <div>
        <p className="text-foreground-alt mb-1.5 text-xs font-medium">
          Operation
        </p>
        <p className="text-foreground text-sm">
          {mode === 'add' ?
            addMethodType === 'pem' ?
              'Generate and download a new backup key (.pem)'
            : 'Add a new auth method'
          : `Remove "${removeMethodLabel ?? 'auth method'}"`}
        </p>
      </div>

      {signers.length > 0 && (
        <div>
          <p className="text-foreground-alt mb-1.5 text-xs font-medium">
            Signing keypairs ({signers.length})
          </p>
          <div className="space-y-1">
            {signers.map((kp) => {
              const pid = kp.keypair?.peerId ?? ''
              const truncated = truncatePeerId(pid)
              return (
                <div key={pid} className="flex items-center gap-2 text-xs">
                  <LuCheck className="text-brand h-3 w-3 shrink-0" />
                  <span className="text-foreground">
                    {methodLabel(kp.keypair?.authMethod ?? '')}
                  </span>
                  <span className="text-foreground-alt font-mono">
                    {truncated}
                  </span>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

// CompleteView renders the success state.
function CompleteView({
  mode,
  addMethodType,
}: {
  mode: WizardMode
  addMethodType?: AddMethodType
}) {
  return (
    <div className="flex flex-col items-center gap-3 py-4">
      <div className="bg-brand/10 flex h-10 w-10 items-center justify-center rounded-full">
        <LuCheck className="text-brand h-5 w-5" />
      </div>
      <p className="text-foreground text-sm font-medium">
        {mode === 'add' ?
          addMethodType === 'pem' ?
            'Backup key generated and downloaded'
          : 'Auth method added'
        : 'Auth method removed'}
      </p>
      <p className="text-foreground-alt text-center text-xs">
        {mode === 'add' ?
          addMethodType === 'pem' ?
            'Store the .pem file in a safe location. You will need it to recover your account.'
          : 'The new auth method is now active on your account.'
        : 'The auth method has been permanently removed from your account.'}
      </p>
    </div>
  )
}
