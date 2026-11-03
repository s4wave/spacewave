import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import { cn } from '@s4wave/web/style/utils.js'
import React, { useMemo } from 'react'
import { LuCheck } from '@s4wave/web/images/icons.js'

import type { ObjectViewerComponent } from './object.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    maxWidth: '100%',
    minWidth: {
      default: 0,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    cursor: 'pointer',
    alignItems: 'center',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule2: { minWidth: `calc(${themeLayout['--spacing']} * 50)` },
  rule3: {
    marginLeft: 'auto',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
  },
})

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
  // Apply the enclosing bar's touch requirements to the actual trigger button.
  const requiredStyles = useControlStyles()
  const attributes = stylexProps(stylexStyles.rule1, {})
  attributes.className = cn(attributes.className, requiredStyles)
  const groups = useMemo(() => groupComponents(components), [components])

  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <button type="button" {...attributes}>
          {children}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side={placement === 'below' ? 'bottom' : 'top'}
        align={placement === 'below' ? 'start' : 'end'}
        sx={stylexStyles.rule2}
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
                  {selected && <LuCheck sx={stylexStyles.rule3} />}
                </DropdownMenuItem>
              )
            })}
          </React.Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
