import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'

import { Spinner, type SpinnerSize } from './Spinner.js'

import * as stylex from '@stylexjs/stylex'
import { fonts as themeFonts } from '../../style/tokens.stylex.js'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: { color: themeColors['--color-brand'] },
  rule3: { color: themeColors['--color-foreground-alt'] },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule5: { color: themeColors['--color-destructive'] },
})

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

// LoadingInlineTone controls the color applied to both the spinner and label.
export type LoadingInlineTone = 'brand' | 'muted' | 'subtle' | 'destructive'

interface LoadingInlineProps {
  className?: never
  label: string
  size?: SpinnerSize
  tone?: LoadingInlineTone
  sx?: ComponentStyles
}

// LoadingInline renders a spinner + one-line label for inline use inside rows,
// buttons, or single-line loaders.
export function LoadingInline({
  label,
  size = 'sm',
  tone = 'muted',
  sx,
}: Omit<LoadingInlineProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <span
      sx={[
        stylexStyles2.rule1,
        tone === 'brand'
          ? stylexStyles2.rule2
          : tone === 'muted'
            ? stylexStyles2.rule3
            : tone === 'subtle'
              ? stylexStyles2.rule4
              : stylexStyles2.rule5,
        sx,
      ]}
    >
      <Spinner size={size} />
      <span sx={stylexStyles.rule1}>{label}</span>
    </span>
  )
}
