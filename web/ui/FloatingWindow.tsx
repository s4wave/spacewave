import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import { LuMinus, LuX } from 'react-icons/lu'
import { isDesktop } from '@aptre/bldr'

import { cn } from '@s4wave/web/style/utils.js'

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
  const baseZIndex = 1000
  const windowOrderRef = useRef<string[]>([])
  const subscribersRef = useRef(new Set<() => void>())

  const notifySubscribers = useCallback(() => {
    subscribersRef.current.forEach((cb) => cb())
  }, [])

  const subscribe = useCallback((callback: () => void) => {
    subscribersRef.current.add(callback)
    return () => subscribersRef.current.delete(callback)
  }, [])

  const register = useCallback(
    (id: string) => {
      if (!windowOrderRef.current.includes(id)) {
        windowOrderRef.current = [...windowOrderRef.current, id]
        notifySubscribers()
      }
    },
    [notifySubscribers],
  )

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

  const bringToFront = useCallback(
    (id: string) => {
      const prev = windowOrderRef.current
      if (!prev.includes(id)) return
      if (prev[prev.length - 1] === id) return
      windowOrderRef.current = [...prev.filter((wid) => wid !== id), id]
      notifySubscribers()
    },
    [notifySubscribers],
  )

  const getZIndex = useCallback((id: string) => {
    const index = windowOrderRef.current.indexOf(id)
    return index === -1 ? baseZIndex : baseZIndex + index
  }, [])

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
  return useContext(FloatingWindowManagerContext)
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
  const manager = useFloatingWindowManager()
  const panelRef = useRef<HTMLDivElement>(null)
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

  const applyStyle = useCallback(() => {
    const el = panelRef.current
    if (!el) return
    const { pos, size } = current.current
    el.style.left = `${pos.x}px`
    el.style.top = `${pos.y}px`
    el.style.width = `${size.width}px`
    el.style.height = `${size.height}px`
  }, [])

  const handleMinimize = useCallback(() => {
    onStateChange({ ...state, expanded: false })
  }, [state, onStateChange])

  const handleDoubleClick = useCallback(() => {
    onStateChange({
      ...state,
      position: defaultPosition,
      size: defaultSize,
    })
  }, [state, onStateChange, defaultPosition, defaultSize])

  const handleMouseDown = useCallback(() => {
    manager?.bringToFront(id)
  }, [manager, id])

  const handleDragStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      manager?.bringToFront(id)
      const { pos } = current.current
      dragOffset.current = { x: e.clientX - pos.x, y: e.clientY - pos.y }

      const maxX = window.innerWidth - 100
      const maxY = window.innerHeight - 100

      const onMove = (e: MouseEvent) => {
        current.current.pos = {
          x: Math.max(0, Math.min(maxX, e.clientX - dragOffset.current.x)),
          y: Math.max(minY, Math.min(maxY, e.clientY - dragOffset.current.y)),
        }
        applyStyle()
      }

      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        const { pos, size } = current.current
        onStateChange({ ...state, position: pos, size })
      }

      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
    },
    [state, onStateChange, minY, applyStyle, manager, id],
  )

  const handleResizeStart = useCallback(
    (edge: string) => (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      manager?.bringToFront(id)

      const startX = e.clientX
      const startY = e.clientY
      const startPos = { ...current.current.pos }
      const startSize = { ...current.current.size }

      const maxWidth = window.innerWidth - startPos.x - 20
      const maxHeight = window.innerHeight - startPos.y - 20

      const onMove = (e: MouseEvent) => {
        const dx = e.clientX - startX
        const dy = e.clientY - startY
        const pos = { ...startPos }
        const size = { ...startSize }

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

        current.current = { pos, size }
        applyStyle()
      }

      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        const { pos, size } = current.current
        onStateChange({ ...state, position: pos, size })
      }

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

  const getZIndexSnapshot = useCallback(() => {
    return manager?.getZIndex(id) ?? 1000
  }, [manager, id])

  const zIndex = useSyncExternalStore(
    subscribeToManager,
    getZIndexSnapshot,
    getZIndexSnapshot,
  )

  const panelStyle = useMemo(
    () => ({
      left: state.position.x,
      top: state.position.y,
      width: state.size.width,
      height: state.size.height,
      zIndex,
    }),
    [state.position, state.size, zIndex],
  )

  return (
    <div
      ref={panelRef}
      className={cn(
        'fixed flex flex-col overflow-hidden',
        'rounded-lg shadow-lg',
        'bg-background-menu/95 backdrop-blur-sm',
        'border-popover-border border',
        className,
      )}
      style={panelStyle}
      onMouseDown={handleMouseDown}
      data-testid={testId}
    >
      {/* Header */}
      <div
        className={cn(
          'flex h-6 shrink-0 items-center justify-between',
          'bg-background-deep/80 px-2',
          'border-popover-border border-b',
          'cursor-grab select-none',
        )}
        onMouseDown={handleDragStart}
        onDoubleClick={handleDoubleClick}
      >
        <div className="flex items-center gap-1.5">
          {icon && (
            <span className="text-brand flex h-3 w-3 items-center justify-center [&>svg]:h-3 [&>svg]:w-3">
              {icon}
            </span>
          )}
          <span className="text-text-secondary text-xs font-medium">
            {title}
          </span>
        </div>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={handleMinimize}
            className={cn(
              'flex h-4 w-4 items-center justify-center rounded',
              'text-foreground-alt hover:text-foreground',
              'hover:bg-pulldown-hover',
              'transition-colors duration-100',
            )}
          >
            <LuMinus className="h-2.5 w-2.5" />
          </button>
          <button
            type="button"
            onClick={onClose ?? handleMinimize}
            className={cn(
              'flex h-4 w-4 items-center justify-center rounded',
              'text-foreground-alt hover:text-error',
              'hover:bg-error-bg',
              'transition-colors duration-100',
            )}
          >
            <LuX className="h-2.5 w-2.5" />
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {children}
      </div>

      {/* Resize handles */}
      <ResizeHandle edge="n" onMouseDown={handleResizeStart('n')} />
      <ResizeHandle edge="s" onMouseDown={handleResizeStart('s')} />
      <ResizeHandle edge="e" onMouseDown={handleResizeStart('e')} />
      <ResizeHandle edge="w" onMouseDown={handleResizeStart('w')} />
      <ResizeHandle edge="ne" onMouseDown={handleResizeStart('ne')} />
      <ResizeHandle edge="nw" onMouseDown={handleResizeStart('nw')} />
      <ResizeHandle edge="se" onMouseDown={handleResizeStart('se')} />
      <ResizeHandle edge="sw" onMouseDown={handleResizeStart('sw')} />
    </div>
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
  ne: 'top-0 right-0 w-2 h-2 cursor-nesw-resize',
  nw: 'top-0 left-0 w-2 h-2 cursor-nwse-resize',
  se: 'bottom-0 right-0 w-2 h-2 cursor-nwse-resize',
  sw: 'bottom-0 left-0 w-2 h-2 cursor-nesw-resize',
}

function ResizeHandle({ edge, onMouseDown }: ResizeHandleProps) {
  return (
    <div
      className={cn(
        'hover:bg-brand/20 absolute z-10',
        RESIZE_HANDLE_CLASSES[edge],
      )}
      onMouseDown={onMouseDown}
    />
  )
}
