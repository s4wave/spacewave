import {
  type AnchorHTMLAttributes,
  type ButtonHTMLAttributes,
  type FocusEvent,
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// ViewerMedia is what the stage's content is doing: "still" for an image, a
// PDF page or a paused video, "moving" for a video that plays, and "failed"
// once the media could not load.
export type ViewerMedia = 'still' | 'moving' | 'failed'

// FloatState is the part of ViewerFloat.tla the stage holds; media comes from
// the viewer.
interface FloatState {
  // active is true while the pointer moved or a key went down within the
  // idle window.
  active: boolean
  // focusInside is true while focus is in one of the stage's floats.
  focusInside: boolean
}

type FloatEvent =
  | { type: 'touch' }
  | { type: 'idle' }
  | { type: 'focus-in' }
  | { type: 'focus-out' }

// initialFloatState is the stage at mount: active, since content that moves
// from the start counts as a touch.
const initialFloatState: FloatState = {
  active: true,
  focusInside: false,
}

// reduceFloat moves the stage between the states of ViewerFloat.tla.
function reduceFloat(state: FloatState, event: FloatEvent): FloatState {
  switch (event.type) {
    case 'touch':
      return state.active ? state : { ...state, active: true }
    case 'idle':
      return state.active ? { ...state, active: false } : state
    case 'focus-in':
      return state.focusInside ? state : { ...state, focusInside: true }
    case 'focus-out':
      return state.focusInside ? { ...state, focusInside: false } : state
  }
}

// floatsVisible is Visible in ViewerFloat.tla: the floats leave only over
// moving content that nobody touched lately, with focus elsewhere.
export function floatsVisible(media: ViewerMedia, state: FloatState): boolean {
  return media !== 'moving' || state.active || state.focusInside
}

interface FloatContextValue {
  visible: boolean
  onFocus: () => void
  onBlur: (e: FocusEvent<HTMLElement>) => void
}

const FloatContext = createContext<FloatContextValue | null>(null)

const floatMark = 'data-viewer-float'

// defaultIdleMs is how long the pointer stays still before floats over moving
// content leave.
const defaultIdleMs = 2500

// ViewerStageProps are the props for ViewerStage.
export interface ViewerStageProps {
  // media is what the content is doing; floats leave only while it moves.
  media: ViewerMedia
  // idleMs is how long a still pointer keeps the floats over moving content.
  idleMs?: number
  className?: string
  children: ReactNode
}

// ViewerStage is the pane-filling area a viewer draws its content in, with
// ViewerFloat toolbars over it. It shows the floats over still or failed
// content, and over moving content hides them once the pointer is still or
// gone, unless focus is in them. Content that starts to move, at mount or
// later, keeps the floats for one idle window. A pointer move
// or a key pressed in the stage brings them back, and a hidden float stays in
// the tab order, so Tab into it does too. Its states are in ViewerFloat.tla.
export function ViewerStage({
  media,
  idleMs = defaultIdleMs,
  className,
  children,
}: ViewerStageProps) {
  // The stage holds the pointer and focus state; the viewer passes media.
  const [state, dispatch] = useReducer(reduceFloat, initialFloatState)
  const stageRef = useRef<HTMLDivElement | null>(null)
  const lastTouch = useRef(0)

  // The stage listens to the pointer and keys natively: it watches activity
  // and offers no action of its own. A touch while already active only moves
  // the idle deadline, so a pointer move renders nothing.
  useEffect(() => {
    // The stage mounts once with the component.
    const stage = stageRef.current
    if (!stage) return

    // A pointer move, a press or a key marks the stage active.
    const touch = () => {
      lastTouch.current = performance.now()
      dispatch({ type: 'touch' })
    }
    const touches = ['pointermove', 'pointerdown', 'keydown'] as const
    touches.forEach((type) => stage.addEventListener(type, touch))

    // The pointer leaving ends the idle window at once.
    const leave = () => dispatch({ type: 'idle' })
    stage.addEventListener('pointerleave', leave)

    // Unmounting drops every listener.
    return () => {
      touches.forEach((type) => stage.removeEventListener(type, touch))
      stage.removeEventListener('pointerleave', leave)
    }
  }, [])

  // Content starting to move counts as a touch, however the play began: from
  // a media key, the system's controls, or with the pointer already gone.
  useEffect(() => {
    // Only the change to moving starts a window.
    if (media !== 'moving') return
    lastTouch.current = performance.now()
    dispatch({ type: 'touch' })
  }, [media])

  // The idle deadline runs only while it could hide something.
  const watching = state.active && media === 'moving'
  useEffect(() => {
    // Nothing to hide while the content is still or nobody touched it.
    if (!watching) return
    let timer: ReturnType<typeof setTimeout>

    // Sleep until the latest touch's deadline, then go idle.
    const check = () => {
      const left = lastTouch.current + idleMs - performance.now()
      if (left > 0) {
        timer = setTimeout(check, left)
        return
      }
      dispatch({ type: 'idle' })
    }
    check()
    return () => clearTimeout(timer)
  }, [watching, idleMs])

  // Focus moving between two floats of this stage stays inside.
  const onBlur = useCallback((e: FocusEvent<HTMLElement>) => {
    const next = e.relatedTarget
    const float =
      next instanceof Element ? next.closest(`[${floatMark}]`) : null
    if (float && stageRef.current?.contains(float)) return
    dispatch({ type: 'focus-out' })
  }, [])

  // Floats read their visibility and report focus through context.
  const visible = floatsVisible(media, state)
  const value = useMemo<FloatContextValue>(
    () => ({
      visible,
      onFocus: () => dispatch({ type: 'focus-in' }),
      onBlur,
    }),
    [visible, onBlur],
  )

  return (
    <FloatContext.Provider value={value}>
      <div
        ref={stageRef}
        data-floats={visible ? 'shown' : 'hidden'}
        className={cn(
          'relative flex min-h-0 flex-1 overflow-hidden',
          !visible && 'cursor-none',
          className,
        )}
      >
        {children}
      </div>
    </FloatContext.Provider>
  )
}

// ViewerFloatAt is where a float sits over the stage.
export type ViewerFloatAt = 'tl' | 'tr' | 'bl' | 'br' | 'bc'

const atClass: Record<ViewerFloatAt, string> = {
  tl: 'top-2 left-2 max-w-[calc(100%-7rem)]',
  tr: 'top-2 right-2',
  bl: 'bottom-2 left-2',
  br: 'bottom-2 right-2',
  bc: 'bottom-2 left-1/2 -translate-x-1/2',
}

// ViewerFloatProps are the props for ViewerFloat.
export interface ViewerFloatProps {
  at: ViewerFloatAt
  // label names the toolbar for assistive technology.
  label: string
  className?: string
  children: ReactNode
}

// ViewerFloat is one toolbar floating over a ViewerStage's content. Its
// nearly opaque card keeps its text readable over any media. It shows at
// once and fades only on the way out. Hidden, it ignores the pointer but
// stays in the tab order, so the keyboard can always bring it back.
export function ViewerFloat({
  at,
  label,
  className,
  children,
}: ViewerFloatProps) {
  const ctx = useContext(FloatContext)
  const visible = ctx?.visible ?? true
  return (
    <div
      role="toolbar"
      aria-label={label}
      {...{ [floatMark]: '' }}
      data-state={visible ? 'shown' : 'hidden'}
      className={cn(
        'border-foreground/10 bg-background/94 text-foreground absolute z-20 flex min-w-0 items-center gap-0.5 rounded-md border p-0.5 shadow-md select-none',
        atClass[at],
        // A tap on a hidden float's spot reaches the content, as video.js's
        // own controls do. Focus inside shows it in the same frame, before
        // the stage's state catches up.
        !visible &&
          'pointer-events-none opacity-0 transition-opacity duration-200 focus-within:pointer-events-auto focus-within:opacity-100 focus-within:transition-none motion-reduce:transition-none',
        className,
      )}
      onFocus={ctx?.onFocus}
      onBlur={ctx?.onBlur}
    >
      {children}
    </div>
  )
}

// ViewerFloatPressProps are the props for ViewerFloatPress.
export interface ViewerFloatPressProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  // label names the press; an icon-only press shows it as its tooltip.
  label: string
  // icon draws the press as a 24px square with label as its name.
  icon?: ReactNode
  // essential keeps the press in a cramped pane, where the rest leave.
  essential?: boolean
}

