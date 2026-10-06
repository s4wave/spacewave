import React, {
  CSSProperties,
  createContext,
  use,
  useEffect,
  useRef,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { IBarProps, Bar } from './bar.js'

const OverlayCloseContext = createContext<(() => void) | undefined>(undefined)

// useOverlayClose returns the close handler for the current overlay.
export function useOverlayClose() {
  return use(OverlayCloseContext)
}

// IFrameProps are properties for Frame.
export interface IFrameProps {
  // children are children elements for the body
  children?: React.ReactNode
  // style are styles to apply to the outer flex
  style?: CSSProperties
  // className is a class name to apply to the outer flex.
  className?: string
  // overlay is content to show in an overlay dialog
  overlay?: React.ReactNode
  // innerClassName is additional className for the children container
  innerClassName?: string
  // overlayClassName is additional className for the overlay container
  overlayClassName?: string
  // overlayLabel is the accessible name of the overlay dialog.
  overlayLabel?: string
  // bottomBar are props for the bottom bar.
  bottomBar?: IBarProps
  // topBar are props for the top bar.
  //
  // note: unlike the bottom bar, the top bar is automatically hidden if the
  // hidden, left, and right properties are undefined.
  topBar?: IBarProps
  // onCloseOverlay is called when the overlay should be closed
  onCloseOverlay?: () => void
}

// Frame is a body with horizontal bars above and/or below it.
export function Frame(props: IFrameProps) {
  const overlayRef = useRef<HTMLDialogElement>(null)
  const bottomBar = !props.bottomBar?.hidden ? (
    <Bar hideTopBorder={!!props.overlay} {...props.bottomBar} />
  ) : undefined

  const topBar =
    !props.topBar?.hidden &&
    (props.topBar?.left !== undefined || props.topBar?.right !== undefined) ? (
      <Bar {...props.topBar} />
    ) : undefined

  useEffect(() => {
    if (props.overlay) overlayRef.current?.focus()
  }, [props.overlay])

  return (
    <div
      role="group"
      className={cn(
        'relative flex-1 overflow-hidden',
        'flex flex-col flex-nowrap gap-0',
        props.className,
      )}
      style={props.style}
      onKeyDown={(e) => {
        if (e.key === 'Escape' && props.overlay && props.onCloseOverlay) {
          props.onCloseOverlay()
        }
      }}
    >
      {topBar}

      {props.overlay ? (
        <dialog
          ref={overlayRef}
          open
          aria-label={props.overlayLabel ?? 'Overlay'}
          className={cn(
            'border-frame-overlay-border bg-frame-overlay relative m-0 flex max-h-none w-full max-w-none flex-1 overflow-hidden border-frame-overlay border-solid text-inherit break-words',
            props.overlayClassName,
          )}
          tabIndex={-1}
          onKeyDown={(e) => {
            if (e.key === 'Escape' && props.onCloseOverlay) {
              props.onCloseOverlay()
            }
          }}
        >
          {props.overlay}
        </dialog>
      ) : null}

      <div
        className={cn(
          'relative flex flex-1 overflow-hidden border-r-0 border-b-0 border-l-0',
          props.overlay && 'hidden',
          props.innerClassName,
        )}
        aria-hidden={!!props.overlay}
        hidden={!!props.overlay}
      >
        {props.children}
      </div>
      {bottomBar}
    </div>
  )
}
