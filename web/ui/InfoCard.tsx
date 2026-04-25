import React from 'react'

// InfoCard displays a titled card with an icon and content.
export interface InfoCardProps {
  icon?: React.ReactNode
  title?: string
  children: React.ReactNode
}

export function InfoCard({ icon, title, children }: InfoCardProps) {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5 backdrop-blur-sm">
      {title && (
        <h3 className="text-foreground mb-3 flex items-center gap-2 text-sm select-none">
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
