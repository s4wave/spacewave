import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    display: 'inline-flex',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '12px',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 1.5,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule4: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
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
            controlTargetStyles.minimum,
          ]}
        >
          {KV_DISPLAY_MODE_LABELS[option]}
        </button>
      ))}
    </div>
  )
}
