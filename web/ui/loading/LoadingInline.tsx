import { cn } from '@s4wave/web/style/utils.js'

import { Spinner, type SpinnerSize } from './Spinner.js'

// LoadingInlineTone controls the color applied to both the spinner and label.
export type LoadingInlineTone = 'brand' | 'muted' | 'destructive'

interface LoadingInlineProps {
  label: string
  size?: SpinnerSize
  tone?: LoadingInlineTone
  className?: string
}

const toneClasses: Record<LoadingInlineTone, string> = {
  brand: 'text-brand',
  muted: 'text-foreground-alt',
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
      className={cn(
        'inline-flex items-center gap-1.5',
        toneClasses[tone],
        className,
      )}
    >
      <Spinner size={size} />
      <span className="text-xs">{label}</span>
    </span>
  )
}
