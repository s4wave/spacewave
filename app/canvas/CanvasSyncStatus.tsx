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
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    pointerEvents: 'none',
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 10)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: { opacity: 0 },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// CanvasSyncStatusProps are the props for CanvasSyncStatus.
interface CanvasSyncStatusProps {
  pending: number
}

// DONE_VISIBLE_MS is how long "Sync complete" stays fully visible before fading.
const DONE_VISIBLE_MS = 1200

// FADE_DURATION_MS is the CSS fade-out duration.
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

// CanvasSyncStatus renders a subtle sync indicator at the bottom-left of the canvas.
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

    // Transitioned from syncing to idle: show "Sync complete" then fade.
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
      sx={[stylexStyles.rule1, !!state.fading && stylexStyles.rule2]}
      style={{ '--canvas-transition-duration': `${FADE_DURATION_MS}ms` }}
    >
      {pending > 0 ? (
        <>
          <svg sx={stylexStyles.rule3} viewBox="0 0 16 16" fill="none">
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
          <svg sx={stylexStyles.rule4} viewBox="0 0 16 16" fill="none">
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
