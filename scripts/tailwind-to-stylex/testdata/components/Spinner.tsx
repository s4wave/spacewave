import { LuLoader } from 'react-icons/lu'
import { cva } from 'class-variance-authority'

import { cn } from '@s4wave/web/style/utils.js'

// SpinnerSize controls the rendered dimensions of Spinner.
export type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'

const sizeClasses: Record<SpinnerSize, string> = {
  sm: 'h-3.5 w-3.5',
  md: 'h-4 w-4',
  lg: 'h-6 w-6',
  xl: 'h-8 w-8',
}

const spinnerVariants = cva('', {
  variants: {
    variant: {
      default: '',
      foreground: 'text-foreground',
      muted: 'text-foreground-alt',
      subtle: 'text-foreground-alt/40',
      brand: 'text-brand',
      destructive: 'text-destructive',
      success: 'text-success',
    },
  },
  defaultVariants: { variant: 'default' },
})

export type SpinnerVariant =
  | 'default'
  | 'foreground'
  | 'muted'
  | 'subtle'
  | 'brand'
  | 'destructive'
  | 'success'

interface SpinnerProps {
  size?: SpinnerSize
  variant?: SpinnerVariant
  className?: string
}

// Spinner renders the atomic animated loading indicator used across the app.
// Inherits text color from the parent so container state colors apply.
export function Spinner({ size = 'md', variant, className }: SpinnerProps) {
  return (
    <LuLoader
      className={cn(
        'animate-spin motion-reduce:animate-none',
        sizeClasses[size],
        spinnerVariants({ variant }),
        className,
      )}
      aria-hidden="true"
    />
  )
}
