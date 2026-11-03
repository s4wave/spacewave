import { useEffect, useReducer, useRef } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    pointerEvents: 'none',
    position: 'absolute',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: { opacity: 0 },
})

// CanvasScaleIndicatorProps are the props for CanvasScaleIndicator.
interface CanvasScaleIndicatorProps {
  scale: number
}

// VISIBLE_MS is how long the indicator stays fully visible after a scale change.
const VISIBLE_MS = 800

// FADE_MS is the CSS fade-out duration.
const FADE_MS = 500

interface ScaleIndicatorState {
  visible: boolean
  fading: boolean
}

type ScaleIndicatorAction =
  | { type: 'show' }
  | { type: 'fade' }
  | { type: 'hide' }

function scaleIndicatorReducer(
  state: ScaleIndicatorState,
  action: ScaleIndicatorAction,
): ScaleIndicatorState {
  switch (action.type) {
    case 'show':
      return { visible: true, fading: false }
    case 'fade':
      return { ...state, fading: true }
    case 'hide':
      return { visible: false, fading: false }
  }
}

// CanvasScaleIndicator shows the current zoom level during scale changes.
export function CanvasScaleIndicator({ scale }: CanvasScaleIndicatorProps) {
  const [state, dispatch] = useReducer(scaleIndicatorReducer, {
    visible: false,
    fading: false,
  })
  const fadeTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const initialRef = useRef(true)

  useEffect(() => {
    // Skip the initial mount render.
    if (initialRef.current) {
      initialRef.current = false
      return
    }

    if (fadeTimer.current) clearTimeout(fadeTimer.current)
    if (hideTimer.current) clearTimeout(hideTimer.current)

    dispatch({ type: 'show' })

    fadeTimer.current = setTimeout(() => dispatch({ type: 'fade' }), VISIBLE_MS)
    hideTimer.current = setTimeout(
      () => dispatch({ type: 'hide' }),
      VISIBLE_MS + FADE_MS,
    )

    return () => {
      if (fadeTimer.current) clearTimeout(fadeTimer.current)
      if (hideTimer.current) clearTimeout(hideTimer.current)
    }
  }, [scale])

  if (!state.visible) return null

  return (
    <div
      sx={[stylexStyles.rule1, !!state.fading && stylexStyles.rule2]}
      style={{ '--canvas-transition-duration': `${FADE_MS}ms` }}
    >
      {Math.round(scale * 100)}%
    </div>
  )
}
