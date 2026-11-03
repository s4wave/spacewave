import type { IconType } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    alignItems: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    backgroundColor: themeColors['--color-background'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
  },
  rule3: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
  },
  rule4: {
    display: { default: 'none', '@container (min-width: 32rem)': 'inline' },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// VmViewOption is one panel a running VM can show.
export interface VmViewOption<V extends string> {
  view: V
  label: string
  icon: IconType
}

// VmViewSwitchProps configures the panel switch of a running VM.
export interface VmViewSwitchProps<V extends string> {
  options: readonly VmViewOption<V>[]
  view: V
  onViewChange: (view: V) => void
}

// VmViewSwitch is the segmented switch between the panels of a running VM,
// such as its screen and its console. Labels collapse to icons in a narrow
// header.
export function VmViewSwitch<V extends string>({
  options,
  view,
  onViewChange,
}: VmViewSwitchProps<V>) {
  return (
    <div role="group" aria-label="VM panel" sx={stylexStyles2.rule1}>
      {options.map(({ view: option, label, icon: Icon }) => (
        <button
          key={option}
          type="button"
          aria-pressed={view === option}
          aria-label={label}
          onClick={() => onViewChange(option)}
          sx={view === option ? stylexStyles2.rule2 : stylexStyles2.rule3}
        >
          <Icon sx={stylexStyles.rule1} />
          <span sx={stylexStyles2.rule4}>{label}</span>
        </button>
      ))}
    </div>
  )
}
