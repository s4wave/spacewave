import React, { useCallback } from 'react'
import * as Collapsible from '@radix-ui/react-collapsible'
import { LuChevronDown } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

export interface CollapsibleSectionProps {
  // Section title displayed in the header.
  title: string
  // Icon rendered before the title.
  icon?: React.ReactNode
  // Whether the section is currently open.
  open: boolean
  // Called when the open state changes.
  onOpenChange: (open: boolean) => void
  // Content rendered when expanded.
  children: React.ReactNode
  // Optional class name for the outer container.
  className?: string
  // Optional badge or count rendered after the title.
  badge?: React.ReactNode
  // Optional actions rendered in the header outside the trigger button.
  headerActions?: React.ReactNode
}

// CollapsibleSection wraps content in a glass card with a clickable
// header that toggles visibility. Uses Radix Collapsible for
// accessibility (keyboard, ARIA).
export function CollapsibleSection({
  title,
  icon,
  open,
  onOpenChange,
  children,
  className,
  badge,
  headerActions,
}: CollapsibleSectionProps) {
  const toggle = useCallback(() => onOpenChange(!open), [open, onOpenChange])

  return (
    <Collapsible.Root open={open} onOpenChange={onOpenChange} asChild>
      <section
        className={cn(
          'border-foreground/6 bg-background-card/30 rounded-lg border backdrop-blur-sm',
          className,
        )}
      >
        <div
          className={cn(
            'hover:bg-background-card/50 flex items-center gap-2 px-3.5 py-2.5 transition-colors',
            open && 'border-foreground/6 border-b',
          )}
        >
          <Collapsible.Trigger asChild>
            <button
              type="button"
              onClick={toggle}
              className="-my-2.5 flex min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch py-2.5 text-left"
            >
              {icon && (
                <span className="text-foreground-alt/50 flex h-3.5 w-3.5 shrink-0 items-center justify-center">
                  {icon}
                </span>
              )}
              <span className="text-foreground flex-1 text-xs font-medium select-none">
                {title}
              </span>
              {badge}
              <LuChevronDown
                className={cn(
                  'text-foreground-alt/30 h-3.5 w-3.5 shrink-0 transition-transform duration-150',
                  open && 'rotate-180',
                )}
              />
            </button>
          </Collapsible.Trigger>
          {headerActions && (
            <div className="flex shrink-0 items-center">{headerActions}</div>
          )}
        </div>
        <Collapsible.Content className="data-[state=closed]:animate-collapsible-up data-[state=open]:animate-collapsible-down overflow-hidden">
          <div className="p-3.5">{children}</div>
        </Collapsible.Content>
      </section>
    </Collapsible.Root>
  )
}
