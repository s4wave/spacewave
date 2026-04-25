import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
} from 'react'
import {
  BottomBarContext,
  BottomBarContextValue,
} from './bottom-bar-context.js'

/**
 * Props for BottomBarLevel component.
 */
export interface BottomBarLevelProps {
  /** Unique identifier for this bottom bar item */
  id: string
  /** Function that renders the button, receiving selected state, onClick handler, and className */
  button: (
    selected: boolean,
    onClick: () => void,
    className?: string,
  ) => React.ReactNode
  /** Optional overlay content to display when this item is active */
  overlay?: React.ReactNode
  /** Optional key used to update the button when its content changes */
  buttonKey?: React.Key
  /** Optional key used to update the overlay when its content changes */
  overlayKey?: React.Key
  /** Optional handler called when the breadcrumb separator to the right of this item is clicked */
  onBreadcrumbClick?: () => void
  /** Position in the bottom bar. Defaults to 'left'. */
  position?: 'left' | 'right'
  /** Child components that may contain nested BottomBarLevel components */
  children: React.ReactNode
}

/**
 * BottomBarLevel is a component that registers a bottom bar item imperatively
 * with the root context and provides context for nested children to register their own items.
 *
 * Usage:
 * ```tsx
 * <BottomBarLevel
 *   id="my-item"
 *   button={(selected, onClick, className) => <BottomBarItem>My Button</BottomBarItem>}
 *   overlay={<div>Optional overlay content</div>}
 * >
 *   <MyContent />
 *   // Nested BottomBarLevel components will appear after this one
 *   <BottomBarLevel id="nested" button={...}>
 *     <NestedContent />
 *   </BottomBarLevel>
 * </BottomBarLevel>
 * ```
 *
 * Ordering:
 * - Items are ordered by nesting depth (outer first, inner last)
 * - This creates a deterministic left-to-right order in the bottom bar
 * - Example: Account > Shared Object > Space
 *
 * Overlays:
 * - When a button is clicked, SessionFrame toggles the openMenu state
 * - The overlay for the active item is displayed in the frame
 *
 * Registration:
 * - Items are registered imperatively with the root context via useEffect
 * - This allows items to be added dynamically, even in children of SessionFrame
 * - Items are automatically unregistered on unmount
 */
export function BottomBarLevel({
  id,
  button,
  overlay,
  buttonKey,
  overlayKey,
  onBreadcrumbClick,
  position,
  children,
}: BottomBarLevelProps) {
  const parent = useContext(BottomBarContext)

  // Calculate depth from parent
  const depth = parent ? parent.depth + 1 : 1

  const registerItem = parent?.registerItem
  const unregisterItem = parent?.unregisterItem

  const buttonStore = useRef<{
    key?: React.Key
    fn: BottomBarLevelProps['button']
  }>({
    key: buttonKey,
    fn: button,
  })
  // eslint-disable-next-line react-hooks/refs
  buttonStore.current.key = buttonKey
  // eslint-disable-next-line react-hooks/refs
  buttonStore.current.fn = button

  const overlayStore = useRef<{ key?: React.Key; node?: React.ReactNode }>({
    key: overlayKey,
    node: overlay,
  })
  // eslint-disable-next-line react-hooks/refs
  overlayStore.current.key = overlayKey
  // eslint-disable-next-line react-hooks/refs
  overlayStore.current.node = overlay

  const renderButton = useCallback(
    (selected: boolean, onClick: () => void, className?: string) =>
      buttonStore.current.fn(selected, onClick, className),
    [],
  )

  const renderOverlay = useCallback(() => overlayStore.current.node, [])

  const hasOverlay = overlay !== undefined

  // Use ref for onBreadcrumbClick to keep the item stable
  const breadcrumbStore = useRef(onBreadcrumbClick)
  // eslint-disable-next-line react-hooks/refs
  breadcrumbStore.current = onBreadcrumbClick

  const renderBreadcrumbClick = useCallback(() => {
    breadcrumbStore.current?.()
  }, [])

  const hasBreadcrumbClick = !!onBreadcrumbClick

  const item = useMemo(() => {
    return {
      id,
      depth,
      button: renderButton,
      buttonKey,
      overlay: hasOverlay ? renderOverlay : undefined,
      overlayKey,
      onBreadcrumbClick: hasBreadcrumbClick ? renderBreadcrumbClick : undefined,
      position,
    }
  }, [
    id,
    depth,
    renderButton,
    renderOverlay,
    hasOverlay,
    buttonKey,
    overlayKey,
    hasBreadcrumbClick,
    renderBreadcrumbClick,
    position,
  ])

  useEffect(() => {
    if (!registerItem || !unregisterItem) {
      console.warn(
        'BottomBarLevel must be used inside a BottomBarRoot provider',
      )
      return
    }

    registerItem(item)

    return () => {
      unregisterItem(id)
    }
  }, [id, item, registerItem, unregisterItem])

  // Provide context for nested children
  const value: BottomBarContextValue = useMemo(
    () => ({
      parent,
      depth,
      registerItem: parent?.registerItem ?? (() => {}),
      unregisterItem: parent?.unregisterItem ?? (() => {}),
      getRoot: parent?.getRoot ?? (() => null),
    }),
    [parent, depth],
  )

  return (
    <BottomBarContext.Provider value={value}>
      {children}
    </BottomBarContext.Provider>
  )
}
