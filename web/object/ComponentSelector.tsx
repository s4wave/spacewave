import React, { useMemo } from 'react'
import { LuCheck } from 'react-icons/lu'

import type { ObjectViewerComponent } from './object.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'

// CategoryGroup represents a group of components under a category label.
interface CategoryGroup {
  category: string | null
  items: ObjectViewerComponent[]
}

interface ComponentSelectorProps {
  open: boolean
  placement?: 'above' | 'below'
  onOpenChange: (open: boolean) => void
  components: ObjectViewerComponent[]
  selectedComponent?: ObjectViewerComponent
  onSelectComponent: (component: ObjectViewerComponent) => void
  children: React.ReactNode
}

// groupComponents lists the uncategorized components first, then each category
// in the order it first appears.
function groupComponents(components: ObjectViewerComponent[]): CategoryGroup[] {
  const ungrouped: ObjectViewerComponent[] = []
  const categories = new Map<string, ObjectViewerComponent[]>()
  for (const comp of components) {
    if (!comp.category) {
      ungrouped.push(comp)
      continue
    }
    const items = categories.get(comp.category)
    if (items) items.push(comp)
    else categories.set(comp.category, [comp])
  }

  const groups: CategoryGroup[] = []
  if (ungrouped.length > 0) groups.push({ category: null, items: ungrouped })
  for (const [category, items] of categories) groups.push({ category, items })
  return groups
}

/**
 * ComponentSelector displays and switches the available object viewer
 * components. The menu renders in a portal, so a clipping ancestor such as the
 * bottom bar cannot cut it off.
 */
export function ComponentSelector({
  open,
  placement = 'above',
  onOpenChange,
  components,
  selectedComponent,
  onSelectComponent,
  children,
}: ComponentSelectorProps) {
  const groups = useMemo(() => groupComponents(components), [components])

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          // The selector may sit inside another button, which it must not activate.
          onClick={(event) => event.stopPropagation()}
          className="flex max-w-full min-w-0 cursor-pointer items-center [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
        >
          {children}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side={placement === 'below' ? 'bottom' : 'top'}
        align={placement === 'below' ? 'start' : 'end'}
        className="min-w-50"
      >
        <DropdownMenuLabel>Available Components</DropdownMenuLabel>
        {groups.map((group, index) => (
          <React.Fragment key={group.category ?? 'ungrouped'}>
            {index > 0 && <DropdownMenuSeparator />}
            {group.category && (
              <DropdownMenuLabel>{group.category}</DropdownMenuLabel>
            )}
            {group.items.map((comp) => {
              const selected =
                selectedComponent?.componentID === comp.componentID
              return (
                <DropdownMenuItem
                  key={comp.componentID}
                  variant={selected ? 'selected' : 'default'}
                  onSelect={() => onSelectComponent(comp)}
                >
                  <span>{comp.name}</span>
                  {selected && (
                    <LuCheck className="text-brand ml-auto size-3.5" />
                  )}
                </DropdownMenuItem>
              )
            })}
          </React.Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
