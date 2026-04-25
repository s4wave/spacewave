import type React from 'react'
import { LuArrowLeft } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

export interface BackButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  floating?: boolean
}

// BackButton renders the shared arrow-left back action used across auth and
// full-page overlays.
export function BackButton({
  floating,
  className,
  children,
  type = 'button',
  ...props
}: BackButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        'text-foreground-alt hover:text-brand flex items-center gap-2 text-sm transition-colors',
        floating && 'absolute top-4 left-4 z-20',
        className,
      )}
      {...props}
    >
      <LuArrowLeft className="h-4 w-4" />
      <span className="select-none">{children}</span>
    </button>
  )
}
