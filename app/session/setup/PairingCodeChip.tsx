import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { useCallback, useState } from 'react'
import { LuCheck, LuCopy } from '@s4wave/web/images/icons.js'

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
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 14)`,
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
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
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'all',
    userSelect: 'all',
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule3: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 14)`,
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
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
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'all',
    userSelect: 'all',
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: { color: themeColors['--color-success'] },
  rule6: { color: themeColors['--color-foreground-alt'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-success'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    marginRight: themeLayout['--spacing'],
    display: 'inline',
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface PairingCodeChipProps {
  className?: never
  code: string
  sx?: ComponentStyles
}

// PairingCodeChip renders an 8-character pairing code as a centered, copyable
// chip matching the code-entry field. The copied indicator clears after a short
// delay.
export function PairingCodeChip({
  code,
  sx,
}: Omit<PairingCodeChipProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const [copied, setCopied] = useState(false)
  const formatted =
    code.length > 4 ? `${code.slice(0, 4)} ${code.slice(4)}` : code

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    })
  }, [code])

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      <button
        type="button"
        onClick={handleCopy}
        aria-label="Copy pairing code"
        sx={copied ? stylexStyles2.rule2 : stylexStyles2.rule3}
      >
        <span>{formatted}</span>
        {copied ? (
          <LuCheck sx={stylexStyles.rule1} aria-hidden="true" />
        ) : (
          <LuCopy sx={stylexStyles.rule2} aria-hidden="true" />
        )}
      </button>
      <span
        sx={[
          stylexStyles2.rule4,
          copied ? stylexStyles2.rule5 : stylexStyles2.rule6,
        ]}
        aria-live="polite"
      >
        {copied ? (
          <>
            <LuCheck sx={stylexStyles.rule3} aria-hidden="true" />
            Copied
          </>
        ) : (
          'Click the code to copy'
        )}
      </span>
    </div>
  )
}
