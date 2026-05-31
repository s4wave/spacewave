import React, { useMemo } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'

import { Frame } from './frame.js'
import { BottomBarBreadcrumbSeparator } from './breadcrumb-separator.js'
import {
  type BottomBarItem,
  useBottomBarItems,
  useBottomBarOpenMenu,
  useBottomBarSetOpenMenu,
} from './bottom-bar-context.js'
import { BottomBarItem as BottomBarButton } from './bottom-bar-item.js'

// ViewerFrameProps are properties for ViewerFrame.
export interface ViewerFrameProps {
  right?: React.ReactNode
  className?: string
  children?: React.ReactNode
}

function renderBottomBarButton(
  item: BottomBarItem,
  openMenu: string,
  setOpenMenu: (id: string) => void,
  className?: string,
) {
  const selected = openMenu === item.id
  return item.button(
    selected,
    () => setOpenMenu(selected ? '' : item.id),
    cn(selected && 'bg-bar-item-selected', className),
  )
}

function CollapsedBottomBarItems({
  items,
  openMenu,
  setOpenMenu,
}: {
  items: BottomBarItem[]
  openMenu: string
  setOpenMenu: (id: string) => void
}) {
  const selected = items.some((item) => item.id === openMenu)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <BottomBarButton
          selected={selected}
          className={cn('px-1.5', selected && 'bg-bar-item-selected')}
          aria-label="Open hidden bottom bar items"
        >
          [...]
        </BottomBarButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="max-w-72">
        {items.map((item) => {
          const itemSelected = openMenu === item.id
          return (
            <DropdownMenuItem
              key={item.id}
              onSelect={() => setOpenMenu(itemSelected ? '' : item.id)}
              className={cn(itemSelected && 'bg-accent text-accent-foreground')}
            >
              <span className="truncate">{item.menuLabel ?? item.id}</span>
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
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

  const left =
    leftItems.length > 2 ? (
      (() => {
        const first = leftItems[0]
        const last = leftItems[leftItems.length - 1]
        const middle = leftItems.slice(1, -1)
        const beforeLast = middle[middle.length - 1]

        return (
          <>
            {renderBottomBarButton(first, openMenu, setOpenMenu)}
            <BottomBarBreadcrumbSeparator onClick={first.onBreadcrumbClick} />
            <CollapsedBottomBarItems
              items={middle}
              openMenu={openMenu}
              setOpenMenu={setOpenMenu}
            />
            <BottomBarBreadcrumbSeparator
              onClick={beforeLast.onBreadcrumbClick}
            />
            {renderBottomBarButton(last, openMenu, setOpenMenu)}
          </>
        )
      })()
    ) : (
      <>
        {leftItems.map((item, index) => {
          const prevItemHandler =
            index > 0 ? leftItems[index - 1].onBreadcrumbClick : undefined
          return (
            <React.Fragment key={item.id}>
              {index > 0 && (
                <BottomBarBreadcrumbSeparator onClick={prevItemHandler} />
              )}
              {renderBottomBarButton(item, openMenu, setOpenMenu)}
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
