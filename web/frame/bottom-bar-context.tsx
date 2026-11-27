import React, {
  createContext,
  use,
  useCallback,
  useSyncExternalStore,
} from 'react'

export type BottomBarContextMenuOpenKind = 'mouse' | 'keyboard' | 'touch'

export type BottomBarContextMenuActionVariant = 'default' | 'destructive'

export type BottomBarContextMenuIcon = React.ComponentType<{
  className?: string
}>

export interface BottomBarContextMenuActionContext {
  itemId: string
  openKind: BottomBarContextMenuOpenKind
  closeMenu: () => void
  openPrimaryOverlay: () => void
}

export interface BottomBarContextMenuAction {
  type: 'action'
  id: string
  label: string
  icon?: BottomBarContextMenuIcon
  disabled?: boolean
  variant?: BottomBarContextMenuActionVariant
  shortcut?: string
  onSelect: (context: BottomBarContextMenuActionContext) => void | Promise<void>
}

export interface BottomBarContextMenuSeparator {
  type: 'separator'
  id: string
}

export interface BottomBarContextMenuGroup {
  type: 'group'
  id: string
  label: string
  disabled?: boolean
  items: readonly BottomBarContextMenuItem[]
}

export type BottomBarContextMenuItem =
  | BottomBarContextMenuAction
  | BottomBarContextMenuSeparator
  | BottomBarContextMenuGroup

/**
 * BottomBarItem represents a single item in the bottom bar.
 * Items are ordered by their nesting depth in the component tree.
 */
export interface BottomBarItem {
  /** id is the unique identifier for this item. */
  id: string
  /** depth is the depth in the component tree (for ordering). */
  depth: number
  /** button renders the button, receiving selected state. */
  button: (
    selected: boolean,
    onClick: () => void,
    className: string,
  ) => React.ReactNode
  /**
   * hasOverlay is set when the item's level renders an overlay while its menu
   * is open.
   */
  hasOverlay?: boolean
  /**
   * menuLabel is an optional label used when this item is collapsed into a
   * menu.
   */
  menuLabel?: React.ReactNode
  /**
   * contextMenuLabel is an optional label for the secondary context menu
   * trigger.
   */
  contextMenuLabel?: string
  /**
   * contextMenuItems are typed secondary actions rendered by the frame's
   * context menu.
   */
  contextMenuItems?: readonly BottomBarContextMenuItem[]
  /**
   * onBreadcrumbClick is an optional handler called when the breadcrumb
   * separator to the right of this item is clicked.
   */
  onBreadcrumbClick?: () => void
  /** position is the position in the bottom bar. Defaults to 'left'. */
  position?: 'left' | 'right'
}

/**
 * BottomBarContextValue is the context value provided by BottomBarLevel
 * components.
 * Each nested BottomBarLevel has a reference to its parent and registers items
 * imperatively.
 */
export interface BottomBarContextValue {
  /** parent is a reference to the parent context (null for root). */
  parent: BottomBarContextValue | null
  /** depth is the depth in the component tree (0 for root). */
  depth: number
  /** registerItem registers an item with the root context. */
  registerItem: (item: BottomBarItem) => void
  /** unregisterItem unregisters an item from the root context. */
  unregisterItem: (id: string) => void
  /** getRoot gets the root context (for SessionFrame to consume items). */
  getRoot: () => BottomBarRootContextValue | null
}

/**
 * BottomBarRootContextValue is the root context value that holds the
 * registration functions.
 * Only the root BottomBarRoot component provides this value.
 */
export interface BottomBarRootContextValue extends BottomBarContextValue {}

// SetBottomBarOpenMenu accepts an absolute selection or a transition from the
// owner's latest selection.
export type SetBottomBarOpenMenu = (
  update: string | ((current: string) => string),
) => void

/**
 * BottomBarItemsContextValue is the items context value for reading items via
 * useSyncExternalStore.
 * This context is separate from BottomBarContext to prevent re-renders when
 * items change.
 */
export interface BottomBarItemsContextValue {
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => BottomBarItem[]
  /** subscribeOverlayHost subscribes to changes of the overlay host element. */
  subscribeOverlayHost: (listener: () => void) => () => void
  /**
   * getOverlayHost returns the element the open level portals its overlay into,
   * if any.
   */
  getOverlayHost: () => HTMLElement | null
  /**
   * overlayHostRef is the ref callback for the frame's overlay host element.
   */
  overlayHostRef: (el: HTMLElement | null) => () => void
  openMenu?: string
  setOpenMenu?: SetBottomBarOpenMenu
}

/**
 * BottomBarContext is the context for managing bottom bar items hierarchically.
 * Items are registered through nesting BottomBarLevel components.
 * This context is stable and does NOT change when items change.
 */
export const BottomBarContext = createContext<BottomBarContextValue | null>(
  null,
)

/**
 * BottomBarItemsContext is the context for reading bottom bar items.
 * Components that need to read items should use useBottomBarItems() hook.
 */
export const BottomBarItemsContext =
  createContext<BottomBarItemsContextValue | null>(null)

// Stable fallback functions for when there's no context
const emptyItems: BottomBarItem[] = []
const emptySubscribe = () => () => {}
const emptyGetSnapshot = () => emptyItems
const noOverlayHost = () => null

/**
 * useBottomBarItems gets the current bottom bar items.
 * Only components that call this hook will re-render when items change.
 */
export function useBottomBarItems(): BottomBarItem[] {
  const context = use(BottomBarItemsContext)
  const subscribe = context?.subscribe ?? emptySubscribe
  const getSnapshot = context?.getSnapshot ?? emptyGetSnapshot
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/**
 * useBottomBarOverlayHost gets the element the frame shows open overlays in.
 */
export function useBottomBarOverlayHost(): HTMLElement | null {
  const context = use(BottomBarItemsContext)
  const subscribe = context?.subscribeOverlayHost ?? emptySubscribe
  const getHost = context?.getOverlayHost ?? noOverlayHost
  return useSyncExternalStore(subscribe, getHost, getHost)
}

/**
 * useBottomBarOverlayHostRef gets the ref callback that sets the frame's
 * overlay host element.
 */
export function useBottomBarOverlayHostRef():
  | ((el: HTMLElement | null) => () => void)
  | undefined {
  return use(BottomBarItemsContext)?.overlayHostRef
}

/**
 * useBottomBarOpenMenu gets openMenu from the BottomBarItemsContext.
 */
export function useBottomBarOpenMenu(): string | undefined {
  const context = use(BottomBarItemsContext)
  return context?.openMenu
}

/**
 * useBottomBarSetOpenMenu gets setOpenMenu from the BottomBarItemsContext.
 */
export function useBottomBarSetOpenMenu(): SetBottomBarOpenMenu | undefined {
  const context = use(BottomBarItemsContext)
  return context?.setOpenMenu
}

// useBottomBarDismissMenu closes only the named menu. A departing popover can
// report dismissal after another trigger has selected a different menu.
export function useBottomBarDismissMenu(id: string): () => void {
  const setOpenMenu = useBottomBarSetOpenMenu()
  return useCallback(() => {
    if (id) setOpenMenu?.((current) => (current === id ? '' : current))
  }, [id, setOpenMenu])
}

/**
 * useIsLastBottomBarItem checks if the current item is the last (deepest) item
 * in the bottom bar.
 */
export function useIsLastBottomBarItem(itemId: string): boolean {
  const items = useBottomBarItems()

  if (items.length === 0) return false

  const maxDepth = Math.max(...items.map((item) => item.depth))
  const currentItem = items.find((item) => item.id === itemId)

  return currentItem?.depth === maxDepth
}
