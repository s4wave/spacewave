import { useCallback, useId, useState } from 'react'
import { LuLock, LuLockOpen } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import {
  SessionRecoveryState,
  type EntityCredential,
  type SessionMetadata,
} from '@s4wave/core/session/session.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
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
  rule5: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    '--tw-outline-style': 'none',
    '--tw-tracking': 'var(--tracking-widest)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    opacity: { default: null, ':disabled': 0.5 },
    outlineStyle: 'none',
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule10: {
    '--tw-outline-style': 'none',
    '--tw-tracking': 'var(--tracking-widest)',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    opacity: { default: null, ':disabled': 0.5 },
    outlineStyle: 'none',
    textAlign: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule11: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule12: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule13: {
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
  rule14: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
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
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
  const pinInputId = useId()
  const handlePinInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])
  const recoveryUnavailable =
    metadata.providerId === 'local' &&
    metadata.recoveryState === SessionRecoveryState.UNAVAILABLE

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
      role="group"
      data-testid="pin-unlock-overlay"
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
        </div>

        <div sx={stylexStyles3.rule4}>
          <div sx={stylexStyles4.rule1}>
            <div sx={[nativeChildSpacingNext.s4, stylexStyles3.rule3]}>
              <div sx={stylexStyles3.rule5}>
                <LuLock sx={stylexStyles.rule1} />
              </div>
              <h2 sx={stylexStyles3.rule6}>
                {metadata.displayName || 'Locked Session'}
              </h2>
              {metadata.providerDisplayName && (
                <p sx={stylexStyles3.rule7}>{metadata.providerDisplayName}</p>
              )}
            </div>

            {!showRecovery ? (
              <>
                <div sx={nativeChildSpacingNext.s4}>
                  <label htmlFor={pinInputId} sx={stylexStyles3.rule8}>
                    Enter PIN to unlock
                  </label>
                  <input
                    ref={handlePinInputRef}
                    id={pinInputId}
                    data-testid="pin-unlock-input"
                    type="password"
                    value={pin}
                    onChange={(e) => setPin(e.target.value)}
                    placeholder="PIN"
                    disabled={unlocking}
                    sx={pinInputStyles(error)}
                  />
                </div>

                {error && (
                  <p sx={[nativeChildSpacingNext.s4, stylexStyles3.rule11]}>
                    {error}
                  </p>
                )}

                <button
                  type="button"
                  data-testid="pin-unlock-submit"
                  onClick={() => void handleUnlock()}
                  disabled={unlocking || pin.length === 0}
                  sx={[
                    nativeChildSpacingNext.s4,
                    [[stylexStyles2.rule1, groupMarker]],
                  ]}
                >
                  <LuLockOpen sx={stylexStyles.rule2} />
                  <span sx={stylexStyles3.rule12}>
                    {unlocking ? 'Unlocking…' : 'Unlock'}
                  </span>
                </button>

                <button
                  type="button"
                  onClick={() => setShowRecovery(true)}
                  sx={[nativeChildSpacingNext.s4, stylexStyles3.rule13]}
                >
                  Forgot PIN?
                </button>
              </>
            ) : recoveryUnavailable ? (
              <>
                <div sx={nativeChildSpacingNext.s4}>
                  <p sx={[nativeChildSpacing.s2, stylexStyles3.rule14]}>
                    This local session has no recovery password or backup key.
                  </p>
                  <p sx={[nativeChildSpacing.s2, stylexStyles3.rule14]}>
                    Your PIN is the only unlock method. Try the PIN again, or
                    return to Sessions and remove this local session to create a
                    new one. Local-only data that only this session can access
                    may not be recoverable.
                  </p>
                </div>

                <button
                  type="button"
                  onClick={() => setShowRecovery(false)}
                  sx={[nativeChildSpacingNext.s4, stylexStyles3.rule13]}
                >
                  Back to PIN entry
                </button>
              </>
            ) : (
              <>
                <p sx={[nativeChildSpacingNext.s4, stylexStyles3.rule14]}>
                  Re-authenticate with your account password or backup key to
                  reset this session. This generates a new session key.
                </p>

                <CredentialProofInput
                  sx={nativeChildSpacingNext.s4}
                  password={cred.password}
                  onPasswordChange={cred.setPassword}
                  pemFileName={cred.pemFileName}
                  onFileChange={cred.handleFileChange}
                  fileInputRef={cred.fileInputRef}
                  pemLabel="Backup key file"
                  error={recoveryError}
                  focusOnMount
                />

                <button
                  type="button"
                  onClick={() => void handleReset()}
                  disabled={resetting || !cred.hasCredential}
                  sx={[
                    nativeChildSpacingNext.s4,
                    [[stylexStyles2.rule1, groupMarker]],
                  ]}
                >
                  <span sx={stylexStyles3.rule12}>
                    {resetting ? 'Resetting…' : 'Reset session'}
                  </span>
                </button>

                <button
                  type="button"
                  onClick={() => setShowRecovery(false)}
                  sx={[nativeChildSpacingNext.s4, stylexStyles3.rule13]}
                >
                  Back to PIN entry
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// pinInputStyles keeps the complete invalid border and focus recipe together.
function pinInputStyles(error: string | null): stylex.StyleXStyles {
  return error ? stylexStyles3.rule9 : stylexStyles3.rule10
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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