// viewerFloatPressClass sizes a press in a float: 24px, and a 44px target
// under a coarse pointer. A link styled as a press uses it too.
export const viewerFloatPressClass =
  'focus-ring hover:bg-foreground/8 aria-pressed:bg-foreground/12 h-viewer-press flex shrink-0 items-center justify-center gap-1 rounded px-2 text-xs whitespace-nowrap transition-colors disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-3.5 pointer-coarse:min-h-11'

const iconPressClass = 'w-viewer-press px-0 pointer-coarse:min-w-11'

// ViewerFloatPress is one press in a ViewerFloat.
export function ViewerFloatPress({
  label,
  icon,
  essential,
  className,
  children,
  ...props
}: ViewerFloatPressProps) {
  return (
    <button
      type="button"
      aria-label={icon ? label : undefined}
      title={icon ? label : undefined}
      className={cn(
        viewerFloatPressClass,
        icon && iconPressClass,
        !essential && '@pane-cramped:hidden',
        className,
      )}
      {...props}
    >
      {icon ?? children ?? label}
    </button>
  )
}

// ViewerFloatLinkProps are the props for ViewerFloatLink.
export interface ViewerFloatLinkProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  // label names the link and shows as its tooltip.
  label: string
  // icon draws the link as a 24px square.
  icon: ReactNode
  // essential keeps the link in a cramped pane, where the rest leave.
  essential?: boolean
}

// ViewerFloatLink is a press in a ViewerFloat that follows a link, such as
// one that downloads the file the viewer shows.
export function ViewerFloatLink({
  label,
  icon,
  essential,
  className,
  ...props
}: ViewerFloatLinkProps) {
  return (
    <a
      aria-label={label}
      title={label}
      className={cn(
        viewerFloatPressClass,
        iconPressClass,
        !essential && '@pane-cramped:hidden',
        className,
      )}
      {...props}
    >
      {icon}
    </a>
  )
}
