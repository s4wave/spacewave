import React, { useMemo, useState } from 'react'
import {
  BottomBarContext,
  BottomBarRootContextValue,
  BottomBarItem,
  BottomBarItemsContext,
  type SetBottomBarOpenMenu,
} from './bottom-bar-context.js'

export interface BottomBarRootProps {
  children: React.ReactNode
  openMenu?: string
  setOpenMenu?: SetBottomBarOpenMenu
}

// ItemsStore holds the registered items and the element the frame shows the
// open overlay in. Levels portal their own overlay into that element, so an
// overlay renders with its level's state and contexts.
class ItemsStore {
  private items: BottomBarItem[] = []
  private listeners = new Set<() => void>()
  private overlayHost: HTMLElement | null = null
  private overlayHostListeners = new Set<() => void>()

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

  // registerItem adds item or replaces the item with its id. Levels memoize
  // their item, so registering the same item again changes nothing.
  registerItem = (item: BottomBarItem) => {
    const index = this.items.findIndex((i) => i.id === item.id)
    if (index !== -1 && this.items[index] === item) return
    const next =
      index === -1 ? [...this.items, item] : this.items.with(index, item)
    next.sort((a, b) => a.depth - b.depth)
    this.items = next
    this.notify()
  }

  subscribeOverlayHost = (listener: () => void) => {
    this.overlayHostListeners.add(listener)
    return () => this.overlayHostListeners.delete(listener)
  }

  getOverlayHost = () => this.overlayHost

  // overlayHostRef names el as the overlay host until React detaches it. A
  // detach only clears the host it set, so a replacing frame keeps its own.
  overlayHostRef = (el: HTMLElement | null) => {
    this.setOverlayHost(el)
    return () => {
      if (this.overlayHost === el) this.setOverlayHost(null)
    }
  }

  private setOverlayHost(el: HTMLElement | null) {
    if (this.overlayHost === el) return
    this.overlayHost = el
    for (const listener of this.overlayHostListeners) {
      listener()
    }
  }

  unregisterItem = (id: string) => {
    const filtered = this.items.filter((i) => i.id !== id)
    if (filtered.length !== this.items.length) {
      this.items = filtered
      this.notify()
    }
  }
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
      subscribeOverlayHost: store.subscribeOverlayHost,
      getOverlayHost: store.getOverlayHost,
      overlayHostRef: store.overlayHostRef,
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
