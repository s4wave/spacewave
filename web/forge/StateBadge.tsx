import { useMemo } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-destructive'],
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: 'var(--color-amber-400)',
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-emerald-400'],
  },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-blue-400'],
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
  },
  rule7: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule8: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-destructive'],
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule9: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: 'color-mix(in oklab, var(--color-amber-400) 20%, transparent)',
    backgroundColor:
      'color-mix(in oklab, var(--color-amber-400) 8%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: 'var(--color-amber-300)',
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-emerald-300'],
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule11: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-blue-400']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-blue-400']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-blue-300'],
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule12: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

/** StateBadgeVariant selects a pill or a status dot beside the label. */
export type StateBadgeVariant = 'pill' | 'dot'

/** StateBadgeTone groups enum labels into the existing semantic colors. */
export type StateBadgeTone = 'idle' | 'active' | 'success' | 'warning' | 'error'

interface StateBadgeProps {
  sx?: stylex.StyleXStyles
  state: number
  labels: Record<number, string>
  variant?: StateBadgeVariant
}

// stateTone maps a state label keyword to a semantic tone bucket.
function stateTone(label: string): StateBadgeTone {
  // Map the existing enum-label keywords to the established semantic palette.
  const l = label.toLowerCase()
  if (l.includes('complete')) return 'success'
  if (l.includes('running') || l.includes('checking') || l.includes('starting'))
    return 'active'
  if (l.includes('stopping')) return 'warning'
  if (l.includes('retry') || l.includes('error')) return 'error'
  return 'idle'
}

const pillStyles = (props?: {
  tone?: 'idle' | 'active' | 'success' | 'warning' | 'error' | null
  class?: stylex.StyleXStyles
  className?: stylex.StyleXStyles
}) => {
  // Resolve "tone" once, retaining explicit null instead of its default.
  const variant0Input = props?.['tone']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'idle'
      ? stylexStyles2.rule12
      : variant0 === 'active'
        ? stylexStyles2.rule11
        : variant0 === 'success'
          ? stylexStyles2.rule10
          : variant0 === 'warning'
            ? stylexStyles2.rule9
            : variant0 === 'error'
              ? stylexStyles2.rule8
              : stylexStyles2.rule7,
    props?.class,
    props?.className,
  ] as const
}

const dotStyles = (props?: {
  tone?: 'idle' | 'active' | 'success' | 'warning' | 'error' | null
  class?: stylex.StyleXStyles
  className?: stylex.StyleXStyles
}) => {
  // Resolve "tone" once, retaining explicit null instead of its default.
  const variant0Input = props?.['tone']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'idle'
      ? stylexStyles2.rule6
      : variant0 === 'active'
        ? stylexStyles2.rule5
        : variant0 === 'success'
          ? stylexStyles2.rule4
          : variant0 === 'warning'
            ? stylexStyles2.rule3
            : variant0 === 'error'
              ? stylexStyles2.rule2
              : stylexStyles2.rule1,
    props?.class,
    props?.className,
  ] as const
}

// StateBadge renders a tone-tinted pill or dot+label for a proto enum state.
export function StateBadge({
  state,
  labels,
  variant = 'pill',
  sx,
}: StateBadgeProps) {
  const label = useMemo(() => labels[state] ?? 'UNKNOWN', [state, labels])
  const tone = useMemo(() => stateTone(label), [label])
  if (variant === 'dot') {
    return (
      <span sx={[stylexStyles.rule1, sx]}>
        <span sx={dotStyles({ tone: tone })} />
        {label}
      </span>
    )
  }
  return <span sx={[pillStyles({ tone }), sx]}>{label}</span>
}
