import { formatKeybinding } from './CommandPalette.js'

import {
  focusContextLabel,
  type KeybindingConflict,
} from './KeybindingResolver.js'

import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginTop: themeLayout['--spacing'] },
})

const childSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-warning'],
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-brand'],
  },
})

export interface KeybindingConflictListProps {
  conflicts: KeybindingConflict[]
  sx?: stylex.StyleXStyles
}

export function KeybindingConflictList({
  conflicts,
  sx,
}: KeybindingConflictListProps) {
  return (
    <div sx={[stylexStyles.rule1, sx]}>
      <div sx={stylexStyles.rule2}>Conflict</div>
      <div sx={stylexStyles2.rule1}>
        {conflicts.map((conflict) => (
          <div
            key={`${conflict.context}:${conflict.kind}:${conflict.key}`}
            sx={[stylexStyles.rule3, childSpacing.s1]}
          >
            {focusContextLabel(conflict.context)} {conflict.kind}{' '}
            <span sx={stylexStyles.rule4}>
              {conflict.key.split(' ').map(formatKeybinding).join(' ')}
            </span>{' '}
            is used by{' '}
            {conflict.bindings.map((binding) => binding.label).join(', ')}.
          </div>
        ))}
      </div>
    </div>
  )
}
