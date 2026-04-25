import type { ReactNode } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { useScrollReveal } from './useScrollReveal.js'

interface UseCaseSectionProps {
  children: ReactNode
  className?: string
}

// UseCaseSection wraps a use-case landing page section with scroll-reveal and
// a consistent max-width container.
export function UseCaseSection({ children, className }: UseCaseSectionProps) {
  const { ref, visible } = useScrollReveal(0.08)
  return (
    <section
      ref={ref}
      className={cn(
        'relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 transition-all duration-700 @lg:px-8',
        visible ? 'translate-y-0 opacity-100' : 'translate-y-8 opacity-0',
        className,
      )}
    >
      {children}
    </section>
  )
}
