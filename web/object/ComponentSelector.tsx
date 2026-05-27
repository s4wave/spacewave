import React, {
  useRef,
  useEffect,
  useState,
  useMemo,
  useCallback,
  useEffectEvent,
} from 'react'
import type { ObjectViewerComponent } from './object.js'
import { cn } from '@s4wave/web/style/utils.js'

// CategoryGroup represents a group of components under a category label.
interface CategoryGroup {
  category: string | null
  items: ObjectViewerComponent[]
}

// DropdownEntry represents either a category header or a selectable item.
type DropdownEntry =
  | { kind: 'header'; category: string }
  | { kind: 'separator'; category: string | null }
  | { kind: 'item'; component: ObjectViewerComponent; index: number }

interface ComponentSelectorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  components: ObjectViewerComponent[]
  selectedComponent?: ObjectViewerComponent
  onSelectComponent: (component: ObjectViewerComponent) => void
  children: React.ReactNode
}

// ComponentSelector displays a dropdown for selecting object viewer components.
export function ComponentSelector({
  open,
  onOpenChange,
  components,
  selectedComponent,
  onSelectComponent,
  children,
}: ComponentSelectorProps) {
  const triggerRef = useRef<HTMLDivElement>(null)
  const [focusedIndex, setFocusedIndex] = useState(0)

  // Build category groups and the flat dropdown entry list.
  const { entries, selectableIndices } = useMemo(() => {
    const ungrouped: ObjectViewerComponent[] = []
    const categoryMap = new Map<string, ObjectViewerComponent[]>()
    const categoryOrder: string[] = []

    for (const comp of components) {
      if (!comp.category) {
        ungrouped.push(comp)
      } else {
        const existing = categoryMap.get(comp.category)
        if (existing) {
          existing.push(comp)
        } else {
          categoryMap.set(comp.category, [comp])
          categoryOrder.push(comp.category)
        }
      }
    }

    const groups: CategoryGroup[] = []
    if (ungrouped.length > 0) {
      groups.push({ category: null, items: ungrouped })
    }
    for (const cat of categoryOrder) {
      const items = categoryMap.get(cat)
      if (items) {
        groups.push({ category: cat, items })
      }
    }

    // Build flat entry list for rendering and keyboard navigation.
    const entries: DropdownEntry[] = []
    const selectableIndices: number[] = []
    let itemIndex = 0

    for (let gi = 0; gi < groups.length; gi++) {
      const group = groups[gi]
      // Add separator between groups (not before the first).
      if (gi > 0) {
        entries.push({ kind: 'separator', category: group.category })
      }
      // Add category header if named.
      if (group.category) {
        entries.push({ kind: 'header', category: group.category })
      }
      for (const comp of group.items) {
        const entryIdx = entries.length
        entries.push({ kind: 'item', component: comp, index: itemIndex })
        selectableIndices.push(entryIdx)
        itemIndex++
      }
    }

    return { entries, selectableIndices }
  }, [components])

  // Map focusedIndex (among selectable items) to entries index.
  const focusedEntryIndex = selectableIndices[focusedIndex] ?? -1

  const handleSelect = useCallback(
    (comp: ObjectViewerComponent) => {
      onSelectComponent(comp)
      onOpenChange(false)
    },
    [onSelectComponent, onOpenChange],
  )
  const closeSelector = useEffectEvent(() => {
    onOpenChange(false)
  })
  const selectComponent = useEffectEvent((comp: ObjectViewerComponent) => {
    handleSelect(comp)
  })

  const handleTriggerKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key !== 'Enter' && event.key !== ' ') return
      event.preventDefault()
      onOpenChange(!open)
    },
    [onOpenChange, open],
  )

  useEffect(() => {
    if (!open) return

    const handleClickOutside = (event: MouseEvent) => {
      if (
        triggerRef.current &&
        !triggerRef.current.contains(event.target as Node)
      ) {
        closeSelector()
      }
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        closeSelector()
        return
      }

      if (event.key === 'ArrowDown') {
        event.preventDefault()
        setFocusedIndex((prev) => (prev + 1) % selectableIndices.length)
        return
      }

      if (event.key === 'ArrowUp') {
        event.preventDefault()
        setFocusedIndex((prev) =>
          prev === 0 ? selectableIndices.length - 1 : prev - 1,
        )
        return
      }

      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        const entryIdx = selectableIndices[focusedIndex]
        if (entryIdx !== undefined) {
          const entry = entries[entryIdx]
          if (entry.kind === 'item') {
            selectComponent(entry.component)
          }
        }
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleKeyDown)

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open, focusedIndex, entries, selectableIndices])

  return (
    <div className="relative" ref={triggerRef}>
      <div
        role="button"
        tabIndex={0}
        className="cursor-pointer"
        onClick={() => {
          onOpenChange(!open)
        }}
        onKeyDown={handleTriggerKeyDown}
      >
        {children}
      </div>

      {open && (
        <div
          role="menu"
          className={cn(
            'absolute right-0 bottom-full z-50 mb-1',
            'border-border bg-background-card min-w-[200px] rounded-md border p-1 shadow-md',
          )}
        >
          <div className="text-foreground px-2 py-1.5 text-sm font-semibold">
            Available Components
          </div>
          {entries.map((entry, idx) => {
            if (entry.kind === 'separator') {
              return (
                <div
                  key={`sep-${entry.category ?? 'ungrouped'}`}
                  className="bg-border mx-1 my-1 h-px"
                />
              )
            }
            if (entry.kind === 'header') {
              return (
                <div
                  key={`hdr-${entry.category}`}
                  className="text-muted-foreground px-2 py-1 text-[10px] font-medium tracking-wider uppercase"
                >
                  {entry.category}
                </div>
              )
            }
            const isSelected =
              selectedComponent?.componentID === entry.component.componentID
            const isFocused = focusedEntryIndex === idx
            return (
              <div
                role="menuitem"
                tabIndex={-1}
                key={entry.component.componentID}
                className={cn(
                  'text-foreground-alt relative flex cursor-pointer items-center rounded px-2 py-1.5 text-sm outline-none select-none',
                  'hover:bg-muted hover:text-foreground',
                  isFocused && 'bg-muted/50',
                  isSelected && 'bg-muted/70 text-foreground',
                )}
                onClick={() => handleSelect(entry.component)}
                onKeyDown={(event) => {
                  if (event.key !== 'Enter' && event.key !== ' ') return
                  event.preventDefault()
                  handleSelect(entry.component)
                }}
                onMouseEnter={() => {
                  const selectIdx = selectableIndices.indexOf(idx)
                  if (selectIdx >= 0) {
                    setFocusedIndex(selectIdx)
                  }
                }}
              >
                <span>{entry.component.name}</span>
                {isSelected && (
                  <span className="text-brand ml-auto text-xs">✓</span>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
