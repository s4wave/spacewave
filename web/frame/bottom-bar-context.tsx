import React, { createContext, useContext, useSyncExternalStore } from 'react'

/**
 * Represents a single item in the bottom bar.
 * Items are ordered by their nesting depth in the component tree.
 */
export interface BottomBarItem {
  /** Unique identifier for this item */
  id: string
  /** Depth in the component tree (for ordering) */
  depth: number
  /** Stable function that renders the button, receiving selected state */
  button: (
    selected: boolean,
    onClick: () => void,
    className: string,
  ) => React.ReactNode
  /** Optional key that triggers button re-registration when it changes */
  buttonKey?: React.Key
  /** Optional overlay resolver to display when this item is active */
  overlay?: () => React.ReactNode
  /** Optional key that triggers overlay re-registration when it changes */
  overlayKey?: React.Key
  /** Optional handler called when the breadcrumb separator to the right of this item is clicked */
  onBreadcrumbClick?: () => void
  /** Position in the bottom bar. Defaults to 'left'. */
  position?: 'left' | 'right'
}

/**
 * Context value provided by BottomBarLevel components.
 * Each nested BottomBarLevel has a reference to its parent and registers items imperatively.
 */
export interface BottomBarContextValue {
  /** Reference to parent context (null for root) */
  parent: BottomBarContextValue | null
  /** Depth in the component tree (0 for root) */
  depth: number
  /** Register an item with the root context */
  registerItem: (item: Omit<BottomBarItem, 'depth'> & { depth: number }) => void
  /** Unregister an item from the root context */
  unregisterItem: (id: string) => void
  /** Get root context (for SessionFrame to consume items) */
  getRoot: () => BottomBarRootContextValue | null
}

/**
 * Root context value that holds the registration functions.
 * Only the root BottomBarRoot component provides this.
 */
export interface BottomBarRootContextValue extends BottomBarContextValue {}

/**
 * Items context value for reading items via useSyncExternalStore.
 * This is separate from BottomBarContext to prevent re-renders when items change.
 */
export interface BottomBarItemsContextValue {
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => BottomBarItem[]
  openMenu?: string
  setOpenMenu?: (id: string) => void
}

/**
 * Context for managing bottom bar items hierarchically.
 * Items are registered through nesting BottomBarLevel components.
 * This context is stable and does NOT change when items change.
 */
export const BottomBarContext = createContext<BottomBarContextValue | null>(
  null,
)

/**
 * Context for reading bottom bar items.
 * Components that need to read items should use useBottomBarItems() hook.
 */
export const BottomBarItemsContext =
  createContext<BottomBarItemsContextValue | null>(null)

// Stable fallback functions for when there's no context
const emptyItems: BottomBarItem[] = []
const emptySubscribe = () => () => {}
const emptyGetSnapshot = () => emptyItems

/**
 * Hook to get the current bottom bar items.
 * Only components that call this hook will re-render when items change.
 */
export function useBottomBarItems(): BottomBarItem[] {
  const context = useContext(BottomBarItemsContext)
  const subscribe = context?.subscribe ?? emptySubscribe
  const getSnapshot = context?.getSnapshot ?? emptyGetSnapshot
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/**
 * Hook to get openMenu from the BottomBarItemsContext.
 */
export function useBottomBarOpenMenu(): string | undefined {
  const context = useContext(BottomBarItemsContext)
  return context?.openMenu
}

/**
 * Hook to get setOpenMenu from the BottomBarItemsContext.
 */
export function useBottomBarSetOpenMenu(): ((id: string) => void) | undefined {
  const context = useContext(BottomBarItemsContext)
  return context?.setOpenMenu
}

/**
 * Hook to check if the current item is the last (deepest) item in the bottom bar.
 */
export function useIsLastBottomBarItem(itemId: string): boolean {
  const items = useBottomBarItems()

  if (items.length === 0) return false

  const maxDepth = Math.max(...items.map((item) => item.depth))
  const currentItem = items.find((item) => item.id === itemId)

  return currentItem?.depth === maxDepth
}
