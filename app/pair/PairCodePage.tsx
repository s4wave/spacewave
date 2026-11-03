import { keyframeNames } from '../../web/style/tokens.stylex.js'
/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  LuArrowLeft,
  LuCamera,
  LuCopy,
  LuLink,
  LuWifi,
  LuX,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { preparePairingSession } from './prepare-session.js'
import { resolvedPairingPeer } from './pairing-peer.js'
import { PairingVerificationStep } from '@s4wave/app/session/setup/PairingVerificationStep.js'
import { LinkDeviceDoneStep } from '@s4wave/app/session/setup/LinkDeviceDoneStep.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { pairingErrorMessage } from '@s4wave/app/session/setup/pairing-copy.js'
import { pairingStatusReachedPeer } from '@s4wave/app/session/pairing-status.js'
import type { Root } from '@s4wave/sdk/root'
import type { Session } from '@s4wave/sdk/session/session.js'
import { Html5Qrcode } from 'html5-qrcode'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule2: { textAlign: 'center' },
  rule3: {
    marginInline: 'auto',
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule6: { display: 'flex', justifyContent: 'center' },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    '--tw-outline-style': { default: null, ':focus': 'none' },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-bold)',
    letterSpacing: '.2em',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    opacity: { default: null, ':disabled': 0.5 },
    outlineStyle: { default: null, ':focus': 'none' },
    textAlign: 'center',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    height: `calc(${themeLayout['--spacing']} * 14)`,
    width: `calc(${themeLayout['--spacing']} * 48)`,
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    },
  },
  rule8: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule9: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'flex' },
  rule10: {
    '--tw-duration': '.3s',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
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
  },
  rule11: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule12: {
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
    width: '100%',
  },
  rule13: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    minHeight: 0,
    width: '100%',
  },
  rule14: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule15: { padding: 0 },
  rule16: {
    '--tw-outline-style': { default: null, ':focus': 'none' },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: { default: null, ':focus': 'none' },
    resize: 'none',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
    },
  },
  rule17: {
    '--tw-duration': '.3s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: '100%',
  },
  rule18: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    width: '100%',
  },
  rule20: {
    '--tw-outline-style': { default: null, ':focus': 'none' },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    WebkitUserSelect: 'all',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: { default: null, ':focus': 'none' },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    userSelect: 'all',
  },
  rule21: {
    '--tw-duration': '.3s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule22: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule23: {
    borderRadius: '3.40282e38px',
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '2s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['pulse'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'cubic-bezier(.4, 0, .6, 1)',
    backgroundColor: themeColors['--color-brand'],
    display: 'inline-block',
    height: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule24: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    inset: 0,
    alignItems: 'center',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 80%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    position: 'fixed',
    zIndex: 50,
  },
  rule25: {
    '--tw-shadow':
      '0 20px 25px -5px var(--tw-shadow-color, #0000001a), 0 8px 10px -6px var(--tw-shadow-color, #0000001a)',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule26: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule27: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule28: { borderRadius: '.25rem', overflow: 'hidden' },
  rule29: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: { color: themeColors['--color-brand'] },
  rule6: { color: themeColors['--color-foreground-alt'] },
})

type PairStep = 'enter' | 'direct' | 'verify' | 'done'
type DisposableResource = { [Symbol.dispose](): void }

export interface PairCodePageProps {
  // When provided, uses this session instead of creating one on submit.
  session?: Session | null
  // Where the back button navigates. Defaults to '/'.
  backPath?: string
  // Where to navigate after pairing completes. Defaults to '/u/{idx}'.
  donePath?: string
}

function usePairRouteCleanup(): RegisterCleanup {
  const resourcesRef = useRef<DisposableResource[]>([])
  const releasedRef = useRef(false)

  useEffect(() => {
    releasedRef.current = false
    return () => {
      releasedRef.current = true
      for (let i = resourcesRef.current.length - 1; i >= 0; i--) {
        resourcesRef.current[i][Symbol.dispose]()
      }
      resourcesRef.current = []
    }
  }, [])

  return useCallback((resource) => {
    if (!resource) return resource
    if (releasedRef.current) {
      resource[Symbol.dispose]()
      return resource
    }
    resourcesRef.current.push(resource)
    return resource
  }, [])
}

