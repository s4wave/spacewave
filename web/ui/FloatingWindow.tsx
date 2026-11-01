import {
  createContext,
  useCallback,
  use,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import { LuMinus, LuX } from 'react-icons/lu'
import { isDesktop } from '@aptre/bldr'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    cursor: 'grab',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-deep']} 80%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule4: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule6: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-error'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-error-bg'],
      },
    },
  },
  rule7: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    overflow: 'hidden',
  },
})

const MIN_Y_ELECTRON = 60

// FloatingWindowManagerContext manages z-index ordering for multiple floating windows.
interface FloatingWindowManagerContextValue {
  bringToFront: (id: string) => void
  getZIndex: (id: string) => number
  register: (id: string) => void
  unregister: (id: string) => void
  subscribe: (callback: () => void) => () => void
}

const FloatingWindowManagerContext =
  createContext<FloatingWindowManagerContextValue | null>(null)

// FloatingWindowManagerProvider manages z-index for all floating windows.
export function FloatingWindowManagerProvider({
  children,
}: {
  children: ReactNode
}) {
  // Keep window stacking order and its subscribers within one manager.
  const baseZIndex = 1000
  const windowOrderRef = useRef<string[]>([])
  const subscribersRef = useRef(new Set<() => void>())

  // Notify every stacking-order subscriber after a window order change.
  const notifySubscribers = useCallback(() => {
    subscribersRef.current.forEach((cb) => cb())
  }, [])

  // Retain a stacking subscriber until its owner releases it.
  const subscribe = useCallback((callback: () => void) => {
    subscribersRef.current.add(callback)
    return () => subscribersRef.current.delete(callback)
  }, [])

  // Append a newly registered window to the stacking order.
  const register = useCallback(
    (id: string) => {
      if (!windowOrderRef.current.includes(id)) {
        windowOrderRef.current = [...windowOrderRef.current, id]
        notifySubscribers()
      }
    },
    [notifySubscribers],
  )

  // Remove a departing window and notify the remaining subscribers.
  const unregister = useCallback(
    (id: string) => {
      const prev = windowOrderRef.current
      if (prev.includes(id)) {
        windowOrderRef.current = prev.filter((wid) => wid !== id)
        notifySubscribers()
      }
    },
    [notifySubscribers],
  )

  // Move an existing window to the front of its stacking group.
  const bringToFront = useCallback(
    (id: string) => {
      // Change the order only when this registered window is behind another.
      const prev = windowOrderRef.current
      if (!prev.includes(id)) return
      if (prev[prev.length - 1] === id) return
      windowOrderRef.current = [...prev.filter((wid) => wid !== id), id]
      notifySubscribers()
    },
    [notifySubscribers],
  )

  // Resolve each registered window's position above the shared base index.
  const getZIndex = useCallback((id: string) => {
    const index = windowOrderRef.current.indexOf(id)
    return index === -1 ? baseZIndex : baseZIndex + index
  }, [])

  // Expose stable window-order operations through the manager context.
  const value = useMemo(
    () => ({ bringToFront, getZIndex, register, unregister, subscribe }),
    [bringToFront, getZIndex, register, unregister, subscribe],
  )

  return (
    <FloatingWindowManagerContext.Provider value={value}>
      {children}
    </FloatingWindowManagerContext.Provider>
  )
}

// useFloatingWindowManager returns the window manager context.
export function useFloatingWindowManager() {
  return use(FloatingWindowManagerContext)
}

export interface FloatingWindowPosition {
  x: number
  y: number
}

export interface FloatingWindowSize {
  width: number
  height: number
}

export interface FloatingWindowState {
  position: FloatingWindowPosition
  size: FloatingWindowSize
  expanded: boolean
}

export const DEFAULT_FLOATING_WINDOW_POSITION: FloatingWindowPosition = {
  x: 16,
  y: isDesktop ? MIN_Y_ELECTRON : 16,
}

export const DEFAULT_FLOATING_WINDOW_SIZE: FloatingWindowSize = {
  width: 320,
  height: 240,
}

export const DEFAULT_FLOATING_WINDOW_STATE: FloatingWindowState = {
  position: DEFAULT_FLOATING_WINDOW_POSITION,
  size: DEFAULT_FLOATING_WINDOW_SIZE,
  expanded: false,
}

export interface FloatingWindowProps {
  /** Unique identifier for this window (used for z-index management) */
  id: string
  /** Window title displayed in header */
  title: string
  /** Icon displayed before title */
  icon?: ReactNode
  /** Current window state */
  state: FloatingWindowState
  /** Callback when state changes */
  onStateChange: (state: FloatingWindowState) => void
  /** Window content */
  children: ReactNode
  /** Additional class name for the window */
  className?: string
  /** Minimum window width */
  minWidth?: number
  /** Minimum window height */
  minHeight?: number
  /** Default position (used for reset on double-click) */
  defaultPosition?: FloatingWindowPosition
  /** Default size (used for reset on double-click) */
  defaultSize?: FloatingWindowSize
  /** Called when close button is clicked (if not provided, minimize is used) */
  onClose?: () => void
  /** Test ID for the window container */
  testId?: string
}

