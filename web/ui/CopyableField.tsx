import React, { useCallback, useState } from 'react'
import { LuCheck, LuCopy } from 'react-icons/lu'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
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
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-green-500'],
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
    <div sx={stylexStyles.rule1}>
      <span sx={stylexStyles.rule2}>{label}</span>
      <button
        type="button"
        onClick={handleCopy}
        className={
          /* TODO(stylex): global or unknown class: group */ 'group hover:bg-foreground/5 -ml-2 flex items-center gap-2 rounded-md px-2 py-1 text-left transition-colors'
        }
        aria-label={copied ? 'Copied!' : 'Click to copy'}
      >
        <span sx={stylexStyles.rule3}>{value}</span>
        {copied ? (
          <LuCheck {...stylex.props(stylexStyles.rule4)} />
        ) : (
          <LuCopy
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: group-hover:opacity-100 */ 'size-3.5 flex-shrink-0 opacity-0 transition-opacity group-hover:opacity-100'
            }
          />
        )}
      </button>
    </div>
  )
}
