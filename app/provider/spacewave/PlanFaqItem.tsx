import type { KeyboardEvent, ReactNode } from 'react'
import { LuPlus } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

// PlanFaqItem renders a single expandable FAQ item.
export function PlanFaqItem({
  question,
  answer,
  isOpen,
  onToggle,
}: {
  question: string
  answer: ReactNode
  isOpen: boolean
  onToggle: () => void
}) {
  const handleFaqKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onToggle()
  }

  return (
    <div
      role="button"
      tabIndex={0}
      className={cn(
        'cursor-pointer rounded-lg border p-4 backdrop-blur-sm transition-all',
        isOpen
          ? 'border-foreground/12 bg-background-card/60'
          : 'border-foreground/6 bg-background-card/20 hover:border-foreground/12',
      )}
      onClick={onToggle}
      onKeyDown={handleFaqKeyDown}
    >
      <div className="flex items-start justify-between gap-3">
        <h3
          className={cn(
            'text-xs leading-relaxed font-medium transition-colors',
            isOpen
              ? 'text-foreground'
              : 'text-foreground-alt group-hover:text-foreground',
          )}
        >
          {question}
        </h3>
        <div
          className={cn(
            'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded transition-all',
            isOpen
              ? 'bg-brand/12 text-brand rotate-45'
              : 'bg-foreground/6 text-foreground-alt',
          )}
        >
          <LuPlus className="size-2.5" />
        </div>
      </div>
      <div
        className={cn(
          'grid transition-all duration-300 ease-in-out',
          isOpen ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
        )}
      >
        <div className="overflow-hidden">
          <p className="text-foreground-alt pt-2 text-xs leading-relaxed">
            {answer}
          </p>
        </div>
      </div>
    </div>
  )
}