// FloatingWindow is a reusable floating, draggable, resizable window component.
export function FloatingWindow({
  id,
  title,
  icon,
  state,
  onStateChange,
  children,
  className,
  minWidth = 200,
  minHeight = 120,
  defaultPosition = DEFAULT_FLOATING_WINDOW_POSITION,
  defaultSize = DEFAULT_FLOATING_WINDOW_SIZE,
  onClose,
  testId,
}: FloatingWindowProps) {
  // Resolve the floating panel, its manager and platform bounds.
  const manager = useFloatingWindowManager()
  const panelRef = useRef<HTMLDialogElement>(null)
  const minY = isDesktop ? MIN_Y_ELECTRON : 0

  // Register/unregister with manager
  useEffect(() => {
    manager?.register(id)
    return () => manager?.unregister(id)
  }, [manager, id])

  // Refs for drag/resize - update DOM directly, persist on mouseup
  const dragOffset = useRef({ x: 0, y: 0 })
  const current = useRef({ pos: state.position, size: state.size })

  // Sync ref from props when they change
  useEffect(() => {
    current.current = { pos: state.position, size: state.size }
  }, [state.position, state.size])

  // Apply the current drag geometry directly to the mounted dialog.
  const applyStyle = useCallback(() => {
    const el = panelRef.current
    if (!el) return
    const { pos, size } = current.current
    Object.assign(el.style, {
      left: `${pos.x}px`,
      top: `${pos.y}px`,
      width: `${size.width}px`,
      height: `${size.height}px`,
    })
  }, [])

  // Collapse the window through its existing state callback.
  const handleMinimize = useCallback(() => {
    onStateChange({ ...state, expanded: false })
  }, [state, onStateChange])

  // Restore the window's configured position and size.
  const handleDoubleClick = useCallback(() => {
    onStateChange({
      ...state,
      position: defaultPosition,
      size: defaultSize,
    })
  }, [state, onStateChange, defaultPosition, defaultSize])

  // Focus within the window, by pointer or keyboard, raises it.
  const handleFocus = useCallback(() => {
    manager?.bringToFront(id)
  }, [manager, id])

  // Begin dragging the active window from the pointer's current offset.
  const handleDragStart = useCallback(
    (e: React.MouseEvent) => {
      // Bring the dragged window forward and retain its pointer offset.
      e.preventDefault()
      manager?.bringToFront(id)
      const { pos } = current.current
      dragOffset.current = { x: e.clientX - pos.x, y: e.clientY - pos.y }

      // Keep a usable part of the dragged window inside the viewport.
      const maxX = window.innerWidth - 100
      const maxY = window.innerHeight - 100

      // Update the drag position within the current viewport bounds.
      const onMove = (e: MouseEvent) => {
        current.current.pos = {
          x: Math.max(0, Math.min(maxX, e.clientX - dragOffset.current.x)),
          y: Math.max(minY, Math.min(maxY, e.clientY - dragOffset.current.y)),
        }
        applyStyle()
      }

      // Release drag listeners and publish the completed window geometry.
      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        const { pos, size } = current.current
        onStateChange({ ...state, position: pos, size })
      }

      // Track pointer movement only for this active drag.
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
    },
    [state, onStateChange, minY, applyStyle, manager, id],
  )

  // Resize the active window from the selected edge.
  const handleResizeStart = useCallback(
    (edge: string) => (e: React.MouseEvent) => {
      // Keep resize gestures on the active window without bubbling them to its parent.
      e.preventDefault()
      e.stopPropagation()
      manager?.bringToFront(id)

      // Retain the pointer and window geometry at the start of this resize.
      const startX = e.clientX
      const startY = e.clientY
      const startPos = { ...current.current.pos }
      const startSize = { ...current.current.size }

      // Resolve viewport bounds before tracking the active resize.
      const maxWidth = window.innerWidth - startPos.x - 20
      const maxHeight = window.innerHeight - startPos.y - 20

      // Update the selected edges from the current pointer displacement.
      const onMove = (e: MouseEvent) => {
        // Resolve the resize displacement against the original position and size.
        const dx = e.clientX - startX
        const dy = e.clientY - startY
        const pos = { ...startPos }
        const size = { ...startSize }

        // Constrain every resized edge to the window's minimum size and viewport.
        if (edge.includes('e')) {
          size.width = Math.max(
            minWidth,
            Math.min(maxWidth, startSize.width + dx),
          )
        }
        if (edge.includes('w')) {
          const newWidth = Math.max(minWidth, startSize.width - dx)
          pos.x = startPos.x + startSize.width - newWidth
          size.width = newWidth
        }
        if (edge.includes('s')) {
          size.height = Math.max(
            minHeight,
            Math.min(maxHeight, startSize.height + dy),
          )
        }
        if (edge.includes('n')) {
          const newHeight = Math.max(minHeight, startSize.height - dy)
          const newY = startPos.y + startSize.height - newHeight
          pos.y = Math.max(minY, newY)
          size.height =
            newY < minY ? startPos.y + startSize.height - minY : newHeight
        }

        // Apply the current resize geometry without committing each pointer movement.
        current.current = { pos, size }
        applyStyle()
      }

      // Release resize listeners and publish the completed window geometry.
      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        const { pos, size } = current.current
        onStateChange({ ...state, position: pos, size })
      }

      // Track pointer movement only for this active resize.
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
    },
    [state, onStateChange, minWidth, minHeight, minY, applyStyle, manager, id],
  )

  // Subscribe to z-index changes via useSyncExternalStore
  const subscribeToManager = useCallback(
    (callback: () => void) => {
      if (!manager) return () => {}
      return manager.subscribe(callback)
    },
    [manager],
  )

  // Read this window's current stacking position from its manager.
  const getZIndexSnapshot = useCallback(() => {
    return manager?.getZIndex(id) ?? 1000
  }, [manager, id])

  // Watch the manager's stacking changes through its shared subscription.
  const zIndex = useSyncExternalStore(
    subscribeToManager,
    getZIndexSnapshot,
    getZIndexSnapshot,
  )

  return (
    <dialog
      ref={panelRef}
      open
      aria-label={title}
      className={
        /* TODO(stylex): global or unknown class: floating-window-position */ cn(
          'floating-window-position fixed m-0 flex flex-col overflow-hidden p-0 text-inherit outline-none',
          'rounded-lg shadow-lg',
          'bg-background-menu/95 backdrop-blur-sm',
          'border-popover-border border',
          className,
        )
      }
      style={{
        '--floating-window-left': `${state.position.x}px`,
        '--floating-window-top': `${state.position.y}px`,
        '--floating-window-width': `${state.size.width}px`,
        '--floating-window-height': `${state.size.height}px`,
        '--floating-window-z-index': zIndex,
      }}
      tabIndex={-1}
      onFocus={handleFocus}
      data-testid={testId}
    >
      {/* Header */}
      <div
        role="presentation"
        sx={[stylexStyles.rule1]}
        onMouseDown={handleDragStart}
        onDoubleClick={handleDoubleClick}
      >
        <div sx={stylexStyles.rule2}>
          {icon && (
            <span
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: [&>svg]:h-3 */ 'text-brand flex size-3 items-center justify-center [&>svg]:h-3 [&>svg]:w-3'
              }
            >
              {icon}
            </span>
          )}
          <span
            className={
              /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs font-medium'
            }
          >
            {title}
          </span>
        </div>
        <div sx={stylexStyles.rule3}>
          <button
            type="button"
            onClick={handleMinimize}
            aria-label="Minimize"
            sx={[stylexStyles.rule4]}
          >
            <LuMinus {...stylex.props(stylexStyles.rule5)} />
          </button>
          <button
            type="button"
            onClick={onClose ?? handleMinimize}
            aria-label="Close"
            sx={[stylexStyles.rule6]}
          >
            <LuX {...stylex.props(stylexStyles.rule5)} />
          </button>
        </div>
      </div>

      {/* Content */}
      <div sx={stylexStyles.rule7}>{children}</div>

      {/* Resize handles */}
      <ResizeHandle edge="n" onMouseDown={handleResizeStart('n')} />
      <ResizeHandle edge="s" onMouseDown={handleResizeStart('s')} />
      <ResizeHandle edge="e" onMouseDown={handleResizeStart('e')} />
      <ResizeHandle edge="w" onMouseDown={handleResizeStart('w')} />
      <ResizeHandle edge="ne" onMouseDown={handleResizeStart('ne')} />
      <ResizeHandle edge="nw" onMouseDown={handleResizeStart('nw')} />
      <ResizeHandle edge="se" onMouseDown={handleResizeStart('se')} />
      <ResizeHandle edge="sw" onMouseDown={handleResizeStart('sw')} />
    </dialog>
  )
}

