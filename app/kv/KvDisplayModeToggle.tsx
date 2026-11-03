import {
  KV_DISPLAY_MODES,
  KV_DISPLAY_MODE_LABELS,
  type KvDisplayMode,
} from './kv-encoding.js'

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'inline-flex',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule4: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

interface KvDisplayModeToggleProps {
  mode: KvDisplayMode
  onModeChange: (mode: KvDisplayMode) => void
}

// KvDisplayModeToggle renders the text/hex/JSON value display mode toggle.
export function KvDisplayModeToggle({
  mode,
  onModeChange,
}: KvDisplayModeToggleProps) {
  return (
    <div
      role="radiogroup"
      aria-label="Value display mode"
      sx={stylexStyles.rule1}
    >
      {KV_DISPLAY_MODES.map((option) => (
        <button
          key={option}
          type="button"
          role="radio"
          aria-checked={option === mode}
          onClick={() => onModeChange(option)}
          sx={[
            stylexStyles.rule2,
            option === mode ? stylexStyles.rule3 : stylexStyles.rule4,
          ]}
        >
          {KV_DISPLAY_MODE_LABELS[option]}
        </button>
      ))}
    </div>
  )
}
