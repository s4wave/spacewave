import { useCallback, useState } from 'react'
import { LuLock, LuLockOpen } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import type {
  SessionMetadata,
  EntityCredential,
} from '@s4wave/core/session/session.pb.js'

export interface PinUnlockOverlayProps {
  metadata: SessionMetadata
  onUnlock: (pin: Uint8Array) => Promise<void>
  onReset: (sessionIdx: number, credential: EntityCredential) => Promise<void>
}

// PinUnlockOverlay renders the PIN unlock gate for PIN-locked sessions.
export function PinUnlockOverlay({
  metadata,
  onUnlock,
  onReset,
}: PinUnlockOverlayProps) {
  const sessionIdx = useSessionIndex()
  const navigate = useNavigate()
  const [pin, setPin] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [unlocking, setUnlocking] = useState(false)
  const [showRecovery, setShowRecovery] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [recoveryError, setRecoveryError] = useState<string | null>(null)
  const cred = useCredentialProof()

  const handleUnlock = useCallback(async () => {
    if (pin.length === 0) {
      setError('Please enter your PIN')
      return
    }
    setError(null)
    setUnlocking(true)
    try {
      await onUnlock(new TextEncoder().encode(pin))
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Wrong PIN. Please try again.',
      )
      setUnlocking(false)
    }
  }, [pin, onUnlock])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter') {
        void handleUnlock()
      }
    },
    [handleUnlock],
  )

  const handleReset = useCallback(async () => {
    if (!cred.hasCredential) return
    setResetting(true)
    setRecoveryError(null)
    try {
      const credential: EntityCredential = cred.credential!
      await onReset(sessionIdx, credential)
      // Reset succeeded. The session tracker will restart with a fresh key.
      // Navigate to sessions list so the user can re-enter.
      navigate({ path: '/sessions' })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Recovery failed'
      if (msg.includes('unknown_keypair')) {
        setRecoveryError('Incorrect password or unrecognized key.')
      } else {
        setRecoveryError(msg)
      }
    } finally {
      setResetting(false)
    }
  }, [cred.credential, cred.hasCredential, onReset, sessionIdx, navigate])

  return (
    <div
      className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10"
      onKeyDown={handleKeyDown}
      tabIndex={-1}
    >
      <BackButton floating onClick={() => navigate({ path: '/sessions' })}>
        Sessions
      </BackButton>

      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />

      <div className="relative z-10 flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            <div className="flex flex-col items-center gap-2">
              <div className="bg-brand/10 flex h-12 w-12 items-center justify-center rounded-full">
                <LuLock className="text-brand h-6 w-6" />
              </div>
              <h2 className="text-foreground text-lg font-medium">
                {metadata.displayName || 'Locked Session'}
              </h2>
              {metadata.providerDisplayName && (
                <p className="text-foreground-alt text-xs">
                  {metadata.providerDisplayName}
                </p>
              )}
            </div>

            {!showRecovery ?
              <>
                <div>
                  <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                    Enter PIN to unlock
                  </label>
                  <input
                    type="password"
                    value={pin}
                    onChange={(e) => setPin(e.target.value)}
                    placeholder="PIN"
                    autoFocus
                    disabled={unlocking}
                    className={cn(
                      'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-center text-lg tracking-widest transition-colors outline-none',
                      'focus:border-brand/50',
                      error && 'border-destructive/50',
                      'disabled:opacity-50',
                    )}
                  />
                </div>

                {error && (
                  <p className="text-destructive text-center text-xs">
                    {error}
                  </p>
                )}

                <button
                  onClick={() => void handleUnlock()}
                  disabled={unlocking || pin.length === 0}
                  className={cn(
                    'group w-full rounded-md border transition-all duration-300',
                    'border-brand/30 bg-brand/10 hover:bg-brand/20',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                    'flex h-10 items-center justify-center gap-2',
                  )}
                >
                  <LuLockOpen className="h-4 w-4" />
                  <span className="text-foreground text-sm">
                    {unlocking ? 'Unlocking...' : 'Unlock'}
                  </span>
                </button>

                <button
                  onClick={() => setShowRecovery(true)}
                  className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
                >
                  Forgot PIN?
                </button>
              </>
            : <>
                <p className="text-foreground-alt text-xs leading-relaxed">
                  Re-authenticate with your account password or backup key to
                  reset this session. This generates a new session key.
                </p>

                <CredentialProofInput
                  password={cred.password}
                  onPasswordChange={cred.setPassword}
                  pemFileName={cred.pemFileName}
                  onFileChange={cred.handleFileChange}
                  fileInputRef={cred.fileInputRef}
                  pemLabel="Backup key file"
                  error={recoveryError}
                  autoFocus
                />

                <button
                  onClick={() => void handleReset()}
                  disabled={resetting || !cred.hasCredential}
                  className={cn(
                    'group w-full rounded-md border transition-all duration-300',
                    'border-brand/30 bg-brand/10 hover:bg-brand/20',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                    'flex h-10 items-center justify-center gap-2',
                  )}
                >
                  <span className="text-foreground text-sm">
                    {resetting ? 'Resetting...' : 'Reset session'}
                  </span>
                </button>

                <button
                  onClick={() => setShowRecovery(false)}
                  className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
                >
                  Back to PIN entry
                </button>
              </>
            }
          </div>
        </div>
      </div>
    </div>
  )
}
