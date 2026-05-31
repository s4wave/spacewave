import React, { useMemo, useState } from 'react'
import {
  BottomBarContext,
  BottomBarRootContextValue,
  BottomBarItem,
  BottomBarItemsContext,
} from './bottom-bar-context.js'

export interface BottomBarRootProps {
  children: React.ReactNode
  openMenu?: string
  setOpenMenu?: (id: string) => void
}

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
      if (isQuietItemUpdate(existing, item)) {
        existing.button = item.button
        existing.overlay = item.overlay
        existing.onBreadcrumbClick = item.onBreadcrumbClick
        return
      }

      const updated = [...this.items]
      updated[existingIndex] = item
      updated.sort((a, b) => a.depth - b.depth)
      this.items = updated
      this.notify()
      return
    }

    const next = [...this.items, item]
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

function isQuietItemUpdate(a: BottomBarItem, b: BottomBarItem) {
  return (
    a.depth === b.depth &&
    a.buttonKey === b.buttonKey &&
    a.overlayKey === b.overlayKey &&
    a.menuLabel === b.menuLabel &&
    !!a.overlay === !!b.overlay &&
    !!a.onBreadcrumbClick === !!b.onBreadcrumbClick &&
    a.position === b.position
  )
}

export function BottomBarRoot({
  children,
  openMenu,
  setOpenMenu,
}: BottomBarRootProps) {
  const [store] = useState(() => new ItemsStore())

  const itemsContextValue = useMemo(
    () => ({
      subscribe: store.subscribe,
      getSnapshot: store.getSnapshot,
      openMenu,
      setOpenMenu,
    }),
    [store, openMenu, setOpenMenu],
  )

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
