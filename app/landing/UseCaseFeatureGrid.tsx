import type { ComponentType } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { useScrollReveal } from './useScrollReveal.js'

export interface UseCaseFeature {
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
}

interface UseCaseFeatureGridProps {
  features: UseCaseFeature[]
}

// UseCaseFeatureGrid renders the two-column grid of feature cards shared
// across every use-case landing page.
export function UseCaseFeatureGrid({ features }: UseCaseFeatureGridProps) {
  return (
    <div className="grid gap-4 @lg:grid-cols-2">
      {features.map((feature, i) => (
        <UseCaseFeatureCard key={feature.title} feature={feature} index={i} />
      ))}
    </div>
  )
}

function UseCaseFeatureCard({
  feature,
  index,
}: {
  feature: UseCaseFeature
  index: number
}) {
  const { ref, visible } = useScrollReveal<HTMLDivElement>(0.1)
  const Icon = feature.icon
  return (
    <div
      ref={ref}
      className={cn(
        'border-foreground/6 bg-background-card/30 group rounded-lg border p-6 backdrop-blur-sm transition-all duration-500',
        visible ?
          'hover:border-foreground/12 hover:bg-background-card/50 translate-y-0 opacity-100 hover:-translate-y-0.5'
        : 'translate-y-8 opacity-0',
      )}
      style={{ transitionDelay: `${index * 80}ms` }}
    >
      <div className="mb-3 flex items-center gap-3">
        <div className="bg-brand/8 group-hover:bg-brand/15 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition-colors">
          <Icon className="text-brand h-4 w-4" />
        </div>
        <h3 className="text-foreground text-sm font-semibold">
          {feature.title}
        </h3>
      </div>
      <p className="text-foreground-alt text-sm leading-relaxed text-balance">
        {feature.description}
      </p>
    </div>
  )
}
