import { useState } from 'react'
import { LuCamera, LuLink } from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { Session } from '@s4wave/sdk/session/session.js'

import { LinkDeviceBackButton } from './LinkDeviceBackButton.js'
import { pairingErrorMessage } from './pairing-copy.js'
import { QRScannerModal } from './QRScannerModal.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { display: 'flex', justifyContent: 'center' },
  rule2: {
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
  rule3: { textAlign: 'center' },
  rule4: {
    marginInline: 'auto',
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule7: {
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
  rule8: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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

// extractCodeFromQR extracts a pairing code from a QR-encoded URL or raw code string.
function extractCodeFromQR(decoded: string): string | null {
  const urlMatch = decoded.match(/#\/pair\/([A-Za-z0-9]{8})/)
  if (urlMatch) return urlMatch[1].toUpperCase()
  const cleaned = decoded.replace(/[^A-Za-z0-9]/g, '')
  if (cleaned.length === 8) return cleaned.toUpperCase()
  return null
}

// cleanCode keeps the alphanumeric characters of a typed or pasted pairing
// code, upper-cased and limited to the 8-character code length.
function cleanCode(value: string): string {
  return value
    .replace(/[^A-Za-z0-9]/g, '')
    .toUpperCase()
    .slice(0, 8)
}

interface CodeInputProps {
  value: string
  onChange: (value: string) => void
  onPaste: (e: React.ClipboardEvent) => void
  onSubmit?: () => void
  disabled?: boolean
}

// CodeInput renders an OTP-style 8-character pairing code input with paste support.
function CodeInput({
  sx,
  value,
  onChange,
  onPaste,
  onSubmit,
  disabled,
}: CodeInputProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const formatted =
    value.length > 4 ? `${value.slice(0, 4)} ${value.slice(4)}` : value

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && value.length === 8 && !disabled) {
      onSubmit?.()
    }
  }
  const handleInputRef = (node: HTMLInputElement | null) => {
    node?.focus()
  }

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      <input
        ref={handleInputRef}
        type="text"
        value={formatted}
        onChange={(e) => onChange(e.target.value)}
        onPaste={onPaste}
        onKeyDown={handleKeyDown}
        aria-label="Pairing code"
        placeholder="XXXX XXXX"
        maxLength={9}
        disabled={disabled}
        sx={[stylexStyles2.rule2]}
      />
    </div>
  )
}

export interface EnterCodeStepProps {
  session: Session | null | undefined
  onRemotePeerResolved: (peerId: string) => void
  onBack: () => void
}

// EnterCodeStep completes pairing with a code typed, pasted, or scanned from
// the other device.
export function EnterCodeStep({
  session,
  onRemotePeerResolved,
  onBack,
}: EnterCodeStepProps) {
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  const handleSubmit = async () => {
    if (!session || code.length < 8) return
    setLoading(true)
    setError(null)
    try {
      const remotePeerId = await session.completePairing({
        code: code.replace(/\s/g, ''),
        offerCurrentAccount: true,
      })
      if (remotePeerId) {
        onRemotePeerResolved(remotePeerId)
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to complete pairing'
      setError(pairingErrorMessage(message))
      // Surface via toast too: a failed submit can re-render or navigate the
      // settings pane away from this card before the inline error is seen.
      toast.error(pairingErrorMessage(message))
    } finally {
      setLoading(false)
    }
  }

  const handlePaste = (e: React.ClipboardEvent) => {
    e.preventDefault()
    setCode(cleanCode(e.clipboardData.getData('text')))
  }

  const handleDecoded = (decoded: string) => {
    const scanned = extractCodeFromQR(decoded)
    if (!scanned) return false
    setCode(scanned)
    setScanning(false)
    return true
  }

  return (
    <div>
      {scanning && (
        <QRScannerModal
          title="Scan QR code"
          hint="Point your camera at the QR code on your other device."
          containerId="qr-scanner-container"
          qrboxSize={200}
          onDecoded={handleDecoded}
          onClose={() => setScanning(false)}
        />
      )}

      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule3]}>
        <div sx={stylexStyles2.rule4}>
          <LuLink sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles2.rule5}>Pair another device</h2>
        <p sx={stylexStyles2.rule6}>
          Enter the 8-character code shown on your other device.
        </p>
      </div>

      <CodeInput
        sx={nativeChildSpacing.s4}
        value={code}
        onChange={(value) => setCode(cleanCode(value))}
        onPaste={handlePaste}
        onSubmit={() => {
          void handleSubmit()
        }}
        disabled={loading}
      />

      <button
        type="button"
        onClick={() => setScanning(true)}
        sx={[nativeChildSpacing.s4, [stylexStyles2.rule7]]}
      >
        <LuCamera sx={stylexStyles.rule2} />
        <span sx={stylexStyles2.rule8}>Scan QR code</span>
      </button>

      {error && (
        <p sx={[nativeChildSpacing.s4, stylexStyles2.rule9]}>{error}</p>
      )}

      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule10]}>
        <LinkDeviceBackButton onClick={onBack} />
        <button
          type="button"
          onClick={() => {
            void handleSubmit()
          }}
          disabled={loading || code.length < 8 || !session}
          sx={[stylexStyles2.rule11]}
        >
          {loading ? (
            <Spinner />
          ) : (
            <>
              <LuLink sx={stylexStyles.rule3} />
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
