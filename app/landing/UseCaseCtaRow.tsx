import type { ComponentType, ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

interface UseCaseCtaRowProps {
  children: ReactNode
}

// UseCaseCtaRow renders a centered row of call-to-action buttons.
export function UseCaseCtaRow({ children }: UseCaseCtaRowProps) {
  return <div className="flex flex-wrap justify-center gap-3">{children}</div>
}

interface UseCaseCtaLinkProps {
  href: string
  icon: ComponentType<{ className?: string }>
  variant?: 'primary' | 'default'
  children: ReactNode
}

// UseCaseCtaLink renders a single CTA anchor matching the landing page style.
export function UseCaseCtaLink({
  href,
  icon: Icon,
  variant = 'default',
  children,
}: UseCaseCtaLinkProps) {
  return (
    <a
      href={href}
      className={cn(
        'flex items-center gap-2 rounded-md border px-5 py-2.5 text-sm font-medium no-underline transition-all duration-300 select-none hover:-translate-y-0.5',
        variant === 'primary' ?
          'border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15'
        : 'border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8',
      )}
    >
      <Icon className="h-4 w-4" />
      <span>{children}</span>
    </a>
  )
}