/** normalizePairCode strips a pairing code to at most 8 uppercase alphanumerics. */
function normalizePairCode(value: string): string {
  return value
    .replace(/[^A-Za-z0-9]/g, '')
    .toUpperCase()
    .slice(0, 8)
}

interface PairBackButtonProps {
  onClick: () => void
}

/** PairBackButton renders the icon-only back control of the pairing steps. */
function PairBackButton({ onClick }: PairBackButtonProps) {
  return (
    <button
      type="button"
      aria-label="Back"
      onClick={onClick}
      sx={[stylexStyles2.rule1]}
    >
      <LuArrowLeft sx={stylexStyles.rule1} />
    </button>
  )
}

interface PairCodeEnterStepProps {
  code: string
  loading: boolean
  error: string | null
  canSubmit: boolean
  onCodeChange: (code: string) => void
  onSubmit: () => void
  onBack: () => void
  onDirect: () => void
}

/** PairCodeEnterStep collects the 8-character pairing code shown on the other device. */
function PairCodeEnterStep({
  code,
  loading,
  error,
  canSubmit,
  onCodeChange,
  onSubmit,
  onBack,
  onDirect,
}: PairCodeEnterStepProps) {
  const formatted =
    code.length > 4 ? `${code.slice(0, 4)} ${code.slice(4)}` : code
  const ready = !loading && code.length === 8 && canSubmit

  const handlePaste = useCallback(
    (e: React.ClipboardEvent) => {
      e.preventDefault()
      onCodeChange(normalizePairCode(e.clipboardData.getData('text')))
    },
    [onCodeChange],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && ready) onSubmit()
    },
    [ready, onSubmit],
  )

  const handleCodeInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <div sx={false}>
      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule2]}>
        <div sx={stylexStyles2.rule3}>
          <LuLink sx={stylexStyles.rule2} />
        </div>
        <h2 sx={stylexStyles2.rule4}>Pair another device</h2>
        <p sx={stylexStyles2.rule5}>
          Enter the 8-character code shown on your other device.
        </p>
      </div>

      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule6]}>
        <input
          ref={handleCodeInputRef}
          aria-label="Pairing code"
          type="text"
          value={formatted}
          onChange={(e) => onCodeChange(normalizePairCode(e.target.value))}
          onPaste={handlePaste}
          onKeyDown={handleKeyDown}
          placeholder="XXXX XXXX"
          maxLength={9}
          disabled={loading}
          sx={[stylexStyles2.rule7]}
        />
      </div>

      {error && (
        <p sx={[nativeChildSpacing.s4, stylexStyles2.rule8]}>{error}</p>
      )}

      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule9]}>
        <PairBackButton onClick={onBack} />
        <button
          type="button"
          onClick={onSubmit}
          disabled={loading || code.length < 8 || !canSubmit}
          sx={[stylexStyles2.rule10]}
        >
          {loading ? (
            <Spinner />
          ) : (
            <>
              <LuLink sx={stylexStyles.rule3} />
              <span sx={stylexStyles2.rule11}>Connect</span>
            </>
          )}
        </button>
      </div>
      <button
        type="button"
        onClick={onDirect}
        disabled={loading}
        sx={[nativeChildSpacing.s4, stylexStyles2.rule12]}
      >
        Use a QR code or link
      </button>
    </div>
  )
}

