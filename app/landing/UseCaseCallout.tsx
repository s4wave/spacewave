import type { ReactNode } from 'react'

interface UseCaseCalloutProps {
  title: string
  children: ReactNode
}

// UseCaseCallout renders the brand-tinted context box under the feature grid
// on each use-case landing page.
export function UseCaseCallout({ title, children }: UseCaseCalloutProps) {
  return (
    <div className="border-brand/20 bg-brand/5 rounded-lg border p-8 backdrop-blur-sm">
      <h2 className="text-foreground mb-4 text-center text-xl font-bold @lg:text-2xl">
        {title}
      </h2>
      <div className="text-foreground-alt space-y-3 text-center text-sm leading-relaxed">
        {children}
      </div>
    </div>
  )
}
