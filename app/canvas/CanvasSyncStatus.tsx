import { useEffect, useReducer, useRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

interface CanvasSyncStatusProps {
  pending: number
}

const DONE_VISIBLE_MS = 1200

const FADE_DURATION_MS = 500

interface SyncStatusState {
  showDone: boolean
  fading: boolean
}

type SyncStatusAction =
  | { type: 'show-done' }
  | { type: 'fade' }
  | { type: 'hide' }

function syncStatusReducer(
  state: SyncStatusState,
  action: SyncStatusAction,
): SyncStatusState {
  switch (action.type) {
    case 'show-done':
      return { showDone: true, fading: false }
    case 'fade':
      return { ...state, fading: true }
    case 'hide':
      return { showDone: false, fading: false }
  }
}

export function CanvasSyncStatus({ pending }: CanvasSyncStatusProps) {
  const [state, dispatch] = useReducer(syncStatusReducer, {
    showDone: false,
    fading: false,
  })
  const prevPendingRef = useRef(pending)

  useEffect(() => {
    const wasSyncing = prevPendingRef.current > 0
    prevPendingRef.current = pending

    if (!wasSyncing || pending > 0) return

    dispatch({ type: 'show-done' })

    const fadeTimer = setTimeout(
      () => dispatch({ type: 'fade' }),
      DONE_VISIBLE_MS,
    )
    const hideTimer = setTimeout(
      () => dispatch({ type: 'hide' }),
      DONE_VISIBLE_MS + FADE_DURATION_MS,
    )

    return () => {
      clearTimeout(fadeTimer)
      clearTimeout(hideTimer)
    }
  }, [pending])

  if (pending === 0 && !state.showDone) return null

  return (
    <div
      className={cn(
        'text-foreground-alt/50 pointer-events-none absolute bottom-10 left-4 flex items-center gap-1.5 font-mono text-xs transition-opacity',
        state.fading && 'opacity-0',
      )}
      style={{ transitionDuration: `${FADE_DURATION_MS}ms` }}
    >
      {pending > 0 ? (
        <>
          <svg className="size-3 animate-spin" viewBox="0 0 16 16" fill="none">
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
      ) : (
        <>
          <svg className="size-3" viewBox="0 0 16 16" fill="none">
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
      )}
    </div>
  )
}
