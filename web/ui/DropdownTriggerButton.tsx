import type React from 'react'
import { LuChevronDown } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { Button } from './button.js'

export interface DropdownTriggerButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  icon?: React.ReactNode
  triggerStyle?: 'outline' | 'ghost'
  showChevron?: boolean
}

// DropdownTriggerButton renders the shared button shell for dropdown triggers.
export function DropdownTriggerButton({
  icon,
  triggerStyle = 'outline',
  showChevron = true,
  className,
  children,
  type = 'button',
  ...props
}: DropdownTriggerButtonProps) {
  return (
    <Button
      type={type}
      variant={triggerStyle === 'ghost' ? 'ghost' : 'outline'}
      size="sm"
      className={cn(
        'gap-1.5',
        triggerStyle === 'outline' && [
          'border-foreground/10 bg-foreground/5 text-foreground-alt',
          'hover:border-brand/30 hover:bg-brand/10 hover:text-brand',
          'h-auto px-2.5 py-1 text-xs',
        ],
        triggerStyle === 'ghost' && [
          'text-foreground-alt hover:text-brand hover:bg-transparent',
          'h-auto px-0 py-0 text-[11px] font-normal shadow-none',
        ],
        className,
      )}
      {...props}
    >
      {icon}
      <span>{children}</span>
      {showChevron && <LuChevronDown className="h-3 w-3" />}
    </Button>
  )
}
