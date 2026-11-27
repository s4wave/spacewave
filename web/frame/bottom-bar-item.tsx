import React, {
  CSSProperties,
  DOMAttributes,
  KeyboardEvent,
  MouseEvent,
  PointerEvent,
  useCallback,
  useEffect,
  useRef,
  type Ref,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import type { BottomBarContextMenuOpenKind } from './bottom-bar-context.js'

export interface BottomBarSecondaryActivation {
  openKind: BottomBarContextMenuOpenKind
  x: number
  y: number
  trigger: HTMLElement
}

export interface IBottomBarItemProps extends DOMAttributes<HTMLButtonElement> {
  selected?: boolean
  disabled?: boolean
  children?: React.ReactNode
  // trailing is a control beside the button, such as a menu trigger. It shares
  // the item's look but is not nested in the button, which cannot hold one.
  trailing?: React.ReactNode
  className?: string
  style?: CSSProperties
  onSecondaryActivate?: (activation: BottomBarSecondaryActivation) => void
  contextMenuOpen?: boolean
  ref?: Ref<HTMLButtonElement>
}

const longPressDelayMs = 550
const longPressMoveTolerancePx = 8

export function BottomBarItem({
  children,
  trailing,
  style,
  onClick,
  onSecondaryActivate,
  contextMenuOpen,
  disabled,
  selected,
  className,
  ref,
  onKeyDown,
  onContextMenu,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onPointerCancel,
  onPointerLeave,
  ...rest
}: IBottomBarItemProps) {
  // Retain one pending gesture and suppress its following click.
  const longPressRef = useRef<{
    timer: ReturnType<typeof setTimeout>
    pointerId: number
    x: number
    y: number
    trigger: HTMLElement
  } | null>(null)
  const suppressNextClickRef = useRef(false)

  // Provide one cancellation path for pointer movement, scrolling and teardown.
  const cancelLongPress = useCallback(function cancelLongPress() {
    // Release the pending gesture and its scroll listener.
    const pending = longPressRef.current
    if (!pending) return
    clearTimeout(pending.timer)
    longPressRef.current = null
    if (typeof window !== 'undefined') {
      window.removeEventListener('scroll', cancelLongPress, true)
    }
  }, [])

  // Release the gesture when this item unmounts.
  useEffect(
    () => () => {
      // Discard the pending gesture before its callback can fire.
      const pending = longPressRef.current
      if (!pending) return
      clearTimeout(pending.timer)
      longPressRef.current = null
      if (typeof window !== 'undefined') {
        window.removeEventListener('scroll', cancelLongPress, true)
      }
    },
    [cancelLongPress],
  )

  // Report the secondary activation with its trigger and coordinates.
  const openSecondaryMenu = (
    openKind: BottomBarContextMenuOpenKind,
    x: number,
    y: number,
    trigger: HTMLElement,
  ) => {
    onSecondaryActivate?.({ openKind, x, y, trigger })
  }

  // Translate keyboard activation into the corresponding menu or click.
  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    // Let the caller consume the keyboard event first.
    onKeyDown?.(event)
    if (event.defaultPrevented) return

    if (
      onSecondaryActivate &&
      (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10'))
    ) {
      event.preventDefault()
      const rect = event.currentTarget.getBoundingClientRect()
      openSecondaryMenu(
        'keyboard',
        rect.left + rect.width / 2,
        rect.top,
        event.currentTarget,
      )
      return
    }

    if (onClick && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault()
      event.currentTarget.click()
    }
  }

  // Consume the click that follows a completed long press.
  const handlePrimaryActivate = (event: MouseEvent<HTMLButtonElement>) => {
    if (suppressNextClickRef.current) {
      suppressNextClickRef.current = false
      event.preventDefault()
      event.stopPropagation()
      return
    }
    onClick?.(event)
  }

  // Translate the native context menu into the item menu.
  const handleContextMenu = (event: MouseEvent<HTMLButtonElement>) => {
    // Let the caller consume the context menu first.
    onContextMenu?.(event)
    if (!onSecondaryActivate || event.defaultPrevented) return

    // Replace the native menu and cancel any pending touch gesture.
    event.preventDefault()
    event.stopPropagation()
    cancelLongPress()
    openSecondaryMenu(
      'mouse',
      event.clientX,
      event.clientY,
      event.currentTarget,
    )
  }

  // Start a long press only for an unhandled touch or pen event.
  const handlePointerDown = (event: PointerEvent<HTMLButtonElement>) => {
    // Let the caller consume the pointer event first.
    onPointerDown?.(event)
    if (
      !onSecondaryActivate ||
      event.defaultPrevented ||
      event.pointerType === 'mouse'
    ) {
      return
    }

    // Schedule secondary activation for the initiating pointer.
    cancelLongPress()
    const trigger = event.currentTarget
    const x = event.clientX
    const y = event.clientY
    const pointerId = event.pointerId
    const timer = setTimeout(() => {
      // Activate only the gesture that still owns the timer.
      const pending = longPressRef.current
      if (!pending || pending.pointerId !== pointerId) return
      longPressRef.current = null
      suppressNextClickRef.current = true
      openSecondaryMenu('touch', x, y, trigger)
    }, longPressDelayMs)

    // Cancel this gesture if scrolling starts.
    longPressRef.current = { timer, pointerId, x, y, trigger }
    if (typeof window !== 'undefined') {
      window.addEventListener('scroll', cancelLongPress, true)
    }
  }

  // Cancel a long press when its pointer moves outside the tolerance.
  const handlePointerMove = (event: PointerEvent<HTMLButtonElement>) => {
    // Track movement only for the pointer that started the gesture.
    onPointerMove?.(event)
    const pending = longPressRef.current
    if (!pending || pending.pointerId !== event.pointerId) return
    const dx = Math.abs(event.clientX - pending.x)
    const dy = Math.abs(event.clientY - pending.y)
    if (dx > longPressMoveTolerancePx || dy > longPressMoveTolerancePx) {
      cancelLongPress()
    }
  }

  // Release the gesture when the pointer is lifted.
  const handlePointerUp = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerUp?.(event)
    cancelLongPress()
  }

  // Release the gesture when the platform cancels the pointer.
  const handlePointerCancel = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerCancel?.(event)
    cancelLongPress()
  }

  // Release the gesture when the pointer leaves the item.
  const handlePointerLeave = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerLeave?.(event)
    cancelLongPress()
  }

  // Apply the shared target size and selected appearance.
  const itemClassName = cn(
    'glow-on-hover text-bar-item-text hover:text-bar-item-text-hover relative flex h-full min-w-6 shrink-0 flex-row items-center justify-start overflow-hidden whitespace-pre select-none [&>svg]:h-3 [&>svg]:w-3 [&>svg:not(:only-child)]:mr-1',
    trailing ? 'pr-1' : 'px-1',
    disabled ? 'cursor-not-allowed' : 'cursor-pointer',
    selected &&
      'bg-bar-item-selected text-bar-item-selected-text text-shadow-bar-item-selected shadow-bar-item-selected',
    className,
  )

  // Render one button, with a sibling control when requested.
  const button = (
    <button
      ref={ref}
      type="button"
      onClick={handlePrimaryActivate}
      onKeyDown={handleKeyDown}
      onContextMenu={handleContextMenu}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
      onPointerLeave={handlePointerLeave}
      aria-disabled={disabled}
      aria-pressed={selected}
      aria-haspopup={onSecondaryActivate ? 'menu' : undefined}
      aria-expanded={onSecondaryActivate ? !!contextMenuOpen : undefined}
      {...rest}
      className={
        trailing
          ? cn(
              'flex h-full min-w-0 flex-row items-center pl-1 whitespace-pre',
              disabled ? 'cursor-not-allowed' : 'cursor-pointer',
            )
          : itemClassName
      }
      style={trailing ? undefined : style}
    >
      {children}
    </button>
  )
  if (!trailing) return button

  return (
    <div className={itemClassName} style={style}>
      {button}
      {trailing}
    </div>
  )
}
