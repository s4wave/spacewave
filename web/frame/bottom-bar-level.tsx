import React, { use, useEffect, useMemo } from 'react'
import {
  BottomBarContext,
  BottomBarContextValue,
  type BottomBarContextMenuItem,
} from './bottom-bar-context.js'

export interface BottomBarLevelProps {
  id: string
  button: (
    selected: boolean,
    onClick: () => void,
    className?: string,
  ) => React.ReactNode
  overlay?: React.ReactNode
  buttonKey?: React.Key
  overlayKey?: React.Key
  menuLabel?: React.ReactNode
  contextMenuLabel?: string
  contextMenuKey?: React.Key
  contextMenuItems?: readonly BottomBarContextMenuItem[]
  onBreadcrumbClick?: () => void
  position?: 'left' | 'right'
  children: React.ReactNode
}

export function BottomBarLevel({
  id,
  button,
  overlay,
  buttonKey,
  overlayKey,
  menuLabel,
  contextMenuLabel,
  contextMenuKey,
  contextMenuItems,
  onBreadcrumbClick,
  position,
  children,
}: BottomBarLevelProps) {
  const parent = use(BottomBarContext)
  const depth = parent ? parent.depth + 1 : 1

  const registerItem = parent?.registerItem
  const unregisterItem = parent?.unregisterItem

  const hasOverlay = overlay !== undefined
  const hasBreadcrumbClick = !!onBreadcrumbClick

  const item = useMemo(() => {
    return {
      id,
      depth,
      button,
      buttonKey,
      overlay: hasOverlay ? () => overlay : undefined,
      overlayKey,
      menuLabel,
      contextMenuLabel,
      contextMenuKey,
      contextMenuItems,
      onBreadcrumbClick: hasBreadcrumbClick ? onBreadcrumbClick : undefined,
      position,
    }
  }, [
    id,
    depth,
    button,
    hasOverlay,
    overlay,
    buttonKey,
    overlayKey,
    menuLabel,
    contextMenuLabel,
    contextMenuKey,
    contextMenuItems,
    hasBreadcrumbClick,
    onBreadcrumbClick,
    position,
  ])

  useEffect(() => {
    if (!registerItem) {
      console.warn(
        'BottomBarLevel must be used inside a BottomBarRoot provider',
      )
      return
    }

    registerItem(item)
  }, [item, registerItem])

  useEffect(() => {
    if (!unregisterItem) return
    return () => {
      unregisterItem(id)
    }
  }, [id, unregisterItem])

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
