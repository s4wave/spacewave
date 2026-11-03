import { useEffect, useEffectEvent, useRef } from 'react'
import { Html5Qrcode } from 'html5-qrcode'
import { LuX } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
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
  rule2: {
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
  rule3: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule6: { borderRadius: '.25rem', overflow: 'hidden' },
  rule7: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export interface QRScannerModalProps {
  title: string
  hint: string
  // containerId names the element html5-qrcode renders the camera into.
  containerId: string
  qrboxSize: number
  // onDecoded receives each decoded QR string. It returns true once it accepts
  // the payload, which stops the scanner.
  onDecoded: (decoded: string) => boolean
  onClose: () => void
}

// QRScannerModal renders a camera-based QR scanner overlay using html5-qrcode.
export function QRScannerModal({
  sx,
  title,
  hint,
  containerId,
  qrboxSize,
  onDecoded,
  onClose,
}: QRScannerModalProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const handleDecoded = useEffectEvent(onDecoded)

  useEffect(() => {
    if (!containerRef.current) return

    const scanner = new Html5Qrcode(containerRef.current.id)
    scanner
      .start(
        { facingMode: 'environment' },
        { fps: 10, qrbox: { width: qrboxSize, height: qrboxSize } },
        (decoded) => {
          if (handleDecoded(decoded)) {
            scanner.stop().catch(() => {})
          }
        },
        () => {},
      )
      .catch(() => {})

    return () => {
      scanner.stop().catch(() => {})
    }
  }, [qrboxSize])

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <h3 sx={stylexStyles2.rule4}>{title}</h3>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            sx={stylexStyles2.rule5}
          >
            <LuX sx={stylexStyles.rule1} />
          </button>
        </div>
        <div id={containerId} ref={containerRef} sx={stylexStyles2.rule6} />
        <p sx={stylexStyles2.rule7}>{hint}</p>
      </div>
    </div>
  )
}
