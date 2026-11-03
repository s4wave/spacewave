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
    position: 'fixed',
    inset: 0,
    zIndex: 50,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 80%, transparent)`,
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule2: {
    width: '100%',
    maxWidth: 'var(--container-sm)',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-shadow':
      '0 20px 25px -5px var(--tw-shadow-color, #0000001a), 0 8px 10px -6px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
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
  rule6: { overflow: 'hidden', borderRadius: '.25rem' },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
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
  title,
  hint,
  containerId,
  qrboxSize,
  onDecoded,
  onClose,
}: QRScannerModalProps) {
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
    <div sx={stylexStyles2.rule1}>
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
