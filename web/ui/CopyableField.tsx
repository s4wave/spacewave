import React, { useCallback, useState } from 'react'
import { LuCheck, LuCopy } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  button: {
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginLeft: `calc(${themeLayout['--spacing']} * -2)`,
  },
  root: {
    gap: themeLayout['--spacing'],
    display: 'flex',
    flexDirection: 'column',
  },
  label: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  value: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    wordBreak: 'break-all',
  },
  copiedIcon: {
    color: themeColors['--color-green-500'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  copyIcon: {
    flexShrink: 0,
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':hover')]: 1,
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'opacity',
    transitionTimingFunction:
      'var(--tw-ease, var(--default-transition-timing-function))',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// CopyableFieldProps configures the label, copied value and native root styles.
export interface CopyableFieldProps {
  sx?: stylex.StyleXStyles
  label: string
  value: string
}

// CopyableField displays a labeled value that can be copied to clipboard.
export function CopyableField({ label, value, sx }: CopyableFieldProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [value])

  return (
    <div sx={[styles.root, sx]}>
      <span sx={styles.label}>{label}</span>
      <button
        type="button"
        onClick={handleCopy}
        {...stylex.props(styles.button, stylex.defaultMarker())}
        aria-label={copied ? 'Copied!' : 'Click to copy'}
      >
        <span sx={styles.value}>{value}</span>
        {copied ? (
          <LuCheck sx={styles.copiedIcon} />
        ) : (
          <LuCopy sx={styles.copyIcon} />
        )}
      </button>
    </div>
  )
}
