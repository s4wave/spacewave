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
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    minHeight: `calc(${themeLayout['--spacing']} * 16)`,
  },
  rule6: {
    borderRadius: '.25rem',
    height: `calc(${themeLayout['--spacing']} * 48)`,
    width: `calc(${themeLayout['--spacing']} * 48)`,
  },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule8: {
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
  rule9: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule10: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'flex' },
  rule11: {
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
  rule12: {
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
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
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
