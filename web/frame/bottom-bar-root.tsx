import React, { useMemo, useState } from 'react'
import {
  BottomBarContext,
  BottomBarRootContextValue,
  BottomBarItem,
  BottomBarItemsContext,
} from './bottom-bar-context.js'

/**
 * Props for BottomBarRoot component.
 */
export interface BottomBarRootProps {
  /** Child components that may contain nested BottomBarLevel components */
  children: React.ReactNode
  /** Current open menu id */
  openMenu?: string
  /** Optional function to set the open menu id */
  setOpenMenu?: (id: string) => void
}

// ItemsStore manages the items state outside of React to prevent context re-renders
// when items change. Components that need to read items use useSyncExternalStore.
class ItemsStore {
  private items: BottomBarItem[] = []
  private listeners = new Set<() => void>()

  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  getSnapshot = () => this.items

  private notify() {
    for (const listener of this.listeners) {
      listener()
    }
  }

  registerItem = (item: Omit<BottomBarItem, 'depth'> & { depth: number }) => {
    const existingIndex = this.items.findIndex((i) => i.id === item.id)
    if (existingIndex !== -1) {
      const existing = this.items[existingIndex]
      if (
        existing.depth === item.depth &&
        existing.button === item.button &&
        existing.overlay === item.overlay &&
        existing.onBreadcrumbClick === item.onBreadcrumbClick &&
        existing.buttonKey === item.buttonKey &&
        existing.overlayKey === item.overlayKey
      ) {
        return
      }

      const updated = [...this.items]
      updated[existingIndex] = item as BottomBarItem
      updated.sort((a, b) => a.depth - b.depth)
      this.items = updated
      this.notify()
      return
    }

    const next = [...this.items, item as BottomBarItem]
    next.sort((a, b) => a.depth - b.depth)
    this.items = next
    this.notify()
  }

  unregisterItem = (id: string) => {
    const filtered = this.items.filter((i) => i.id !== id)
    if (filtered.length !== this.items.length) {
      this.items = filtered
      this.notify()
    }
  }
}

/**
 * BottomBarRoot is the root provider for bottom bar items.
 * It maintains the state of all registered items and provides registration functions.
 *
 * Usage:
 * ```tsx
 * <BottomBarRoot>
 *   <BottomBarLevel id="item1" button={...}>
 *     <BottomBarLevel id="item2" button={...}>
 *       <SessionFrame>
 *         <BottomBarLevel id="item3" button={...}>
 *           <Content />
 *         </BottomBarLevel>
 *       </SessionFrame>
 *     </BottomBarLevel>
 *   </BottomBarLevel>
 * </BottomBarRoot>
 * ```
 *
 * All items (item1, item2, item3) will be registered and available in SessionFrame.
 */
export function BottomBarRoot({
  children,
  openMenu,
  setOpenMenu,
}: BottomBarRootProps) {
  // Use useState with lazy initialization for the store
  const [store] = useState(() => new ItemsStore())

  // Items context value - uses useSyncExternalStore so only components
  // that actually read items will re-render when items change
  const itemsContextValue = useMemo(
    () => ({
      subscribe: store.subscribe,
      getSnapshot: store.getSnapshot,
      openMenu,
      setOpenMenu,
    }),
    [store, openMenu, setOpenMenu],
  )

  // Create stable context value - does NOT depend on items
  // Only components that call useBottomBarItems() will re-render when items change
  // Use useState for rootValue to enable stable getRoot() closure
  const [rootValue] = useState<BottomBarRootContextValue>(() => {
    const value: BottomBarRootContextValue = {
      parent: null,
      depth: 0,
      registerItem: store.registerItem,
      unregisterItem: store.unregisterItem,
      getRoot: () => value,
    }
    return value
  })

  return (
    <BottomBarContext.Provider value={rootValue}>
      <BottomBarItemsContext.Provider value={itemsContextValue}>
        {children}
      </BottomBarItemsContext.Provider>
    </BottomBarContext.Provider>
  )
}
