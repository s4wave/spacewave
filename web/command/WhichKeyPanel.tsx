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
  }, [state.mode, state.whichKeyDelayMs])

  if (state.mode !== 'prefix' || !visible) return null
  const path = state.activePath.map(formatResolvedKey).join(' ')
  const query = state.query ?? ''
  const selectedIndex = state.selectedIndex ?? 0

  return (
    <>
      <div className="bg-background/30 pointer-events-none fixed inset-0 z-40 backdrop-blur-[1px]" />
      <section
        aria-label="Key sequence continuations"
        className="border-foreground/10 bg-background-card/95 pointer-events-auto fixed right-0 bottom-0 left-0 z-50 border-t px-4 py-3 shadow-none"
      >
        <div className="mx-auto w-full max-w-3xl">
          <div className="mb-2 flex items-center gap-3">
            <div className="text-brand shrink-0 font-mono text-xs font-medium tracking-wider uppercase">
              {path}
            </div>
            {query ? (
              <div className="text-foreground min-w-0 flex-1 truncate text-sm">
                Filter: <span className="font-medium">{query}</span>
              </div>
            ) : (
              <div className="text-foreground-alt min-w-0 flex-1 text-xs">
                Type a chord or command name
              </div>
            )}
            <div className="text-foreground-alt/60 shrink-0 text-xs">
              ↑↓ Navigate · Enter Run · Esc Cancel
            </div>
          </div>
          <div
            aria-label="Matching key sequence commands"
            className="max-h-72 overflow-y-auto"
            role="listbox"
          >
            {state.continuations.map((continuation, index) => {
              const remainingKeys =
                continuation.remainingKeys ??
                (continuation.key ? [continuation.key] : [])
              return (
                <div
                  aria-selected={index === selectedIndex}
                  key={`${continuation.key}:${continuation.commandId ?? 'branch'}`}
                  className={cn(
                    'border-foreground/6 flex min-w-0 items-center gap-3 border-l px-3 py-2',
                    index === selectedIndex &&
                      'border-brand bg-brand/10 text-foreground',
                  )}
                  role="option"
                >
                  <div className="flex min-w-24 shrink-0 items-center gap-1">
                    {remainingKeys.length > 0 ? (
                      remainingKeys.map((key, keyIndex) => (
                        <kbd
                          className="bg-foreground/5 text-brand rounded px-1.5 py-0.5 font-mono text-xs font-semibold"
                          key={`${key}:${keyIndex}`}
                        >
                          {formatResolvedKey(key)}
                        </kbd>
                      ))
                    ) : (
                      <kbd className="bg-foreground/5 text-brand rounded px-1.5 py-0.5 font-mono text-xs font-semibold">
                        Enter
                      </kbd>
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-foreground truncate text-sm font-medium">
                      {continuation.label ??
                        continuation.commandId ??
                        'More commands'}
                    </div>
                    {continuation.commandId && (
                      <div className="text-foreground-alt/60 truncate font-mono text-xs">
                        {continuation.commandId}
                      </div>
                    )}
                  </div>
                  {continuation.conflict && (
                    <span className="text-warning text-xs">Conflict</span>
                  )}
                </div>
              )
            })}
            {state.continuations.length === 0 && (
              <div className="text-foreground-alt px-3 py-4 text-sm">
                No matching leader commands
              </div>
            )}
          </div>
        </div>
        {state.conflicts.length > 0 && (
          <div className="border-warning/30 text-warning mx-auto mt-2 max-w-3xl rounded border px-2 py-1 text-xs">
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
