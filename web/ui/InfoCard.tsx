import React from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// InfoCard displays a titled card with an icon and content.
export interface InfoCardProps {
  icon?: React.ReactNode
  title?: string
  children: React.ReactNode
  // Tightens card padding for a narrow container. Off by default.
  compact?: boolean
}

export function InfoCard({
  icon,
  title,
  children,
  compact = false,
}: InfoCardProps) {
  return (
    <div
      className={cn(
        'border-foreground/6 bg-background-card/30 min-w-0 rounded-lg border backdrop-blur-sm',
        compact ? 'p-2.5' : 'p-3.5',
      )}
    >
      {title && (
        <h3
          className={cn(
            'text-foreground flex items-center gap-2 text-sm select-none',
            compact ? 'mb-2' : 'mb-3',
          )}
        >
          {icon}
          {title}
        </h3>
      )}
      {!title && icon && (
        <div className="text-foreground mb-2 flex items-center">{icon}</div>
      )}
      {children}
    </div>
  )
}
