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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 6)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-outline-style': 'none',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    outlineStyle: 'none',
    position: 'relative',
    overflowY: 'auto',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-brand'],
    fontSize: '9px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule8: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
  },
  rule9: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    position: 'relative',
  },
  rule10: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule11: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule13: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    justifyContent: 'center',
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
      sx={stylexStyles3.rule1}
      onKeyDown={handleKeyDown}
      tabIndex={-1}
    >
      <BackButton floating onClick={() => navigate({ path: '/sessions' })}>
        Sessions
      </BackButton>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <AnimatedLogo followMouse={false} />
          <h1 sx={stylexStyles3.rule4}>Session Expired</h1>
          {entityId && (
            <div sx={stylexStyles3.rule5}>
              <span sx={stylexStyles3.rule6}>{entityId}</span>
              <span sx={stylexStyles3.rule7}>{providerLabel}</span>
            </div>
          )}
          <p sx={stylexStyles3.rule8}>
            Enter your password or upload a backup key to reconnect.
          </p>
        </div>

        <div sx={stylexStyles3.rule9}>
          <div sx={stylexStyles4.rule1}>
            <CredentialProofInput
              sx={nativeChildSpacing.s4}
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
              sx={[nativeChildSpacing.s4, [[stylexStyles2.rule1, groupMarker]]]}
            >
              <LuKeyRound sx={stylexStyles.rule1} />
              <span sx={stylexStyles3.rule10}>
                {submitting ? 'Reconnecting…' : 'Reconnect'}
              </span>
            </button>

            <button
              type="button"
              onClick={() => navigate({ path: '/recover' })}
              sx={[nativeChildSpacing.s4, stylexStyles3.rule11]}
            >
              Forgot password?
            </button>

            <div sx={[nativeChildSpacing.s4, stylexStyles3.rule12]}>
              <button
                type="button"
                onClick={() => setLogoutOpen(true)}
                sx={stylexStyles3.rule13}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
})
