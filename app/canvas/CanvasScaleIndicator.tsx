import { useEffect, useRef, useState } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// CanvasScaleIndicatorProps are the props for CanvasScaleIndicator.
interface CanvasScaleIndicatorProps {
  scale: number
}

// VISIBLE_MS is how long the indicator stays fully visible after a scale change.
const VISIBLE_MS = 800

// FADE_MS is the CSS fade-out duration.
const FADE_MS = 500

// CanvasScaleIndicator shows the current zoom level during scale changes.
export function CanvasScaleIndicator({ scale }: CanvasScaleIndicatorProps) {
  const [visible, setVisible] = useState(false)
  const [fading, setFading] = useState(false)
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

    setVisible(true)
    setFading(false)

    fadeTimer.current = setTimeout(() => setFading(true), VISIBLE_MS)
    hideTimer.current = setTimeout(() => {
      setVisible(false)
      setFading(false)
    }, VISIBLE_MS + FADE_MS)

    return () => {
      if (fadeTimer.current) clearTimeout(fadeTimer.current)
      if (hideTimer.current) clearTimeout(hideTimer.current)
    }
  }, [scale])

  if (!visible) return null

  return (
    <div
      className={cn(
        'text-foreground-alt/60 pointer-events-none absolute bottom-4 left-4 font-mono text-xs tabular-nums transition-opacity',
        fading && 'opacity-0',
      )}
      style={{ transitionDuration: `${FADE_MS}ms` }}
    >
      {Math.round(scale * 100)}%
    </div>
  )
}
