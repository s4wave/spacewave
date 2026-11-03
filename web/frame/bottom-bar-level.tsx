import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import React, { use, useEffect, useMemo } from 'react'
import { createPortal } from 'react-dom'
import {
  BottomBarContext,
  BottomBarContextValue,
  type BottomBarContextMenuItem,
  type BottomBarItem,
  useBottomBarOpenMenu,
  useBottomBarOverlayHost,
} from './bottom-bar-context.js'

export interface BottomBarLevelProps {
  id: string
  button: (
    selected: boolean,
    onClick: () => void,
    sx?: BottomBarStyles,
  ) => React.ReactNode
  // overlay renders in the frame while this level's menu is open. It is part
  // of this level's tree, so it reads this level's state and contexts.
  overlay?: React.ReactNode
  menuLabel?: React.ReactNode
  contextMenuLabel?: string
  contextMenuItems?: readonly BottomBarContextMenuItem[]
  onBreadcrumbClick?: () => void
  position?: 'left' | 'right'
  children: React.ReactNode
}

export function BottomBarLevel({
  id,
  button,
  overlay,
  menuLabel,
  contextMenuLabel,
  contextMenuItems,
  onBreadcrumbClick,
  position,
  children,
}: BottomBarLevelProps) {
  const parent = use(BottomBarContext)
  const depth = parent ? parent.depth + 1 : 1

  const registerItem = parent?.registerItem
  const unregisterItem = parent?.unregisterItem

  // The item changes only when something the frame renders changes.
  const hasOverlay = overlay !== undefined
  const item = useMemo<BottomBarItem>(
    () => ({
      id,
      depth,
      button,
      hasOverlay,
      menuLabel,
      contextMenuLabel,
      contextMenuItems,
      onBreadcrumbClick,
      position,
    }),
    [
      id,
      depth,
      button,
      hasOverlay,
      menuLabel,
      contextMenuLabel,
      contextMenuItems,
      onBreadcrumbClick,
      position,
    ],
  )

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

  // Portal the overlay into the frame while this level's menu is open.
  const openMenu = useBottomBarOpenMenu()
  const overlayHost = useBottomBarOverlayHost()
  const portal =
    hasOverlay && openMenu === id && overlayHost
      ? createPortal(overlay, overlayHost)
      : null

  return (
    <>
      {portal}
      <BottomBarContext.Provider value={value}>
        {children}
      </BottomBarContext.Provider>
    </>
  )
}
