import type { ComponentType } from 'react'

// FeatureGrid renders icon and text feature items in a two-column grid.
export function FeatureGrid({
  features,
}: {
  features: {
    icon: ComponentType<{ className?: string }>
    text: string
  }[]
}) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {features.map((feature) => (
        <div key={feature.text} className="flex items-start gap-2">
          <feature.icon className="text-brand mt-0.5 size-4 shrink-0" />
          <span className="text-foreground-alt text-sm">{feature.text}</span>
        </div>
      ))}
    </div>
  )
}
