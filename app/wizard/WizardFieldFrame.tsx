import { type ReactNode } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

export interface WizardFieldFrameProps {
  label: ReactNode
  help?: ReactNode
  fieldClassName?: string
  labelClassName?: string
  children: ReactNode
}

export function WizardFieldFrame({
  label,
  help,
  fieldClassName,
  labelClassName,
  children,
}: WizardFieldFrameProps) {
  return (
    <label className={cn('block min-w-0 space-y-1.5', fieldClassName)}>
      <span
        className={cn(
          'text-foreground-alt/70 block text-[0.65rem] font-medium',
          labelClassName,
        )}
      >
        {label}
      </span>
      {children}
      {help && (
        <span className="text-foreground-alt/50 block text-[0.65rem] leading-relaxed">
          {help}
        </span>
      )}
    </label>
  )
}
