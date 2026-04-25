import { useEffect, useRef, useState } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// CanvasSyncStatusProps are the props for CanvasSyncStatus.
interface CanvasSyncStatusProps {
  pending: number
}

// DONE_VISIBLE_MS is how long "Sync complete" stays fully visible before fading.
const DONE_VISIBLE_MS = 1200

// FADE_DURATION_MS is the CSS fade-out duration.
const FADE_DURATION_MS = 500

// CanvasSyncStatus renders a subtle sync indicator at the bottom-left of the canvas.
export function CanvasSyncStatus({ pending }: CanvasSyncStatusProps) {
  const [showDone, setShowDone] = useState(false)
  const [fading, setFading] = useState(false)
  const prevPendingRef = useRef(pending)

  useEffect(() => {
    const wasSyncing = prevPendingRef.current > 0
    prevPendingRef.current = pending

    if (!wasSyncing || pending > 0) return

    // Transitioned from syncing to idle: show "Sync complete" then fade.
    setShowDone(true)
    setFading(false)

    const fadeTimer = setTimeout(() => setFading(true), DONE_VISIBLE_MS)
    const hideTimer = setTimeout(() => {
      setShowDone(false)
      setFading(false)
    }, DONE_VISIBLE_MS + FADE_DURATION_MS)

    return () => {
      clearTimeout(fadeTimer)
      clearTimeout(hideTimer)
    }
  }, [pending])

  if (pending === 0 && !showDone) return null

  return (
    <div
      className={cn(
        'text-foreground-alt/50 pointer-events-none absolute bottom-10 left-4 flex items-center gap-1.5 font-mono text-xs transition-opacity',
        fading && 'opacity-0',
      )}
      style={{ transitionDuration: `${FADE_DURATION_MS}ms` }}
    >
      {pending > 0 ?
        <>
          <svg className="h-3 w-3 animate-spin" viewBox="0 0 16 16" fill="none">
            <circle
              cx="8"
              cy="8"
              r="6"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeDasharray="28"
              strokeDashoffset="8"
            />
          </svg>
          Applying {pending} {pending === 1 ? 'change' : 'changes'}...
        </>
      : <>
          <svg className="h-3 w-3" viewBox="0 0 16 16" fill="none">
            <path
              d="M3 8.5l3.5 3.5 6.5-8"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          Synced
        </>
      }
    </div>
  )
}
