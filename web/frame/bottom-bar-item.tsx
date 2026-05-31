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

export interface IBottomBarItemProps extends DOMAttributes<HTMLDivElement> {
  selected?: boolean
  disabled?: boolean
  children?: React.ReactNode
  className?: string
  style?: CSSProperties
  onClick?: () => void
  onSecondaryActivate?: (activation: BottomBarSecondaryActivation) => void
  contextMenuOpen?: boolean
  ref?: Ref<HTMLDivElement>
}

const longPressDelayMs = 550
const longPressMoveTolerancePx = 8

export function BottomBarItem({
  children,
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
  const longPressRef = useRef<{
    timer: ReturnType<typeof setTimeout>
    pointerId: number
    x: number
    y: number
    trigger: HTMLElement
  } | null>(null)
  const suppressNextClickRef = useRef(false)

  const cancelLongPress = useCallback(() => {
    const pending = longPressRef.current
    if (!pending) return
    clearTimeout(pending.timer)
    longPressRef.current = null
    if (typeof window !== 'undefined') {
      window.removeEventListener('scroll', cancelLongPress, true)
    }
  }, [])

  useEffect(
    () => () => {
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

  const openSecondaryMenu = (
    openKind: BottomBarContextMenuOpenKind,
    x: number,
    y: number,
    trigger: HTMLElement,
  ) => {
    onSecondaryActivate?.({ openKind, x, y, trigger })
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
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
      onClick()
    }
  }

  const handlePrimaryActivate = (event: MouseEvent<HTMLDivElement>) => {
    if (suppressNextClickRef.current) {
      suppressNextClickRef.current = false
      event.preventDefault()
      event.stopPropagation()
      return
    }
    onClick?.()
  }

  const handleContextMenu = (event: MouseEvent<HTMLDivElement>) => {
    onContextMenu?.(event)
    if (!onSecondaryActivate || event.defaultPrevented) return

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

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    onPointerDown?.(event)
    if (
      !onSecondaryActivate ||
      event.defaultPrevented ||
      event.pointerType === 'mouse'
    ) {
      return
    }

    cancelLongPress()
    const trigger = event.currentTarget
    const x = event.clientX
    const y = event.clientY
    const pointerId = event.pointerId
    const timer = setTimeout(() => {
      const pending = longPressRef.current
      if (!pending || pending.pointerId !== pointerId) return
      longPressRef.current = null
      suppressNextClickRef.current = true
      openSecondaryMenu('touch', x, y, trigger)
    }, longPressDelayMs)

    longPressRef.current = { timer, pointerId, x, y, trigger }
    if (typeof window !== 'undefined') {
      window.addEventListener('scroll', cancelLongPress, true)
    }
  }

  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    onPointerMove?.(event)
    const pending = longPressRef.current
    if (!pending || pending.pointerId !== event.pointerId) return
    const dx = Math.abs(event.clientX - pending.x)
    const dy = Math.abs(event.clientY - pending.y)
    if (dx > longPressMoveTolerancePx || dy > longPressMoveTolerancePx) {
      cancelLongPress()
    }
  }

  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    onPointerUp?.(event)
    cancelLongPress()
  }

  const handlePointerCancel = (event: PointerEvent<HTMLDivElement>) => {
    onPointerCancel?.(event)
    cancelLongPress()
  }

  const handlePointerLeave = (event: PointerEvent<HTMLDivElement>) => {
    onPointerLeave?.(event)
    cancelLongPress()
  }

  return (
    <div
      ref={ref}
      role="button"
      tabIndex={0}
      onClick={handlePrimaryActivate}
      onKeyDown={handleKeyDown}
      onContextMenu={handleContextMenu}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
      onPointerLeave={handlePointerLeave}
      aria-disabled={disabled}
      aria-selected={selected}
      aria-haspopup={onSecondaryActivate ? 'menu' : undefined}
      aria-expanded={onSecondaryActivate ? !!contextMenuOpen : undefined}
      {...rest}
      className={cn(
        `glow-on-hover text-bar-item-text hover:text-bar-item-text-hover relative flex h-full shrink-0 cursor-pointer flex-row items-center justify-start overflow-hidden px-[5px] whitespace-pre select-none [&>svg]:h-3 [&>svg]:w-3 [&>svg:not(:only-child)]:mr-1`,
        selected &&
          'bg-bar-item-selected text-bar-item-selected-text text-shadow-bar-item-selected border-t-primary border-t',
        className,
      )}
      style={{
        cursor: disabled ? 'not-allowed' : 'pointer',
        ...style,
      }}
    >
      {children}
    </div>
  )
}
