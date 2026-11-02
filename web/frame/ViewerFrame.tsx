import React, { useCallback, useMemo, useRef, useState } from 'react'

import { usePhoneShell } from '@s4wave/web/hooks/usePhoneShell.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'

import { Frame } from './frame.js'
import { BottomBarBreadcrumbSeparator } from './breadcrumb-separator.js'
import {
  type BottomBarContextMenuAction,
  type BottomBarContextMenuItem,
  type BottomBarContextMenuOpenKind,
  type BottomBarItem,
  useBottomBarItems,
  useBottomBarOpenMenu,
  useBottomBarOverlayHostRef,
  useBottomBarSetOpenMenu,
} from './bottom-bar-context.js'
import {
  BottomBarItem as BottomBarButton,
  type BottomBarSecondaryActivation,
  type IBottomBarItemProps,
} from './bottom-bar-item.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { paddingInline: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule2: { backgroundColor: themeColors['--color-bar-item-selected'] },
  rule3: { maxWidth: `calc(${themeLayout['--spacing']} * 72)` },
  rule4: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule5: {
    maxWidth: `calc(${themeLayout['--spacing']} * 72)`,
    minWidth: `calc(${themeLayout['--spacing']} * 44)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule7: { minWidth: 0, flexShrink: 1 },
  rule8: { display: 'contents' },
})

// ViewerFrameProps are properties for ViewerFrame.
export interface ViewerFrameProps {
  right?: React.ReactNode
  className?: string
  children?: React.ReactNode
}

interface BottomBarContextMenuState {
  itemId: string
  x: number
  y: number
  openKind: BottomBarContextMenuOpenKind
}

interface BottomBarButtonSlotProps {
  item: BottomBarItem
  openMenu: string
  setOpenMenu: (id: string) => void
  openContextMenu?: (
    item: BottomBarItem,
    activation: BottomBarSecondaryActivation,
  ) => void
  contextMenuOpen?: boolean
  className?: string
}

function BottomBarButtonSlot({
  item,
  openMenu,
  setOpenMenu,
  openContextMenu,
  contextMenuOpen,
  className,
}: BottomBarButtonSlotProps) {
  const selected = openMenu === item.id
  const button = item.button(
    selected,
    () => setOpenMenu(selected ? '' : item.id),
    cn(selected && 'bg-bar-item-selected', className),
  )
  const hasContextMenu = (item.contextMenuItems?.length ?? 0) > 0
  if (!hasContextMenu || !openContextMenu || !React.isValidElement(button)) {
    return button
  }
  if (button.type !== BottomBarButton) return button

  return React.cloneElement(button as React.ReactElement<IBottomBarItemProps>, {
    onSecondaryActivate: (activation: BottomBarSecondaryActivation) => {
      openContextMenu(item, activation)
    },
    contextMenuOpen,
  })
}

function CollapsedBottomBarItems({
  items,
  openMenu,
  setOpenMenu,
  openContextMenu,
  contextMenuOpenItemId,
}: {
  items: BottomBarItem[]
  openMenu: string
  setOpenMenu: (id: string) => void
  openContextMenu?: (
    item: BottomBarItem,
    activation: BottomBarSecondaryActivation,
  ) => void
  contextMenuOpenItemId?: string
}) {
  const selected = items.some((item) => item.id === openMenu)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <BottomBarButton
          selected={selected}
          {...stylex.props([
            stylexStyles.rule1,
            !!selected && stylexStyles.rule2,
          ])}
          aria-label="Open hidden bottom bar items"
        >
          ...
        </BottomBarButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" sx={stylexStyles.rule3}>
        {items.map((item) => {
          const itemSelected = openMenu === item.id
          const hasContextMenu = (item.contextMenuItems?.length ?? 0) > 0
          return (
            <DropdownMenuItem
              key={item.id}
              onSelect={() => setOpenMenu(itemSelected ? '' : item.id)}
              aria-haspopup={hasContextMenu ? 'menu' : undefined}
              aria-expanded={
                hasContextMenu ? contextMenuOpenItemId === item.id : undefined
              }
              onContextMenu={(event) => {
                if (!hasContextMenu || !openContextMenu) return
                event.preventDefault()
                event.stopPropagation()
                openContextMenu(item, {
                  openKind: 'mouse',
                  x: event.clientX,
                  y: event.clientY,
                  trigger: event.currentTarget,
                })
              }}
              onKeyDown={(event) => {
                if (!hasContextMenu || !openContextMenu) return
                const keyboardContextMenu =
                  event.key === 'ContextMenu' ||
                  (event.shiftKey && event.key === 'F10')
                if (!keyboardContextMenu) return
                event.preventDefault()
                event.stopPropagation()
                const rect = event.currentTarget.getBoundingClientRect()
                openContextMenu(item, {
                  openKind: 'keyboard',
                  x: rect.left + rect.width / 2,
                  y: rect.top,
                  trigger: event.currentTarget,
                })
              }}
              variant={itemSelected ? 'selected' : 'default'}
            >
              <span sx={stylexStyles.rule4}>{item.menuLabel ?? item.id}</span>
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function BottomBarContextMenu({
  item,
  state,
  setOpenMenu,
  onClose,
  returnFocusRef,
}: {
  item: BottomBarItem | undefined
  state: BottomBarContextMenuState | null
  setOpenMenu: (id: string) => void
  onClose: () => void
  returnFocusRef: React.RefObject<HTMLElement | null>
}) {
  const items = item?.contextMenuItems ?? []
  const open = !!item && !!state && items.length > 0

  return (
    <DropdownMenu
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onClose()
      }}
    >
      <DropdownMenuTrigger asChild>
        <DropdownMenuGhostAnchor x={state?.x ?? 0} y={state?.y ?? 0} />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="top"
        align="center"
        sx={stylexStyles.rule5}
        aria-label={item?.contextMenuLabel ?? `${item?.id ?? ''} actions`}
        onCloseAutoFocus={(event) => {
          const trigger = returnFocusRef.current
          if (!trigger) return
          event.preventDefault()
          trigger.focus({ preventScroll: true })
          returnFocusRef.current = null
        }}
      >
        {items.map((menuItem) =>
          renderContextMenuItem(menuItem, item, state, setOpenMenu, onClose),
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function renderContextMenuItem(
  menuItem: BottomBarContextMenuItem,
  item: BottomBarItem | undefined,
  state: BottomBarContextMenuState | null,
  setOpenMenu: (id: string) => void,
  onClose: () => void,
): React.ReactNode {
  switch (menuItem.type) {
    case 'separator':
      return <DropdownMenuSeparator key={menuItem.id} />
    case 'group':
      return (
        <DropdownMenuSub key={menuItem.id}>
          <DropdownMenuSubTrigger disabled={menuItem.disabled}>
            {menuItem.label}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {menuItem.items.map((child) =>
              renderContextMenuItem(child, item, state, setOpenMenu, onClose),
            )}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      )
    case 'action':
      return (
        <BottomBarContextMenuActionItem
          key={menuItem.id}
          action={menuItem}
          item={item}
          state={state}
          setOpenMenu={setOpenMenu}
          onClose={onClose}
        />
      )
  }
}

function BottomBarContextMenuActionItem({
  action,
  item,
  state,
  setOpenMenu,
  onClose,
}: {
  action: BottomBarContextMenuAction
  item: BottomBarItem | undefined
  state: BottomBarContextMenuState | null
  setOpenMenu: (id: string) => void
  onClose: () => void
}) {
  const Icon = action.icon

  return (
    <DropdownMenuItem
      disabled={action.disabled}
      variant={action.variant}
      onSelect={() => {
        if (!item || !state) return
        void Promise.resolve(
          action.onSelect({
            itemId: item.id,
            openKind: state.openKind,
            closeMenu: onClose,
            openPrimaryOverlay: () => setOpenMenu(item.id),
          }),
        ).catch((err) => {
          console.warn('bottom bar context menu action failed', err)
        })
      }}
    >
      {Icon ? <Icon sx={stylexStyles.rule6} /> : null}
      <span sx={stylexStyles.rule4}>{action.label}</span>
      {action.shortcut ? (
        <DropdownMenuShortcut>{action.shortcut}</DropdownMenuShortcut>
      ) : null}
    </DropdownMenuItem>
  )
}

// ViewerFrame renders bottom bar items with breadcrumb separators and an overlay.
// Extracted from SessionFrame for reuse in standalone ObjectViewer contexts.
export function ViewerFrame(props: ViewerFrameProps) {
  const items = useBottomBarItems()
  const phoneShell = usePhoneShell()
  const openMenu = useBottomBarOpenMenu() ?? ''
  const setOpenMenu = useBottomBarSetOpenMenu() ?? (() => {})
  const overlayHostRef = useBottomBarOverlayHostRef()
  const [contextMenuState, setContextMenuState] =
    useState<BottomBarContextMenuState | null>(null)
  const contextMenuTriggerRef = useRef<HTMLElement | null>(null)

  const openContextMenu = useCallback(
    (item: BottomBarItem, activation: BottomBarSecondaryActivation) => {
      contextMenuTriggerRef.current = activation.trigger
      setContextMenuState({
        itemId: item.id,
        x: activation.x,
        y: activation.y,
        openKind: activation.openKind,
      })
    },
    [],
  )

  const closeContextMenu = useCallback(() => {
    setContextMenuState(null)
  }, [])

  const leftItems = useMemo(
    () => items.filter((item) => item.position !== 'right'),
    [items],
  )
  const rightItems = useMemo(
    () => items.filter((item) => item.position === 'right'),
    [items],
  )

  // A phone has room for the last breadcrumb only: the rest open from the
  // collapsed menu. That breadcrumb gives up label width before a control
  // inside it, such as the viewer selector, is cut off.
  const headCount = phoneShell ? 0 : 1

  const left =
    leftItems.length > (phoneShell ? 1 : 3) ? (
      (() => {
        const head = leftItems.slice(0, headCount)
        const last = leftItems[leftItems.length - 1]
        const middle = leftItems.slice(headCount, -1)
        const beforeLast = middle[middle.length - 1]

        return (
          <>
            {head.map((item) => (
              <React.Fragment key={item.id}>
                <BottomBarButtonSlot
                  item={item}
                  openMenu={openMenu}
                  setOpenMenu={setOpenMenu}
                  openContextMenu={openContextMenu}
                  contextMenuOpen={contextMenuState?.itemId === item.id}
                />
                <BottomBarBreadcrumbSeparator
                  onClick={item.onBreadcrumbClick}
                />
              </React.Fragment>
            ))}
            <CollapsedBottomBarItems
              items={middle}
              openMenu={openMenu}
              setOpenMenu={setOpenMenu}
              openContextMenu={openContextMenu}
              contextMenuOpenItemId={contextMenuState?.itemId}
            />
            <BottomBarBreadcrumbSeparator
              onClick={beforeLast.onBreadcrumbClick}
            />
            <BottomBarButtonSlot
              item={last}
              openMenu={openMenu}
              setOpenMenu={setOpenMenu}
              openContextMenu={openContextMenu}
              contextMenuOpen={contextMenuState?.itemId === last.id}
              {...stylex.props(phoneShell ? stylexStyles.rule7 : undefined)}
            />
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
              <BottomBarButtonSlot
                item={item}
                openMenu={openMenu}
                setOpenMenu={setOpenMenu}
                openContextMenu={openContextMenu}
                contextMenuOpen={contextMenuState?.itemId === item.id}
                {...stylex.props(
                  index === leftItems.length - 1
                    ? phoneShell
                      ? stylexStyles.rule7
                      : undefined
                    : undefined,
                )}
              />
            </React.Fragment>
          )
        })}
      </>
    )

  const right = (
    <>
      {rightItems.map((item) => {
        return (
          <React.Fragment key={item.id}>
            <BottomBarButtonSlot
              item={item}
              openMenu={openMenu}
              setOpenMenu={setOpenMenu}
              openContextMenu={openContextMenu}
              contextMenuOpen={contextMenuState?.itemId === item.id}
            />
          </React.Fragment>
        )
      })}
      {props.right}
    </>
  )

  const activeItem = items.find((item) => item.id === openMenu)
  // The open item's level portals its overlay into this host element.
  const activeOverlay = activeItem?.hasOverlay ? (
    <div ref={overlayHostRef} sx={stylexStyles.rule8} />
  ) : undefined
  const contextMenuItem = contextMenuState
    ? items.find((item) => item.id === contextMenuState.itemId)
    : undefined

  return (
    <>
      <Frame
        className={
          /* TODO(stylex): dynamic class or forwarded className */ props.className
        }
        bottomBar={{
          className: 'viewer-bottom-bar px-1',
          left,
          right,
        }}
        overlay={activeOverlay}
        overlayLabel={
          typeof activeItem?.menuLabel === 'string'
            ? activeItem.menuLabel
            : activeItem?.id
        }
        onCloseOverlay={() => setOpenMenu('')}
      >
        {props.children}
      </Frame>
      <BottomBarContextMenu
        item={contextMenuItem}
        state={contextMenuState}
        setOpenMenu={setOpenMenu}
        onClose={closeContextMenu}
        returnFocusRef={contextMenuTriggerRef}
      />
    </>
  )
}
