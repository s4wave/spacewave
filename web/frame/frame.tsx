import React, { CSSProperties, useEffect, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { IBarProps, Bar } from './bar.js'

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
  const groupRef = useRef<HTMLDivElement>(null)
  const overlayRef = useRef<HTMLDialogElement>(null)
  const hasOverlay = !!props.overlay
  const onCloseOverlay = props.onCloseOverlay
  const bottomBar = !props.bottomBar?.hidden ? (
    <Bar hideTopBorder={hasOverlay} {...props.bottomBar} />
  ) : undefined

  const topBar =
    !props.topBar?.hidden &&
    (props.topBar?.left !== undefined || props.topBar?.right !== undefined) ? (
      <Bar {...props.topBar} />
    ) : undefined

  useEffect(() => {
    if (hasOverlay) overlayRef.current?.focus()
  }, [hasOverlay])

  // Escape closes the overlay unless a layer inside it already handled the
  // key. The listener is on the DOM because overlay content may be a portal
  // whose React parents sit outside this frame.
  useEffect(() => {
    const group = groupRef.current
    if (!group || !hasOverlay || !onCloseOverlay) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented) onCloseOverlay()
    }
    group.addEventListener('keydown', handleKeyDown)
    return () => group.removeEventListener('keydown', handleKeyDown)
  }, [hasOverlay, onCloseOverlay])

  return (
    <div
      ref={groupRef}
      role="group"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'relative flex-1 overflow-hidden',
          'flex flex-col flex-nowrap gap-0',
          props.className,
        )
      }
      style={props.style}
    >
      {topBar}

      {hasOverlay ? (
        <dialog
          ref={overlayRef}
          open
          aria-label={props.overlayLabel ?? 'Overlay'}
          className={
            /* TODO(stylex): dynamic class or forwarded className */ cn(
              'border-frame-overlay-border bg-frame-overlay relative m-0 flex max-h-none w-full max-w-none flex-1 overflow-hidden border-frame-overlay border-solid text-inherit break-words',
              props.overlayClassName,
            )
          }
          tabIndex={-1}
        >
          {props.overlay}
        </dialog>
      ) : null}

      <div
        className={
          /* TODO(stylex): dynamic class or forwarded className */ cn(
            'relative flex flex-1 overflow-hidden border-r-0 border-b-0 border-l-0',
            hasOverlay && 'hidden',
            props.innerClassName,
          )
        }
        aria-hidden={hasOverlay}
        hidden={hasOverlay}
      >
        {props.children}
      </div>
      {bottomBar}
    </div>
  )
}
