import React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { Button } from '@s4wave/web/ui/button.js'
import { cn } from '@s4wave/web/style/utils.js'

const dashboardButtonVariants = cva(
  'rounded-menu-button flex items-center gap-1 select-none h-7 px-2 transition-all duration-150',
  {
    variants: {
      variant: {
        default:
          'border-foreground/8 bg-transparent text-foreground-alt hover:bg-foreground/5 hover:border-foreground/15 hover:text-foreground',
        active: 'border-foreground/15 bg-foreground/8 text-foreground',
        destructive:
          'text-destructive hover:bg-destructive/10 hover:text-destructive',
        primary:
          'border border-brand/30 bg-brand/10 text-foreground hover:border-brand/50 hover:bg-brand/15',
      },
    },
    defaultVariants: { variant: 'default' },
  },
)

export interface DashboardButtonProps
  extends
    React.ComponentPropsWithRef<'button'>,
    VariantProps<typeof dashboardButtonVariants> {
  icon: React.ReactNode
  children?: React.ReactNode
}

export function DashboardButton({
  icon,
  children,
  className,
  variant,
  ...props
}: DashboardButtonProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      className={cn(dashboardButtonVariants({ variant, className }))}
      {...props}
    >
      {icon}
      {children}
    </Button>
  )
}
