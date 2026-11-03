import { cn } from '@s4wave/web/style/utils.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import * as stylex from '@stylexjs/stylex'
import React, { CSSProperties, useEffect, useRef } from 'react'

import { IBarProps, Bar } from './bar.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    flexWrap: 'nowrap',
    gap: 0,
    overflow: 'hidden',
  },
  rule2: {
    position: 'relative',
    margin: 0,
    display: 'flex',
    maxHeight: 'none',
    width: '100%',
    maxWidth: 'none',
    flex: 1,
    overflow: 'hidden',
    borderWidth: '.21rem',
    borderColor: themeColors['--color-frame-overlay'],
    '--tw-border-style': 'solid',
    borderStyle: 'solid',
    backgroundColor: themeColors['--color-frame-overlay'],
    overflowWrap: 'break-word',
    color: 'inherit',
  },
  rule3: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    overflow: 'hidden',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: 0,
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: 0,
  },
  rule4: { display: 'none' },
})

// IFrameProps are properties for Frame.
export interface IFrameProps {
  // dark marks descendants that still use the shared dark theme variant.
  dark?: boolean
  // children are children elements for the body
  children?: React.ReactNode
  // style are styles to apply to the outer flex
  style?: CSSProperties
  // sx adds styles to the outer flex.
  sx?: stylex.StyleXStyles
  className?: never
  // overlay is content to show in an overlay dialog
  overlay?: React.ReactNode
  // innerSx adds styles to the children container.
  innerSx?: stylex.StyleXStyles
  // overlaySx adds styles to the overlay container.
  overlaySx?: stylex.StyleXStyles
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
export function Frame({ sx, innerSx, overlaySx, ...props }: IFrameProps) {
  // Select the visible body and bars from this frame's overlay state.
  const groupRef = useRef<HTMLDivElement>(null)
  const overlayRef = useRef<HTMLDialogElement>(null)
  const hasOverlay = !!props.overlay
  const onCloseOverlay = props.onCloseOverlay
  const bottomBar = !props.bottomBar?.hidden ? (
    <Bar hideTopBorder={hasOverlay} {...props.bottomBar} />
  ) : undefined

  // A top bar without either slot has no visible content.
  const topBar =
    !props.topBar?.hidden &&
    (props.topBar?.left !== undefined || props.topBar?.right !== undefined) ? (
      <Bar {...props.topBar} />
    ) : undefined

  // Move focus into the overlay when its content becomes visible.
  useEffect(() => {
    if (hasOverlay) overlayRef.current?.focus()
  }, [hasOverlay])

  // Escape closes the overlay unless a layer inside it already handled the
  // key. The listener is on the DOM because overlay content may be a portal
  // whose React parents sit outside this frame.
  useEffect(() => {
    // Listen only while this frame owns a closable overlay.
    const group = groupRef.current
    if (!group || !hasOverlay || !onCloseOverlay) return

    // Handle Escape at the physical frame boundary, including portal events.
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented) onCloseOverlay()
    }
    group.addEventListener('keydown', handleKeyDown)
    return () => group.removeEventListener('keydown', handleKeyDown)
  }, [hasOverlay, onCloseOverlay])

  // Preserve native caller styles beside compiled variables and the theme marker.
  const attributes = stylexProps([stylexStyles.rule1, sx], {
    style: props.style,
  })
  attributes.className = cn(attributes.className, props.dark && 'dark')
  return (
    <div ref={groupRef} role="group" {...attributes}>
      {topBar}

      {hasOverlay ? (
        <dialog
          ref={overlayRef}
          open
          aria-label={props.overlayLabel ?? 'Overlay'}
          sx={[[stylexStyles.rule2], overlaySx]}
          tabIndex={-1}
        >
          {props.overlay}
        </dialog>
      ) : null}

      <div
        sx={[[stylexStyles.rule3, !!hasOverlay && stylexStyles.rule4], innerSx]}
        aria-hidden={hasOverlay}
        hidden={hasOverlay}
      >
        {props.children}
      </div>
      {bottomBar}
    </div>
  )
}
