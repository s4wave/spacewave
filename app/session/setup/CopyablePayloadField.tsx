import { useState } from 'react'
import { LuCopy } from '@s4wave/web/images/icons.js'

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
    width: '100%',
  },
  rule2: {
    '--tw-outline-style': { default: null, ':focus': 'none' },
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    WebkitUserSelect: 'all',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: { default: null, ':focus': 'none' },
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    userSelect: 'all',
  },
  rule3: {
    '--tw-duration': '.3s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: { color: themeColors['--color-brand'] },
  rule3: { color: themeColors['--color-foreground-alt'] },
})

export interface CopyablePayloadFieldProps {
  value: string
  label: string
}

// CopyablePayloadField shows a pairing payload in a select-all field with a
// button that copies it to the clipboard.
export function CopyablePayloadField({
  sx,
  value,
  label,
}: CopyablePayloadFieldProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    void navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      <input
        readOnly
        value={value}
        aria-label={label}
        sx={[stylexStyles2.rule2]}
        onClick={(e) => e.currentTarget.select()}
      />
      <button
        type="button"
        onClick={handleCopy}
        sx={[stylexStyles2.rule3]}
        title="Copy to clipboard"
      >
        <LuCopy
          sx={[
            stylexStyles.rule1,
            copied ? stylexStyles.rule2 : stylexStyles.rule3,
          ]}
        />
      </button>
    </div>
  )
}
