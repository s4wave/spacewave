import React from 'react'

// StatCard displays a metric with an icon, label, value, and optional detail.
export interface StatCardProps {
  icon: React.ElementType
  label: string
  value: string | number
  detail?: string
}

export function StatCard({ icon: Icon, label, value, detail }: StatCardProps) {
  return (
    <div className="group border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/60 flex items-center gap-3 rounded-lg border p-3 transition-all duration-150">
      <div className="bg-brand/10 group-hover:bg-brand/15 flex h-9 w-9 shrink-0 items-center justify-center rounded transition-all duration-150">
        <Icon className="text-brand h-4.5 w-4.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-foreground-alt text-xs select-none">{label}</p>
        <p className="text-foreground text-sm font-medium select-none">
          {value}
        </p>
        {detail && (
          <p className="text-foreground-alt/50 text-[0.6rem] select-none">
            {detail}
          </p>
        )}
      </div>
    </div>
  )
}
