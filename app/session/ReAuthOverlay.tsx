import { useCallback, useState } from 'react'
import { LuKeyRound, LuLogOut } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import type { ReauthenticateSessionRequest } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

export interface ReAuthOverlayProps {
  metadata: SessionMetadata
  onReauth: (request: ReauthenticateSessionRequest) => Promise<void>
  onLogout: () => Promise<void>
}

// ReAuthOverlay renders a full-page re-login gate for sessions with stale credentials.
export function ReAuthOverlay({
  metadata,
  onReauth,
  onLogout,
}: ReAuthOverlayProps) {
  const sessionIdx = useSessionIndex()
  const navigate = useNavigate()
  const cred = useCredentialProof()
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [logoutOpen, setLogoutOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)

  const handleLogout = useCallback(async () => {
    setLoggingOut(true)
    await onLogout()
    navigate({ path: '/sessions' })
  }, [onLogout, navigate])

  const handleSubmit = useCallback(async () => {
    if (!cred.hasCredential) return
    setSubmitting(true)
    setError(null)
    try {
      const request: ReauthenticateSessionRequest = {
        sessionIndex: sessionIdx,
        entityId: metadata.displayName ?? '',
      }
      if (cred.pemData) {
        request.credential = { case: 'pem', value: { pemData: cred.pemData } }
      } else {
        request.credential = {
          case: 'password',
          value: { password: cred.password },
        }
      }
      await onReauth(request)
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'Re-authentication failed'
      if (msg.includes('unknown_keypair')) {
        setError('Incorrect password or unrecognized key.')
      } else {
        setError(msg)
      }
    } finally {
      setSubmitting(false)
    }
  }, [
    cred.hasCredential,
    cred.pemData,
    cred.password,
    sessionIdx,
    metadata.displayName,
    onReauth,
  ])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter') {
        void handleSubmit()
      }
    },
    [handleSubmit],
  )

  const entityId = metadata.displayName ?? ''
  const providerLabel = metadata.providerDisplayName || 'Cloud'

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
          <h1 className="text-foreground mt-2 text-xl font-bold tracking-wide">
            Session Expired
          </h1>
          {entityId && (
            <div className="flex items-center gap-2">
              <span className="text-foreground text-sm font-medium">
                {entityId}
              </span>
              <span className="bg-brand/15 text-brand rounded-full px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase">
                {providerLabel}
              </span>
            </div>
          )}
          <p className="text-foreground-alt text-center text-sm">
            Enter your password or upload a backup key to reconnect.
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            <CredentialProofInput
              password={cred.password}
              onPasswordChange={cred.setPassword}
              pemFileName={cred.pemFileName}
              onFileChange={cred.handleFileChange}
              fileInputRef={cred.fileInputRef}
              pemLabel="Backup key file"
              error={error}
              disabled={submitting}
              autoFocus
            />

            <button
              onClick={() => void handleSubmit()}
              disabled={submitting || !cred.hasCredential}
              className={cn(
                'group w-full rounded-md border transition-all duration-300',
                'border-brand/30 bg-brand/10 hover:bg-brand/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
                'flex h-10 items-center justify-center gap-2',
              )}
            >
              <LuKeyRound className="h-4 w-4" />
              <span className="text-foreground text-sm">
                {submitting ? 'Reconnecting...' : 'Reconnect'}
              </span>
            </button>

            <button
              onClick={() => navigate({ path: '/recover' })}
              className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
            >
              Forgot password?
            </button>

            <div className="border-foreground/10 border-t pt-4">
              <button
                onClick={() => setLogoutOpen(true)}
                className="text-destructive/70 hover:text-destructive flex w-full items-center justify-center gap-2 text-center text-xs transition-colors"
              >
                <LuLogOut className="h-3 w-3" />
                Log out
              </button>
            </div>
          </div>
        </div>
      </div>

      <Dialog open={logoutOpen} onOpenChange={setLogoutOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Log out of session?</DialogTitle>
            <DialogDescription>
              This will remove the session from your device. You will need your
              password or backup key to sign in again.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              onClick={() => setLogoutOpen(false)}
              disabled={loggingOut}
              className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => void handleLogout()}
              disabled={loggingOut}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-destructive/30 bg-destructive/10 hover:bg-destructive/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              {loggingOut ? 'Logging out...' : 'Log out'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
