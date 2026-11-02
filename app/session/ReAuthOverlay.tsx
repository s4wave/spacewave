import { useCallback, useState } from 'react'
import { LuKeyRound, LuLogOut } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'

import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import type { ReauthenticateSessionRequest } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { LogoutConfirmDialog } from './LogoutConfirmDialog.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
      role="group"
      className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10"
      onKeyDown={handleKeyDown}
      tabIndex={-1}
    >
      <BackButton floating onClick={() => navigate({ path: '/sessions' })}>
        Sessions
      </BackButton>
      <div className="relative z-10 flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="text-foreground mt-2 text-xl font-semibold tracking-wide">
            Session Expired
          </h1>
          {entityId && (
            <div className="flex items-center gap-2">
              <span className="text-foreground text-sm font-medium">
                {entityId}
              </span>
              <span className="bg-brand/15 text-brand micro-nine rounded-full px-1.5 py-0.5 font-semibold tracking-wider uppercase">
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
              focusOnMount
            />

            <button
              type="button"
              onClick={() => void handleSubmit()}
              disabled={submitting || !cred.hasCredential}
              sx={[[stylexStyles2.rule1, groupMarker]]}
            >
              <LuKeyRound sx={stylexStyles.rule1} />
              <span className="text-foreground text-sm">
                {submitting ? 'Reconnecting…' : 'Reconnect'}
              </span>
            </button>

            <button
              type="button"
              onClick={() => navigate({ path: '/recover' })}
              className="text-foreground-alt hover:text-foreground w-full text-center text-xs transition-colors"
            >
              Forgot password?
            </button>

            <div className="border-foreground/10 border-t pt-4">
              <button
                type="button"
                onClick={() => setLogoutOpen(true)}
                className="text-destructive/70 hover:text-destructive flex w-full items-center justify-center gap-2 text-center text-xs transition-colors"
              >
                <LuLogOut sx={stylexStyles.rule2} />
                Log out
              </button>
            </div>
          </div>
        </div>
      </div>

      <LogoutConfirmDialog
        open={logoutOpen}
        onOpenChange={setLogoutOpen}
        loggingOut={loggingOut}
        onConfirm={() => void handleLogout()}
      />
    </div>
  )
}
