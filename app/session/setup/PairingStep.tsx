import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { LuLink, LuRefreshCw } from '@s4wave/web/images/icons.js'
import QRCode from 'qrcode'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'
import { PairingCodeChip } from './PairingCodeChip.js'
import {
  PAIRING_CODE_INSTRUCTIONS,
  pairingErrorMessage,
} from './pairing-copy.js'
import { usePairingWatch } from './usePairingWatch.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 40)`,
    height: `calc(${themeLayout['--spacing']} * 40)`,
    borderRadius: '.25rem',
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    width: '100%',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule5: { textAlign: 'center' },
  rule6: {
    marginInline: 'auto',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule8: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 16)`,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule12: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule13: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule14: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule15: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule16: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule17: {
    display: 'inline-block',
    width: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 2)`,
    animationName: 'pulse',
    animationDuration: '2s',
    animationTimingFunction: 'cubic-bezier(.4, 0, .6, 1)',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
  },
  rule18: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: { color: themeColors['--color-destructive'] },
  rule20: { color: themeColors['--color-foreground-alt'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

const CODE_TTL_SECONDS = 600

// PairingQRCode renders a QR code encoding the deep link URL for the pairing code.
function PairingQRCode({ code }: { code: string }) {
  const qrCallback = useCallback(() => {
    if (!code) return undefined
    const url = `${SPACEWAVE_PUBLIC_BASE_URL}/#/pair/${code}`
    return QRCode.toDataURL(url, {
      width: 160,
      margin: 1,
      color: { dark: '#ffffff', light: '#00000000' },
    })
  }, [code])
  const qrResult = usePromise(qrCallback)

  if (!qrResult.data) return null

  return (
    <div sx={stylexStyles2.rule1}>
      <img src={qrResult.data} alt="Pairing QR code" sx={stylexStyles2.rule2} />
      <span sx={stylexStyles2.rule3}>Or scan this QR code</span>
    </div>
  )
}

// useCodeCountdown counts down the pairing code lifetime from the moment the
// code arrives, and calls onExpired once when it reaches zero.
function useCodeCountdown(code: string | null, onExpired: () => void) {
  const [elapsed, setElapsed] = useState(0)

  useEffect(() => {
    if (!code) return
    const interval = setInterval(() => {
      setElapsed((e) => e + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [code])

  const secondsLeft = code
    ? Math.max(0, CODE_TTL_SECONDS - elapsed)
    : CODE_TTL_SECONDS

  const handleExpired = useEffectEvent(onExpired)
  const prevSecondsLeft = useRef(secondsLeft)
  useEffect(() => {
    if (prevSecondsLeft.current > 0 && secondsLeft === 0 && code) {
      handleExpired()
    }
    prevSecondsLeft.current = secondsLeft
  }, [secondsLeft, code])

  const mins = Math.floor(secondsLeft / 60)
  const secs = secondsLeft % 60
  return {
    secondsLeft,
    countdown: `${mins}:${secs.toString().padStart(2, '0')}`,
  }
}

export interface PairingStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// PairingStep shows a pairing code for the other device to enter. Generating a
// new code remounts the code view, so each code starts with a fresh countdown
// and connection state.
export function PairingStep({
  session,
  onRemotePeerResolved,
  onBack,
}: PairingStepProps) {
  const [generation, setGeneration] = useState(0)

  return (
    <PairingCodeView
      key={generation}
      session={session}
      onRegenerateCode={() => setGeneration((n) => n + 1)}
      onRemotePeerResolved={onRemotePeerResolved}
      onBack={onBack}
    />
  )
}

interface PairingCodeViewProps extends PairingStepProps {
  onRegenerateCode: () => void
}

// PairingCodeView generates one pairing code and watches for the remote device
// to connect with it.
function PairingCodeView({
  session,
  onRegenerateCode,
  onRemotePeerResolved,
  onBack,
}: PairingCodeViewProps) {
  const generateCode = useCallback(
    (signal: AbortSignal) => session?.generatePairingCode(signal),
    [session],
  )
  const codeResult = usePromise(generateCode)
  const code = codeResult.data ?? null
  const loading = codeResult.loading
  const [connectionError, setConnectionError] = useState<string | null>(null)

  usePairingWatch({
    session,
    enabled: !!code,
    reportFailures: true,
    streamFailure: 'Pairing status stream failed',
    onPeerResolved: onRemotePeerResolved,
    onFailure: setConnectionError,
  })
  const { secondsLeft, countdown } = useCodeCountdown(code, onRegenerateCode)

  const instructions = PAIRING_CODE_INSTRUCTIONS

  return (
    <div sx={stylexStyles2.rule4}>
      <div sx={stylexStyles2.rule5}>
        <div sx={stylexStyles2.rule6}>
          <LuLink sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles2.rule7}>Link My Device</h2>
        <p sx={stylexStyles2.rule8}>{instructions.heading}</p>
        <p sx={stylexStyles2.rule8}>{instructions.hint}</p>
      </div>

      <div sx={stylexStyles2.rule9}>
        {loading && <Spinner size="lg" variant="muted" />}
        {!loading && code && (
          <>
            <PairingCodeChip code={code} />
            <PairingQRCode code={code} />
          </>
        )}
      </div>

      {connectionError && (
        <div sx={stylexStyles2.rule10}>
          <p sx={stylexStyles2.rule11}>{connectionError}</p>
          <button
            type="button"
            onClick={onRegenerateCode}
            sx={[stylexStyles2.rule12]}
          >
            Retry
          </button>
        </div>
      )}

      {codeResult.error && !connectionError && (
        <p sx={stylexStyles2.rule11}>
          {pairingErrorMessage(codeResult.error.message)}
        </p>
      )}

      <div sx={stylexStyles2.rule13}>
        <LinkDeviceBackButton onClick={onBack} />
        <button
          type="button"
          onClick={onRegenerateCode}
          disabled={loading}
          sx={[stylexStyles2.rule14]}
        >
          <LuRefreshCw sx={stylexStyles.rule2} />
          <span sx={stylexStyles2.rule15}>Generate new code</span>
        </button>
      </div>
      {!loading && code && !connectionError && (
        <WaitingForConnection secondsLeft={secondsLeft} countdown={countdown} />
      )}
    </div>
  )
}

// WaitingForConnection shows the pending connection and the code expiry.
function WaitingForConnection({
  secondsLeft,
  countdown,
}: {
  secondsLeft: number
  countdown: string
}) {
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule16}>
        <span sx={stylexStyles2.rule17} />
        <span sx={stylexStyles2.rule3}>Waiting for connection…</span>
      </div>
      <span
        sx={[
          stylexStyles2.rule18,
          secondsLeft <= 60 ? stylexStyles2.rule19 : stylexStyles2.rule20,
        ]}
      >
        Code expires in {countdown}
      </span>
    </div>
  )
}
