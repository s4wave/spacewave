import { useEffect, useState } from 'react'

import { useKeyDispatcherState } from './KeyDispatcher.js'

// WhichKeyPanel renders the active key sequence continuations owned by KeyDispatcher.
export function WhichKeyPanel() {
  const state = useKeyDispatcherState()
  const [visible, setVisible] = useState(state.mode === 'prefix')

  useEffect(() => {
    if (state.mode !== 'prefix') {
      setVisible(false)
      return
    }
    if (state.whichKeyDelayMs <= 0) {
      setVisible(true)
      return
    }
    setVisible(false)
    const timeout = window.setTimeout(() => {
      setVisible(true)
    }, state.whichKeyDelayMs)
    return () => {
      window.clearTimeout(timeout)
    }
  }, [state.mode, state.activePath, state.whichKeyDelayMs])

  if (state.mode !== 'prefix' || !visible) return null
  const path = state.activePath.map(formatResolvedKey).join(' ')

  return (
    <section
      aria-label="Key sequence continuations"
      className="border-foreground/10 bg-background-card/95 pointer-events-auto fixed right-4 bottom-16 z-50 w-[min(28rem,calc(100vw-2rem))] rounded-lg border p-3 shadow-lg backdrop-blur"
    >
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="text-foreground-alt text-xs font-medium tracking-wider uppercase">
          {path}
        </div>
        <div className="text-foreground-alt text-xs">Esc cancels</div>
      </div>
      <div className="space-y-1">
        {state.continuations.map((continuation) => (
          <div
            key={`${continuation.key}:${continuation.commandId ?? 'branch'}`}
            className="flex items-center gap-3 rounded px-2 py-1"
          >
            <kbd className="bg-foreground/5 text-foreground min-w-10 rounded px-2 py-0.5 text-center font-mono text-xs">
              {formatResolvedKey(continuation.key)}
            </kbd>
            <div className="min-w-0 flex-1">
              <div className="text-foreground truncate text-sm">
                {continuation.label ??
                  continuation.commandId ??
                  'More commands'}
              </div>
              {continuation.commandId && (
                <div className="text-foreground-alt truncate font-mono text-xs">
                  {continuation.commandId}
                </div>
              )}
            </div>
            {continuation.conflict && (
              <span className="text-warning text-xs">Conflict</span>
            )}
          </div>
        ))}
      </div>
      {state.conflicts.length > 0 && (
        <div className="border-warning/30 text-warning mt-2 rounded border px-2 py-1 text-xs">
          Conflict: {state.conflicts.map((conflict) => conflict.key).join(', ')}
        </div>
      )}
    </section>
  )
}

function formatResolvedKey(key: string): string {
  return key
    .split('+')
    .map((part) => {
      switch (part) {
        case 'ctrl':
          return 'Ctrl'
        case 'meta':
          return 'Cmd'
        case 'shift':
          return 'Shift'
        case 'alt':
          return 'Alt'
        case 'space':
          return 'Space'
        default:
          return part.length === 1 ? part.toUpperCase() : part
      }
    })
    .join('+')
}
