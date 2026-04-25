import { LuLoader } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

// SpinnerSize controls the rendered dimensions of Spinner.
export type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'

const sizeClasses: Record<SpinnerSize, string> = {
  sm: 'h-3.5 w-3.5',
  md: 'h-4 w-4',
  lg: 'h-6 w-6',
  xl: 'h-8 w-8',
}

interface SpinnerProps {
  size?: SpinnerSize
  className?: string
}

// Spinner renders the atomic animated loading indicator used across the app.
// Inherits text color from the parent so container state colors apply.
export function Spinner({ size = 'md', className }: SpinnerProps) {
  return (
    <LuLoader
      className={cn('animate-spin', sizeClasses[size], className)}
      aria-hidden="true"
    />
  )
}
