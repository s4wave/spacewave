import { useEffect, useReducer, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

interface CanvasScaleIndicatorProps {
  scale: number
}

const VISIBLE_MS = 800

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

export function CanvasScaleIndicator({ scale }: CanvasScaleIndicatorProps) {
  const [state, dispatch] = useReducer(scaleIndicatorReducer, {
    visible: false,
    fading: false,
  })
  const fadeTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const initialRef = useRef(true)

  useEffect(() => {
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
      className={cn(
        'text-foreground-alt/60 pointer-events-none absolute bottom-4 left-4 font-mono text-xs tabular-nums transition-opacity',
        state.fading && 'opacity-0',
      )}
      style={{ transitionDuration: `${FADE_MS}ms` }}
    >
      {Math.round(scale * 100)}%
    </div>
  )
}