interface ResizeHandleProps {
  edge: string
  onMouseDown: (e: React.MouseEvent) => void
}

const RESIZE_HANDLE_CLASSES: Record<string, string> = {
  n: 'top-0 left-2 right-2 h-1 cursor-ns-resize',
  s: 'bottom-0 left-2 right-2 h-1 cursor-ns-resize',
  e: 'right-0 top-2 bottom-2 w-1 cursor-ew-resize',
  w: 'left-0 top-2 bottom-2 w-1 cursor-ew-resize',
  ne: 'top-0 right-0 size-2 cursor-nesw-resize',
  nw: 'top-0 left-0 size-2 cursor-nwse-resize',
  se: 'bottom-0 right-0 size-2 cursor-nwse-resize',
  sw: 'bottom-0 left-0 size-2 cursor-nesw-resize',
}

function ResizeHandle({ edge, onMouseDown }: ResizeHandleProps) {
  return (
    <div
      role="separator"
      aria-label={`Resize ${edge}`}
      tabIndex={-1}
      aria-orientation={
        edge === 'n' || edge === 's' ? 'horizontal' : 'vertical'
      }
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'hover:bg-brand/20 absolute z-10',
          RESIZE_HANDLE_CLASSES[edge],
        )
      }
      onMouseDown={onMouseDown}
    />
  )
}
