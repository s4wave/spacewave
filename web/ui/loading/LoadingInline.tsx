import { cn } from '@s4wave/web/style/utils.js'

import { Spinner, type SpinnerSize } from './Spinner.js'

import * as stylex from '@stylexjs/stylex'
import { fonts as themeFonts } from '../../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

// LoadingInlineTone controls the color applied to both the spinner and label.
export type LoadingInlineTone = 'brand' | 'muted' | 'subtle' | 'destructive'

interface LoadingInlineProps {
  label: string
  size?: SpinnerSize
  tone?: LoadingInlineTone
  className?: string
}

const toneClasses: Record<LoadingInlineTone, string> = {
  brand: 'text-brand',
  muted: 'text-foreground-alt',
  subtle: 'text-foreground-alt/40',
  destructive: 'text-destructive',
}

// LoadingInline renders a spinner + one-line label for inline use inside rows,
// buttons, or single-line loaders.
export function LoadingInline({
  label,
  size = 'sm',
  tone = 'muted',
  className,
}: LoadingInlineProps) {
  return (
    <span
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'inline-flex items-center gap-1.5',
          toneClasses[tone],
          className,
        )
      }
    >
      <Spinner size={size} />
      <span sx={stylexStyles.rule1}>{label}</span>
    </span>
  )
}
