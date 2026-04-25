import React from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// ActionCard renders an interactive card with icon, label, and description.
export interface ActionCardProps {
  icon: React.ReactNode
  label: string
  description: string
  onClick?: () => void
  variant?: 'default' | 'destructive'
}

export function ActionCard({
  icon,
  label,
  description,
  onClick,
  variant = 'default',
}: ActionCardProps) {
  return (
    <button
      onClick={onClick}
      disabled={!onClick}
      className={cn(
        'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 group flex w-full cursor-pointer items-center gap-3 rounded-lg border p-3 text-left backdrop-blur-sm transition-all',
        variant === 'destructive' &&
          'hover:border-destructive/30 hover:bg-destructive/5',
        !onClick && 'cursor-not-allowed opacity-50',
      )}
    >
      <div
        className={cn(
          'bg-foreground/5 group-hover:bg-foreground/8 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors',
          variant === 'destructive' &&
            'bg-destructive/10 group-hover:bg-destructive/15',
        )}
      >
        <div
          className={cn(
            'text-foreground-alt',
            variant === 'destructive' && 'text-destructive',
          )}
        >
          {icon}
        </div>
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <h4 className="text-foreground text-xs font-medium select-none">
          {label}
        </h4>
        <p className="text-foreground-alt/50 text-[0.6rem] select-none">
          {description}
        </p>
      </div>
    </button>
  )
}
