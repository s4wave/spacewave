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
    marginLeft: `calc(${themeLayout['--spacing']} * -2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  root: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
  },
  label: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  value: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: themeColors['--color-foreground'],
  },
  copiedIcon: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-green-500'],
  },
  copyIcon: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':hover')]: 1,
      },
    },
    transitionProperty: 'opacity',
    transitionTimingFunction:
      'var(--tw-ease, var(--default-transition-timing-function))',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

// CopyableField displays a labeled value that can be copied to clipboard.
export interface CopyableFieldProps {
  label: string
  value: string
}

export function CopyableField({ label, value }: CopyableFieldProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [value])

  return (
    <div sx={styles.root}>
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
