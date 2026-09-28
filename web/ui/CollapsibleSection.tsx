import React, { useCallback } from 'react'
import * as Collapsible from '@radix-ui/react-collapsible'
import { LuChevronDown } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

/** CollapsibleSectionProps configures a section and its optional header actions. */
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
  // Tightens header and content padding for a narrow container. Off by default.
  compact?: boolean
}

/** CollapsibleSection renders a section whose header toggles its content. */
export function CollapsibleSection({
  title,
  icon,
  open,
  onOpenChange,
  children,
  className,
  badge,
  headerActions,
  compact = false,
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
            'hover:bg-background-card/50 flex min-h-11 items-center gap-2 transition-colors fine-pointer:min-h-0',
            compact ? 'px-2.5 sm:py-1.5' : 'px-3.5 sm:py-2.5',
            open && 'border-foreground/6 border-b',
          )}
        >
          <Collapsible.Trigger asChild>
            <button
              type="button"
              onClick={toggle}
              className={cn(
                'flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-2 self-stretch text-left fine-pointer:min-h-0',
                compact ? 'sm:-my-1.5 sm:py-1.5' : 'sm:-my-2.5 sm:py-2.5',
              )}
            >
              {icon && (
                <span className="text-foreground-alt/50 flex size-3.5 shrink-0 items-center justify-center">
                  {icon}
                </span>
              )}
              <span className="text-foreground min-w-0 flex-1 text-xs font-medium select-none">
                {title}
              </span>
              {badge}
              <LuChevronDown
                className={cn(
                  'text-foreground-alt/30 size-3.5 shrink-0 transition-transform duration-150',
                  open && 'rotate-180',
                )}
              />
            </button>
          </Collapsible.Trigger>
          {headerActions && (
            <div className="fine-pointer:[&_button]:min-h-0 fine-pointer:[&_button]:min-w-0 flex shrink-0 items-center [&_button]:min-h-11 [&_button]:min-w-11">
              {headerActions}
            </div>
          )}
        </div>
        <Collapsible.Content className="data-[state=closed]:animate-collapsible-up data-[state=open]:animate-collapsible-down overflow-hidden">
          <div className={cn(compact ? 'p-2.5' : 'p-3.5')}>{children}</div>
        </Collapsible.Content>
      </section>
    </Collapsible.Root>
  )
}
