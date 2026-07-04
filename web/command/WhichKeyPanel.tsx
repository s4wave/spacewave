import { useEffect, useState } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

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
    <>
      <div className="bg-background/30 pointer-events-none fixed inset-0 z-40 backdrop-blur-[1px]" />
      <section
        aria-label="Key sequence continuations"
        className="border-foreground/10 bg-background-card/95 pointer-events-auto fixed right-0 bottom-0 left-0 z-50 border-t px-4 py-3 shadow-none"
      >
        <div className="mx-auto flex w-full max-w-6xl items-start gap-4">
          <div className="min-w-32 shrink-0">
            <div className="text-brand font-mono text-xs font-medium tracking-wider uppercase">
              {path}
            </div>
            <div className="text-foreground-alt/50 mt-1 text-[10px]">
              Esc cancels
            </div>
          </div>
          <div className="grid min-w-0 flex-1 grid-cols-[repeat(auto-fit,minmax(12rem,1fr))] gap-x-4 gap-y-1">
            {state.continuations.map((continuation) => (
              <div
                key={`${continuation.key}:${continuation.commandId ?? 'branch'}`}
                className={cn(
                  'border-foreground/6 flex min-w-0 items-center gap-2 border-l px-2 py-1',
                  !continuation.commandId && 'border-brand/30 bg-brand/5',
                )}
              >
                <kbd className="text-brand min-w-8 text-center font-mono text-xs font-semibold">
                  {formatResolvedKey(continuation.key)}
                </kbd>
                <div className="min-w-0 flex-1">
                  <div className="text-foreground truncate text-xs font-medium">
                    {continuation.label ??
                      continuation.commandId ??
                      'More commands'}
                  </div>
                  {continuation.commandId && (
                    <div className="text-foreground-alt/50 truncate font-mono text-[10px]">
                      {continuation.commandId}
                    </div>
                  )}
                </div>
                {continuation.conflict && (
                  <span className="text-warning text-[10px]">Conflict</span>
                )}
              </div>
            ))}
          </div>
        </div>
        {state.conflicts.length > 0 && (
          <div className="border-warning/30 text-warning mx-auto mt-2 max-w-6xl rounded border px-2 py-1 text-xs">
            Conflict:{' '}
            {state.conflicts.map((conflict) => conflict.key).join(', ')}
          </div>
        )}
      </section>
    </>
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
