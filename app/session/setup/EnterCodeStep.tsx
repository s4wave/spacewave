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
    height: `calc(${themeLayout['--spacing']} * 14)`,
    width: `calc(${themeLayout['--spacing']} * 48)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    letterSpacing: '.2em',
    color: themeColors['--color-foreground'],
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus': 'none' },
    outlineStyle: { default: null, ':focus': 'none' },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule3: { textAlign: 'center' },
  rule4: {
    marginInline: 'auto',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
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
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4'
      }
    >
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

      <div sx={stylexStyles2.rule3}>
        <div sx={stylexStyles2.rule4}>
          <LuLink sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles2.rule5}>Pair another device</h2>
        <p sx={stylexStyles2.rule6}>
          Enter the 8-character code shown on your other device.
        </p>
      </div>

      <CodeInput
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
        sx={[stylexStyles2.rule7]}
      >
        <LuCamera sx={stylexStyles.rule2} />
        <span sx={stylexStyles2.rule8}>Scan QR code</span>
      </button>

      {error && <p sx={stylexStyles2.rule9}>{error}</p>}

      <div sx={stylexStyles2.rule10}>
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
