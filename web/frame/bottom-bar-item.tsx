import * as stylex from '@stylexjs/stylex'

import { stylexProps } from '@s4wave/web/style/props.js'
import { useControlStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import {
  bottomBarIconMarker,
  iconRequired,
} from '@s4wave/web/style/icon-defaults.stylex.js'
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

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'not-allowed',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderTopColor: themeColors['--color-primary'],
    backgroundColor: themeColors['--color-bar-item-selected'],
    paddingRight: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-selected-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    textShadow: '0 0 3px var(--tw-text-shadow-color, oklch(0% 0 0 / .8))',
  },
  rule2: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'not-allowed',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderTopColor: themeColors['--color-primary'],
    backgroundColor: themeColors['--color-bar-item-selected'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-selected-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    textShadow: '0 0 3px var(--tw-text-shadow-color, oklch(0% 0 0 / .8))',
  },
  rule3: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'pointer',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderTopColor: themeColors['--color-primary'],
    backgroundColor: themeColors['--color-bar-item-selected'],
    paddingRight: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-selected-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    textShadow: '0 0 3px var(--tw-text-shadow-color, oklch(0% 0 0 / .8))',
  },
  rule4: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'pointer',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderTopColor: themeColors['--color-primary'],
    backgroundColor: themeColors['--color-bar-item-selected'],
    paddingInline: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-selected-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    textShadow: '0 0 3px var(--tw-text-shadow-color, oklch(0% 0 0 / .8))',
  },
  rule5: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'not-allowed',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    paddingRight: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'not-allowed',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'pointer',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    paddingRight: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule8: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    flexShrink: 0,
    cursor: 'pointer',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'flex-start',
    overflow: 'hidden',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.25)`,
    whiteSpace: 'pre',
    color: {
      default: themeColors['--color-bar-item-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-bar-item-text-hover'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const itemVariants = (props?: {
  state?:
    | 'idle-enabled-single'
    | 'idle-enabled-trailing'
    | 'idle-disabled-single'
    | 'idle-disabled-trailing'
    | 'selected-enabled-single'
    | 'selected-enabled-trailing'
    | 'selected-disabled-single'
    | 'selected-disabled-trailing'
    | null
  class?: stylex.StyleXStyles
  className?: stylex.StyleXStyles
}) => {
  // Resolve "state" once, retaining explicit null instead of its default.
  const variant0Input = props?.['state']
  const variant0 = variant0Input === undefined ? undefined : variant0Input

  // Select one fully compiled recipe outcome, then compose caller styles.
  return [
    variant0 === 'idle-enabled-single'
      ? stylexStyles.rule8
      : variant0 === 'idle-enabled-trailing'
        ? stylexStyles.rule7
        : variant0 === 'idle-disabled-single'
          ? stylexStyles.rule6
          : variant0 === 'idle-disabled-trailing'
            ? stylexStyles.rule5
            : variant0 === 'selected-enabled-single'
              ? stylexStyles.rule4
              : variant0 === 'selected-enabled-trailing'
                ? stylexStyles.rule3
                : variant0 === 'selected-disabled-single'
                  ? stylexStyles.rule2
                  : variant0 === 'selected-disabled-trailing'
                    ? stylexStyles.rule1
                    : false,
    props?.class,
    props?.className,
  ] as const
}
// BottomBarSecondaryActivation identifies the input and anchor for an item menu.
export interface BottomBarSecondaryActivation {
  openKind: BottomBarContextMenuOpenKind
  x: number
  y: number
  trigger: HTMLElement
}

// IBottomBarItemProps configures an item and its primary and secondary actions.
export interface IBottomBarItemProps extends DOMAttributes<HTMLButtonElement> {
  selected?: boolean
  disabled?: boolean
  children?: React.ReactNode
  // trailing is a control beside the button, such as a menu trigger. It shares
  // the item's look but is not nested in the button, which cannot hold one.
  trailing?: React.ReactNode
  sx?: stylex.StyleXStyles
  className?: never
  style?: CSSProperties
  onClick?: () => void
  onSecondaryActivate?: (activation: BottomBarSecondaryActivation) => void
  contextMenuOpen?: boolean
  ref?: Ref<HTMLButtonElement>
}

const longPressDelayMs = 550
const longPressMoveTolerancePx = 8

// BottomBarItem renders a primary action with optional sibling secondary controls.
export function BottomBarItem({
  children,
  trailing,
  style,
  onClick,
  onSecondaryActivate,
  contextMenuOpen,
  disabled,
  selected,
  sx,
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

  const cancelLongPress = useCallback(function cancelLongPress() {
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

  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
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

  const handlePrimaryActivate = (event: MouseEvent<HTMLButtonElement>) => {
    if (suppressNextClickRef.current) {
      suppressNextClickRef.current = false
      event.preventDefault()
      event.stopPropagation()
      return
    }
    onClick?.()
  }

  const handleContextMenu = (event: MouseEvent<HTMLButtonElement>) => {
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

  const handlePointerDown = (event: PointerEvent<HTMLButtonElement>) => {
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

  const handlePointerMove = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerMove?.(event)
    const pending = longPressRef.current
    if (!pending || pending.pointerId !== event.pointerId) return
    const dx = Math.abs(event.clientX - pending.x)
    const dy = Math.abs(event.clientY - pending.y)
    if (dx > longPressMoveTolerancePx || dy > longPressMoveTolerancePx) {
      cancelLongPress()
    }
  }

  const handlePointerUp = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerUp?.(event)
    cancelLongPress()
  }

  const handlePointerCancel = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerCancel?.(event)
    cancelLongPress()
  }

  const handlePointerLeave = (event: PointerEvent<HTMLButtonElement>) => {
    onPointerLeave?.(event)
    cancelLongPress()
  }

  // Keep finite item states at the root that owns the native caller styles.
  const state =
    `${selected ? 'selected' : 'idle'}-${disabled ? 'disabled' : 'enabled'}-${trailing ? 'trailing' : 'single'}` as const
  const requiredControls = useControlStyles()
  const attributes = stylexProps(
    [itemVariants({ state }), itemEffects.glow, bottomBarIconMarker, sx],
    { style },
  )
  attributes.className = cn(attributes.className, requiredControls)

  // The compound item gives its inner button the same touch target requirements.
  const buttonAttributes = trailing
    ? {
        ...stylex.props([
          itemEffects.inner,
          disabled ? itemEffects.disabled : itemEffects.enabled,
        ]),
      }
    : attributes
  if (trailing)
    buttonAttributes.className = cn(
      buttonAttributes.className,
      requiredControls,
    )

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
      {...buttonAttributes}
    >
      {children}
    </button>
  )
  if (!trailing)
    return (
      <IconStylesProvider requiredSx={iconRequired.bottomBar}>
        {button}
      </IconStylesProvider>
    )

  return (
    <div {...attributes}>
      <IconStylesProvider requiredSx={iconRequired.bottomBar}>
        {button}
        {trailing}
      </IconStylesProvider>
    </div>
  )
}

const itemEffects = stylex.create({
  glow: {
    '::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      right: 0,
      bottom: 0,
      left: 0,
      backgroundColor: 'var(--color-white)',
      opacity: { default: 0, ':hover': 0.12 },
      transitionProperty: 'opacity',
      transitionDuration: '.042s',
      transitionTimingFunction: 'ease-in-out',
      pointerEvents: 'none',
    },
  },
  inner: {
    display: 'flex',
    height: '100%',
    minWidth: 0,
    flexDirection: 'row',
    alignItems: 'center',
    paddingLeft: 'calc(var(--spacing) * 1.25)',
    whiteSpace: 'pre',
  },
  disabled: { cursor: 'not-allowed' },
  enabled: { cursor: 'pointer' },
})
