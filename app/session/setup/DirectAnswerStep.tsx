import { keyframeNames } from '../../../web/style/tokens.stylex.js'
import { useState } from 'react'
import { LuCamera, LuLink } from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { CopyablePayloadField } from './CopyablePayloadField.js'
import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'
import { QRScannerModal } from './QRScannerModal.js'
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
  rule1: { textAlign: 'center' },
  rule2: {
    marginInline: 'auto',
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule5: {
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
  rule6: {
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
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule9: {
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
  rule10: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule11: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'flex' },
  rule12: {
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
  rule13: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// extractDirectOfferPayload extracts the raw b58 offer payload from a QR-decoded
// string. The QR may encode a full URL (https://spacewave.app/#/pair/<payload>)
// or just the raw payload. Returns the payload either way.
function extractDirectOfferPayload(decoded: string): string {
  const match = decoded.match(/#\/pair\/(.+)/)
  if (match) return match[1]
  return decoded
}

export interface DirectAnswerStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// DirectAnswerStep accepts an offer (via QR scan or paste), generates an
// answer, and displays it for the offerer to paste back.
export function DirectAnswerStep({
  session,
  onRemotePeerResolved,
  onBack,
}: DirectAnswerStepProps) {
  const [offerInput, setOfferInput] = useState('')
  const [answerPayload, setAnswerPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  const handleAcceptOffer = async (payload: string) => {
    if (!session || !payload.trim()) return
    setLoading(true)
    setError(null)
    try {
      const resp = await session.acceptLocalPairingOffer(payload.trim(), true)
      setAnswerPayload(resp.answerPayload ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to accept offer')
    } finally {
      setLoading(false)
    }
  }

  const handleDecoded = (decoded: string) => {
    setScanning(false)
    const payload = extractDirectOfferPayload(decoded)
    setOfferInput(payload)
    void handleAcceptOffer(payload)
    return true
  }

  // Watch pairing status for peer connection after answer is shared.
  usePairingWatch({
    session,
    enabled: !!answerPayload,
    reportFailures: false,
    streamFailure: 'Direct connection failed',
    onPeerResolved: onRemotePeerResolved,
    onFailure: setError,
  })

  return (
    <div>
      {scanning && (
        <QRScannerModal
          sx={nativeChildSpacingNext.s4}
          title="Scan direct pairing QR"
          hint="Point your camera at the QR code on the other device."
          containerId="direct-qr-scanner-container"
          qrboxSize={250}
          onDecoded={handleDecoded}
          onClose={() => setScanning(false)}
        />
      )}

      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule1]}>
        <div sx={stylexStyles2.rule2}>
          <LuCamera sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles2.rule3}>
          {answerPayload ? 'Share this response' : 'Scan or paste offer'}
        </h2>
        <p sx={stylexStyles2.rule4}>
          {answerPayload
            ? 'Copy this response and paste it on the other device.'
            : 'Scan the QR code or paste the offer from the other device.'}
        </p>
      </div>

      {!answerPayload && (
        <>
          <textarea
            value={offerInput}
            onChange={(e) => setOfferInput(e.target.value)}
            aria-label="Offer payload"
            placeholder="Paste offer payload here…"
            rows={3}
            sx={[nativeChildSpacingNext.s4, [stylexStyles2.rule5]]}
          />

          <button
            type="button"
            onClick={() => setScanning(true)}
            sx={[nativeChildSpacingNext.s4, [stylexStyles2.rule6]]}
          >
            <LuCamera sx={stylexStyles.rule2} />
            <span sx={stylexStyles2.rule7}>Scan QR code</span>
          </button>
        </>
      )}

      {answerPayload && (
        <div sx={nativeChildSpacingNext.s4}>
          <CopyablePayloadField
            sx={nativeChildSpacing.s2}
            value={answerPayload}
            label="Answer payload"
          />
          <div sx={[nativeChildSpacing.s2, stylexStyles2.rule8]}>
            <span sx={stylexStyles2.rule9} />
            <span sx={stylexStyles2.rule7}>Waiting for connection…</span>
          </div>
        </div>
      )}

      {error && (
        <p sx={[nativeChildSpacingNext.s4, stylexStyles2.rule10]}>{error}</p>
      )}

      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule11]}>
        <LinkDeviceBackButton onClick={onBack} />
        {!answerPayload && (
          <button
            type="button"
            onClick={() => void handleAcceptOffer(offerInput)}
            disabled={loading || !offerInput.trim() || !session}
            sx={[stylexStyles2.rule12]}
          >
            {loading ? (
              <Spinner />
            ) : (
              <>
                <LuLink sx={stylexStyles.rule3} />
                <span sx={stylexStyles2.rule13}>Accept offer</span>
              </>
            )}
          </button>
        )}
      </div>
    </div>
  )
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
