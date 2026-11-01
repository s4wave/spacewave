import type { StyleXStyles } from '@stylexjs/stylex'

import { Spinner, type SpinnerSize } from './Spinner.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  layout as themeLayout,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const styles = stylex.create({
  root: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  brand: { color: themeColors['--color-brand'] },
  muted: { color: themeColors['--color-foreground-alt'] },
  subtle: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  destructive: { color: themeColors['--color-destructive'] },
  label: {
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
  sx?: StyleXStyles
}

// LoadingInline renders a spinner + one-line label for inline use inside rows,
// buttons, or single-line loaders.
export function LoadingInline({
  label,
  size = 'sm',
  tone = 'muted',
  sx,
}: LoadingInlineProps) {
  return (
    <span
      sx={[
        styles.root,
        tone === 'brand'
          ? styles.brand
          : tone === 'muted'
            ? styles.muted
            : tone === 'subtle'
              ? styles.subtle
              : styles.destructive,
        sx,
      ]}
    >
      <Spinner size={size} />
      <span sx={styles.label}>{label}</span>
    </span>
  )
}
