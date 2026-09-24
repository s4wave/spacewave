import type { ReactNode } from 'react'

// InspectorSection is one titled group of readings inside a subsystem
// inspector. The action slot holds controls that act on the section.
export function InspectorSection({
  title,
  description,
  action,
  children,
}: {
  title: string
  description?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="border-foreground/8 bg-background-card/30 rounded-lg border p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-foreground text-sm font-semibold tracking-tight">
            {title}
          </h3>
          {description && (
            <p className="text-foreground-alt/60 mt-0.5 text-xs leading-relaxed">
              {description}
            </p>
          )}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}
