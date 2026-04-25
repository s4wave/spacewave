import React, { useMemo } from 'react'

import { Frame } from './frame.js'
import { BottomBarBreadcrumbSeparator } from './breadcrumb-separator.js'
import {
  useBottomBarItems,
  useBottomBarOpenMenu,
  useBottomBarSetOpenMenu,
} from './bottom-bar-context.js'
import { cn } from '@s4wave/web/style/utils.js'

// ViewerFrameProps are properties for ViewerFrame.
export interface ViewerFrameProps {
  right?: React.ReactNode
  className?: string
  children?: React.ReactNode
}

// ViewerFrame renders bottom bar items with breadcrumb separators and an overlay.
// Extracted from SessionFrame for reuse in standalone ObjectViewer contexts.
export function ViewerFrame(props: ViewerFrameProps) {
  const items = useBottomBarItems()
  const openMenu = useBottomBarOpenMenu() ?? ''
  const setOpenMenu = useBottomBarSetOpenMenu() ?? (() => {})

  const leftItems = useMemo(
    () => items.filter((item) => item.position !== 'right'),
    [items],
  )
  const rightItems = useMemo(
    () => items.filter((item) => item.position === 'right'),
    [items],
  )

  const left = (
    <>
      {leftItems.map(({ id, button }, index) => {
        const selected = openMenu === id
        const prevItemHandler =
          index > 0 ? leftItems[index - 1].onBreadcrumbClick : undefined
        return (
          <React.Fragment key={id}>
            {index > 0 && (
              <BottomBarBreadcrumbSeparator onClick={prevItemHandler} />
            )}
            {button(
              selected,
              () => setOpenMenu(selected ? '' : id),
              cn(selected && 'bg-bar-item-selected'),
            )}
          </React.Fragment>
        )
      })}
    </>
  )

  const right = (
    <>
      {rightItems.map(({ id, button }) => {
        const selected = openMenu === id
        return (
          <React.Fragment key={id}>
            {button(
              selected,
              () => setOpenMenu(selected ? '' : id),
              cn(selected && 'bg-bar-item-selected'),
            )}
          </React.Fragment>
        )
      })}
      {props.right}
    </>
  )

  const activeOverlay = items.find((item) => item.id === openMenu)?.overlay?.()

  return (
    <Frame
      className={props.className}
      bottomBar={{
        className: 'px-1',
        left,
        right,
      }}
      overlay={activeOverlay}
      onCloseOverlay={() => setOpenMenu('')}
    >
      {props.children}
    </Frame>
  )
}
