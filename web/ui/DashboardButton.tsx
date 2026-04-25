import React from 'react'
import { Button } from '@s4wave/web/ui/button.js'
import { cn } from '@s4wave/web/style/utils.js'

export interface DashboardButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  icon: React.ReactNode
  children?: React.ReactNode
}

export function DashboardButton({
  icon,
  children,
  className,
  ...props
}: DashboardButtonProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      className={cn(
        'rounded-menu-button border-foreground/8 bg-transparent',
        'hover:bg-foreground/5 hover:border-foreground/15',
        'text-foreground-alt hover:text-foreground',
        'flex items-center gap-1 text-xs select-none',
        'h-7 px-2 transition-all duration-150',
        className,
      )}
      {...props}
    >
      {icon}
      {children}
    </Button>
  )
}