// PairCodePage handles device pairing code entry and verification.
// Two modes: top-level (#/pair/:code) creates a session on submit,
// session-scoped (#/u/N/pair) uses the already-mounted session.
export function PairCodePage(props: PairCodePageProps) {
  const providedSession = props.session ?? null
  const params = useParams()
  const rawCode = params.code ?? ''
  const isDirectMode = rawCode === 'direct' || rawCode.length > 8
  const initialCode = isDirectMode ? '' : normalizePairCode(rawCode)
  const initialOfferPayload = rawCode.length > 8 ? rawCode : undefined
  const navigate = useNavigate()

  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const registerCleanup = usePairRouteCleanup()

  const [step, setStep] = useState<PairStep>(isDirectMode ? 'direct' : 'enter')
  const [code, setCode] = useState(initialCode)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [remotePeerId, setRemotePeerId] = useState<string | null>(null)
  const [currentSession, setCurrentSession] = useState<Session | null>(
    providedSession,
  )
  const sessionRef = useRef<Session | null>(providedSession)

  // Keep sessionRef in sync when prop changes.
  useEffect(() => {
    if (!providedSession) return
    sessionRef.current = providedSession
    const timer = window.setTimeout(() => setCurrentSession(providedSession), 0)
    return () => window.clearTimeout(timer)
  }, [providedSession])

  const handleSubmit = useCallback(async () => {
    if (code.length < 8) return
    setLoading(true)
    setError(null)
    try {
      let session = sessionRef.current
      const controller = new AbortController()
      registerCleanup({ [Symbol.dispose]: () => controller.abort() })
      if (!session) {
        // No session provided, create one (top-level route).
        if (!root) return
        session = await preparePairingSession(
          root,
          controller.signal,
          registerCleanup,
        )
        sessionRef.current = session
        setCurrentSession(session)
      }
      const peerId = await session.completePairing(
        {
          code,
          offerCurrentAccount: providedSession != null,
          remotePeerId: resolvedPairingPeer(code),
        },
        controller.signal,
      )
      if (peerId) {
        setRemotePeerId(peerId)
        setStep('verify')
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to complete pairing'
      setError(pairingErrorMessage(message))
    } finally {
      setLoading(false)
    }
  }, [root, code, registerCleanup, providedSession])

  const handleBack = useCallback(() => {
    navigate({ path: props.backPath ?? '/' })
  }, [navigate, props.backPath])

  const handleDone = useCallback(
    (entry?: SessionListEntry) => {
      navigate({
        path:
          entry?.sessionIndex != null
            ? `/u/${entry.sessionIndex}`
            : (props.donePath ?? '/'),
      })
    },
    [navigate, props.donePath],
  )

  return (
    <div sx={stylexStyles2.rule13}>
      <div sx={stylexStyles2.rule14}>
        <div sx={stylexStyles2.rule15}>
          {step === 'enter' && (
            <PairCodeEnterStep
              code={code}
              loading={loading}
              error={error}
              canSubmit={!!providedSession || !!root}
              onCodeChange={setCode}
              onSubmit={() => void handleSubmit()}
              onBack={handleBack}
              onDirect={() => setStep('direct')}
            />
          )}

          {step === 'direct' && (
            <PairDirectStep
              initialOfferPayload={initialOfferPayload}
              session={currentSession}
              offerCurrentAccount={providedSession != null}
              root={root ?? null}
              registerCleanup={registerCleanup}
              onSessionCreated={(sess) => {
                sessionRef.current = sess
                setCurrentSession(sess)
              }}
              onRemotePeerResolved={(peerId) => {
                setRemotePeerId(peerId)
                setStep('verify')
              }}
              onBack={handleBack}
            />
          )}

          {step === 'verify' && (
            <PairingVerificationStep
              session={currentSession}
              onContinue={() => setStep('done')}
              onAbort={() => {
                setRemotePeerId(null)
                setStep(isDirectMode ? 'direct' : 'enter')
              }}
            />
          )}

          {step === 'done' && (
            <LinkDeviceDoneStep
              session={currentSession}
              remotePeerId={remotePeerId}
              onDone={handleDone}
              onLinkMore={(entry) => {
                if (entry?.sessionIndex != null) {
                  navigate({
                    path: `/u/${entry.sessionIndex}/setup/link-device`,
                  })
                } else {
                  setStep('enter')
                }
              }}
            />
          )}
        </div>
      </div>
    </div>
  )
}

// -- Direct pairing (no-cloud WebRTC answerer) --

interface PairDirectStepProps {
  initialOfferPayload?: string
  session: Session | null
  offerCurrentAccount: boolean
  root: Root | null
  registerCleanup: RegisterCleanup
  onSessionCreated: (session: Session) => void
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// PairDirectStep handles the no-cloud answerer flow: accepts an offer (via
// paste or QR scan), auto-creates a local session if needed, calls
// acceptLocalPairingOffer, and displays the answer payload for the offerer.
function PairDirectStep({
  initialOfferPayload,
  session,
  offerCurrentAccount,
  root,
  registerCleanup,
  onSessionCreated,
  onRemotePeerResolved,
  onBack,
}: PairDirectStepProps) {
  const [offerInput, setOfferInput] = useState(initialOfferPayload ?? '')
  const [answerPayload, setAnswerPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)
  const [copied, setCopied] = useState(false)
  const sessionRef = useRef<Session | null>(session)

  useEffect(() => {
    if (!session) return
    sessionRef.current = session
  }, [session])

  const ensureSession = useCallback(async (): Promise<Session | null> => {
    if (sessionRef.current) return sessionRef.current
    if (!root) return null
    const controller = new AbortController()
    registerCleanup({ [Symbol.dispose]: () => controller.abort() })
    const session = await preparePairingSession(
      root,
      controller.signal,
      registerCleanup,
    )
    sessionRef.current = session
    onSessionCreated(session)
    return session
  }, [root, registerCleanup, onSessionCreated])

  const handleAcceptOffer = useCallback(
    async (payload: string) => {
      if (!payload.trim()) return
      setLoading(true)
      setError(null)
      try {
        const sess = await ensureSession()
        if (!sess) return
        const controller = new AbortController()
        registerCleanup({ [Symbol.dispose]: () => controller.abort() })
        const resp = await sess.acceptLocalPairingOffer(
          extractDirectOfferPayload(payload.trim()),
          offerCurrentAccount,
          controller.signal,
        )
        setAnswerPayload(resp.answerPayload ?? null)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to accept offer')
      } finally {
        setLoading(false)
      }
    },
    [ensureSession, registerCleanup, offerCurrentAccount],
  )

  // Auto-accept if initial offer payload was provided via URL.
  const autoAccepted = useRef(false)
  useEffect(() => {
    if (initialOfferPayload && !autoAccepted.current) {
      autoAccepted.current = true
      void handleAcceptOffer(initialOfferPayload)
    }
  }, [initialOfferPayload, handleAcceptOffer])

  const handleSubmit = useCallback(() => {
    void handleAcceptOffer(offerInput)
  }, [handleAcceptOffer, offerInput])

  const handleQRScanned = useCallback(
    (decoded: string) => {
      setScanning(false)
      const payload = extractDirectOfferPayload(decoded)
      setOfferInput(payload)
      void handleAcceptOffer(payload)
    },
    [handleAcceptOffer],
  )

  const handleCopy = useCallback(() => {
    if (!answerPayload) return
    void navigator.clipboard.writeText(answerPayload)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [answerPayload])

  // Watch pairing status for peer connection after answer is shared.
  useEffect(() => {
    const sess = sessionRef.current
    if (!sess || !answerPayload) return
    const controller = new AbortController()
    ;(async () => {
      for await (const resp of sess.watchPairingStatus(controller.signal)) {
        if (controller.signal.aborted) break
        if (pairingStatusReachedPeer(resp.status) && resp.remotePeerId) {
          onRemotePeerResolved(resp.remotePeerId)
          break
        }
      }
    })().catch((err) => {
      if (controller.signal.aborted) return
      setError(err instanceof Error ? err.message : 'Direct connection failed')
    })
    return () => controller.abort()
  }, [answerPayload, onRemotePeerResolved])

  return (
    <div sx={false}>
      {scanning && (
        <PairDirectQRScanner
          sx={nativeChildSpacingNext.s4}
          onScanned={handleQRScanned}
          onClose={() => setScanning(false)}
        />
      )}

      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule2]}>
        <div sx={stylexStyles2.rule3}>
          <LuWifi sx={stylexStyles.rule2} />
        </div>
        <h2 sx={stylexStyles2.rule4}>
          {answerPayload ? 'Share this response' : 'Direct pairing'}
        </h2>
        <p sx={stylexStyles2.rule5}>
          {answerPayload
            ? 'Copy this response and paste it on the other device.'
            : 'Scan the QR code or paste the offer from the other device.'}
        </p>
      </div>

      {!answerPayload && (
        <>
          <textarea
            aria-label="Offer payload"
            value={offerInput}
            onChange={(e) => setOfferInput(e.target.value)}
            placeholder="Paste offer payload here…"
            rows={3}
            sx={[nativeChildSpacingNext.s4, [stylexStyles2.rule16]]}
          />

          <button
            type="button"
            onClick={() => setScanning(true)}
            sx={[nativeChildSpacingNext.s4, [stylexStyles2.rule17]]}
          >
            <LuCamera sx={stylexStyles.rule1} />
            <span sx={stylexStyles2.rule18}>Scan QR code</span>
          </button>
        </>
      )}

      {answerPayload && (
        <div sx={[nativeChildSpacingNext.s4, false]}>
          <div sx={[nativeChildSpacing.s2, stylexStyles2.rule19]}>
            <input
              readOnly
              aria-label="Answer payload"
              value={answerPayload}
              sx={[stylexStyles2.rule20]}
              onClick={(e) => (e.target as HTMLInputElement).select()}
            />
            <button
              type="button"
              aria-label="Copy to clipboard"
              onClick={handleCopy}
              sx={[stylexStyles2.rule21]}
              title="Copy to clipboard"
            >
              <LuCopy
                sx={[
                  stylexStyles.rule4,
                  copied ? stylexStyles.rule5 : stylexStyles.rule6,
                ]}
              />
            </button>
          </div>
          <div sx={[nativeChildSpacing.s2, stylexStyles2.rule22]}>
            <span sx={stylexStyles2.rule23} />
            <span sx={stylexStyles2.rule18}>Waiting for connection…</span>
          </div>
        </div>
      )}

      {error && (
        <p sx={[nativeChildSpacingNext.s4, stylexStyles2.rule8]}>{error}</p>
      )}

      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule9]}>
        <PairBackButton onClick={onBack} />
        {!answerPayload && (
          <button
            type="button"
            onClick={handleSubmit}
            disabled={loading || !offerInput.trim() || (!session && !root)}
            sx={[stylexStyles2.rule10]}
          >
            {loading ? (
              <Spinner />
            ) : (
              <>
                <LuLink sx={stylexStyles.rule3} />
                <span sx={stylexStyles2.rule11}>Accept offer</span>
              </>
            )}
          </button>
        )}
      </div>
    </div>
  )
}

