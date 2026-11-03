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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    WebkitUserSelect: 'all',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 40%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-bold)',
    justifyContent: 'center',
    letterSpacing: '.2em',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    outlineStyle: { default: null, ':focus-visible': 'none' },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'all',
    height: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    WebkitUserSelect: 'all',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-bold)',
    justifyContent: 'center',
    letterSpacing: '.2em',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    outlineStyle: { default: null, ':focus-visible': 'none' },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'all',
    height: `calc(${themeLayout['--spacing']} * 14)`,
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
    color: themeColors['--color-success'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'inline',
    height: `calc(${themeLayout['--spacing']} * 3)`,
    marginRight: themeLayout['--spacing'],
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
