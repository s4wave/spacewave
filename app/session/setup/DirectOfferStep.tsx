import { useCallback, useEffect, useState } from 'react'
import { LuLink, LuWifi } from '@s4wave/web/images/icons.js'
import QRCode from 'qrcode'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { CopyablePayloadField } from './CopyablePayloadField.js'
import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'

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
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 16)`,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 48)`,
    height: `calc(${themeLayout['--spacing']} * 48)`,
    borderRadius: '.25rem',
  },
  rule7: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    width: '100%',
    resize: 'none',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus': 'none' },
    outlineStyle: { default: null, ':focus': 'none' },
  },
  rule9: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule10: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule11: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flex: 1,
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
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
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
    color: themeColors['--color-brand'],
  },
})

export interface DirectOfferStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// DirectOfferStep generates a local pairing offer and displays it as a QR
// code and copyable string. After the user pastes back the answerer's payload,
// completes the WebRTC connection and transitions to verification.
export function DirectOfferStep({
  session,
  onRemotePeerResolved,
  onBack,
}: DirectOfferStepProps) {
  const [offerPayload, setOfferPayload] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [answerInput, setAnswerInput] = useState('')
  const [accepting, setAccepting] = useState(false)

  // Generate the offer on mount.
  const generateOffer = useCallback(async () => {
    if (!session) return
    setLoading(true)
    setError(null)
    try {
      const resp = await session.createLocalPairingOffer()
      setOfferPayload(resp.offerPayload ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create offer')
    } finally {
      setLoading(false)
    }
  }, [session])

  useEffect(() => {
    queueMicrotask(() => {
      void generateOffer()
    })
  }, [generateOffer])

  const handleAcceptAnswer = async () => {
    if (!session || !answerInput.trim()) return
    setAccepting(true)
    setError(null)
    try {
      const resp = await session.acceptLocalPairingAnswer(answerInput.trim())
      if (resp.remotePeerId) {
        onRemotePeerResolved(resp.remotePeerId)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to accept answer')
    } finally {
      setAccepting(false)
    }
  }

  const qrCallback = useCallback(() => {
    if (!offerPayload) return undefined
    const url = `${SPACEWAVE_PUBLIC_BASE_URL}/#/pair/${offerPayload}`
    return QRCode.toDataURL(url, {
      width: 200,
      margin: 1,
      color: { dark: '#ffffff', light: '#00000000' },
      errorCorrectionLevel: 'L',
    })
  }, [offerPayload])
  const qrResult = usePromise(qrCallback)

  return (
    <div>
      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule1]}>
        <div sx={stylexStyles2.rule2}>
          <LuWifi sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles2.rule3}>Direct connection</h2>
        <p sx={stylexStyles2.rule4}>
          Show this QR code to your other device, or copy the text below.
        </p>
      </div>

      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule5]}>
        {loading && <Spinner size="lg" variant="muted" />}
        {!loading && offerPayload && (
          <>
            {qrResult.data && (
              <img
                src={qrResult.data}
                alt="Direct pairing QR"
                sx={stylexStyles2.rule6}
              />
            )}
            <CopyablePayloadField value={offerPayload} label="Offer payload" />
          </>
        )}
      </div>

      {offerPayload && (
        <div sx={nativeChildSpacingNext.s4}>
          <p sx={[nativeChildSpacing.s2, stylexStyles2.rule7]}>
            Paste the response from the other device:
          </p>
          <textarea
            value={answerInput}
            onChange={(e) => setAnswerInput(e.target.value)}
            aria-label="Answer payload"
            placeholder="Paste answer payload here…"
            rows={3}
            sx={[nativeChildSpacing.s2, [stylexStyles2.rule8]]}
          />
        </div>
      )}

      {error && (
        <p sx={[nativeChildSpacingNext.s4, stylexStyles2.rule9]}>{error}</p>
      )}

      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule10]}>
        <LinkDeviceBackButton onClick={onBack} />
        <button
          type="button"
          onClick={() => {
            void handleAcceptAnswer()
          }}
          disabled={accepting || !answerInput.trim() || !session}
          sx={[stylexStyles2.rule11]}
        >
          {accepting ? (
            <Spinner />
          ) : (
            <>
              <LuLink sx={stylexStyles.rule2} />
              <span sx={stylexStyles2.rule12}>Connect</span>
            </>
          )}
        </button>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