// extractDirectOfferPayload accepts a pairing URL or its encoded offer.
function extractDirectOfferPayload(decoded: string): string {
  const match = decoded.match(/#\/pair\/(.+)/)
  if (match) return match[1]
  return decoded
}

// PairDirectQRScanner renders a camera-based QR scanner for direct pairing payloads.
function PairDirectQRScanner({
  sx,
  onScanned,
  onClose,
}: {
  onScanned: (payload: string) => void
  onClose: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return
    const scanner = new Html5Qrcode(containerRef.current.id)
    scanner
      .start(
        { facingMode: 'environment' },
        { fps: 10, qrbox: { width: 250, height: 250 } },
        (decoded) => {
          scanner.stop().catch(() => {})
          onScanned(decoded)
        },
        () => {},
      )
      .catch(() => {})
    return () => {
      scanner.stop().catch(() => {})
    }
  }, [onScanned])

  return (
    <div sx={[stylexStyles2.rule24, sx]}>
      <div sx={stylexStyles2.rule25}>
        <div sx={stylexStyles2.rule26}>
          <h3 sx={stylexStyles2.rule4}>Scan direct pairing QR</h3>
          <button
            type="button"
            aria-label="Close scanner"
            onClick={onClose}
            sx={stylexStyles2.rule27}
          >
            <LuX sx={stylexStyles.rule4} />
          </button>
        </div>
        <div
          id="pair-direct-qr-scanner"
          ref={containerRef}
          sx={stylexStyles2.rule28}
        />
        <p sx={stylexStyles2.rule29}>
          Point your camera at the QR code on the other device.
        </p>
      </div>
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
