import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { stylexProps } from '@s4wave/web/style/props.js'

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
    alignItems: 'center',
    cursor: 'pointer',
    display: 'flex',
    maxWidth: '100%',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: 0,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule2: { minWidth: `calc(${themeLayout['--spacing']} * 50)` },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    marginLeft: 'auto',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// CategoryGroup represents a group of components under a category label.
interface CategoryGroup {
  category: string | null
  items: ObjectViewerComponent[]
}

interface ComponentSelectorProps {
  sx?: stylex.StyleXStyles
  className?: never
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
  // Accumulate each viewer once in its visible category.
  const ungrouped: ObjectViewerComponent[] = []
  const categories = new Map<string, ObjectViewerComponent[]>()

  // Keep each category in its first-seen order while separating ungrouped viewers.
  for (const comp of components) {
    if (!comp.category) {
      ungrouped.push(comp)
      continue
    }
    const items = categories.get(comp.category)
    if (items) items.push(comp)
    else categories.set(comp.category, [comp])
  }

  // Place uncategorized viewers before the ordered categories.
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
  sx,
}: ComponentSelectorProps) {
  // Apply the enclosing bar's touch requirements to the actual trigger button.
  const requiredStyles = useControlStyles()
  const attributes = stylexProps([stylexStyles.rule1, sx], {})
  attributes.className = [attributes.className, requiredStyles]
    .filter(Boolean)
    .join(' ')
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
