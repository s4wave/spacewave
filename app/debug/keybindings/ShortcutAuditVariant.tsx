import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import {
  LuChartNoAxesColumnIncreasing,
  LuCheck,
  LuSearch,
  LuSparkles,
  LuTriangleAlert,
  LuWandSparkles,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

import { AuditQueueRow } from './AuditQueueRow.js'
import { BindingCapture } from './BindingCapture.js'
import { Keycap } from './Keycap.js'
import { commandMatchesQuery, contextsOverlap } from './keybindings-model.js'
import { chordsMatch } from './keyboard-utils.js'
import type { KeybindingVariantProps } from './variant-types.js'

type AuditMode = 'issues' | 'customized' | 'all'

const SUGGESTED_CHORDS = [
  'CmdOrCtrl+Shift+1',
  'CmdOrCtrl+Shift+2',
  'CmdOrCtrl+Shift+3',
  'Ctrl+Alt+K',
  'Ctrl+Alt+L',
  'Alt+Shift+K',
  'Alt+Shift+L',
  'CmdOrCtrl+Alt+K',
  'CmdOrCtrl+Alt+L',
] as const

export function ShortcutAuditVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [mode, setMode] = useState<AuditMode>('issues')
  const [query, setQuery] = useState('')
  const [selectedCommandId, setSelectedCommandId] = useState(
    () =>
      commands.find((command) => conflictCommandIds.has(command.id))?.id ??
      commands.find((command) => !command.binding)?.id ??
      commands[0]?.id ??
      '',
  )

  const unassignedCount = useMemo(
    () => commands.filter((command) => !command.binding).length,
    [commands],
  )
  const queueCommands = useMemo(
    () =>
      commands.filter((command) => {
        if (!commandMatchesQuery(command, query)) return false
        if (mode === 'issues') {
          return conflictCommandIds.has(command.id) || !command.binding
        }
        if (mode === 'customized') return customizedCommandIds.has(command.id)
        return true
      }),
    [commands, conflictCommandIds, customizedCommandIds, mode, query],
  )
  const selectedCommand = useMemo(
    () =>
      queueCommands.find((command) => command.id === selectedCommandId) ??
      queueCommands[0],
    [queueCommands, selectedCommandId],
  )
  const conflictPartners = useMemo(() => {
    if (!selectedCommand?.binding) return []
    return commands.filter(
      (command) =>
        command.id !== selectedCommand.id &&
        chordsMatch(command.binding, selectedCommand.binding) &&
        contextsOverlap(command.context, selectedCommand.context),
    )
  }, [commands, selectedCommand])
  const suggestions = useMemo(() => {
    if (!selectedCommand) return []
    return SUGGESTED_CHORDS.filter((chord) =>
      commands.every(
        (command) =>
          !chordsMatch(command.binding, chord) ||
          !contextsOverlap(command.context, selectedCommand.context),
      ),
    ).slice(0, 3)
  }, [commands, selectedCommand])

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuery(event.target.value),
    [],
  )
  const showIssues = useCallback(() => setMode('issues'), [])
  const showCustomized = useCallback(() => setMode('customized'), [])
  const showAll = useCallback(() => setMode('all'), [])
  const changeBinding = useCallback(
    (binding: string) => {
      if (selectedCommand) setBinding(selectedCommand.id, binding)
    },
    [selectedCommand, setBinding],
  )
  const reset = useCallback(() => {
    if (selectedCommand) resetBinding(selectedCommand.id)
  }, [resetBinding, selectedCommand])
  const applySuggestion = useCallback(
    (binding: string) => {
      if (selectedCommand) setBinding(selectedCommand.id, binding)
    },
    [selectedCommand, setBinding],
  )

  return (
    <section className="space-y-4">
      <div>
        <div className="flex items-center gap-2">
          <LuChartNoAxesColumnIncreasing className="text-brand size-4" />
          <h2 className="text-sm font-semibold">Binding health workbench</h2>
        </div>
        <p className="text-foreground-alt/55 mt-1 max-w-2xl text-xs">
          Triage collisions and missing shortcuts, compare defaults, and apply a
          clear alternative without leaving the review queue.
        </p>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <button
          type="button"
          aria-pressed={mode === 'issues'}
          className={cn(
            'border-foreground/8 bg-background-card/30 rounded-xl border p-4 text-left transition-colors',
            mode === 'issues' && 'border-destructive/35 bg-destructive/5',
          )}
          onClick={showIssues}
        >
          <LuTriangleAlert className="text-destructive size-4" />
          <strong className="mt-3 block text-2xl font-semibold">
            {conflictCommandIds.size + unassignedCount}
          </strong>
          <span className="text-foreground-alt/50 text-xs">
            Needs attention
          </span>
        </button>
        <button
          type="button"
          aria-pressed={mode === 'customized'}
          className={cn(
            'border-foreground/8 bg-background-card/30 rounded-xl border p-4 text-left transition-colors',
            mode === 'customized' && 'border-brand/35 bg-brand/5',
          )}
          onClick={showCustomized}
        >
          <LuSparkles className="text-brand size-4" />
          <strong className="mt-3 block text-2xl font-semibold">
            {customizedCommandIds.size}
          </strong>
          <span className="text-foreground-alt/50 text-xs">Customized</span>
        </button>
        <button
          type="button"
          aria-pressed={mode === 'all'}
          className={cn(
            'border-foreground/8 bg-background-card/30 rounded-xl border p-4 text-left transition-colors',
            mode === 'all' && 'border-brand/35 bg-brand/5',
          )}
          onClick={showAll}
        >
          <LuCheck className="text-brand size-4" />
          <strong className="mt-3 block text-2xl font-semibold">
            {commands.length - conflictCommandIds.size - unassignedCount}
          </strong>
          <span className="text-foreground-alt/50 text-xs">Ready to use</span>
        </button>
      </div>

      <div className="border-foreground/8 bg-background-card/30 grid overflow-hidden rounded-xl border lg:grid-cols-[minmax(19rem,0.9fr)_minmax(0,1.1fr)]">
        <div className="border-foreground/8 border-b lg:border-r lg:border-b-0">
          <div className="border-foreground/8 border-b p-3">
            <label className="relative block">
              <LuSearch className="text-foreground-alt/40 pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
              <Input
                value={query}
                onChange={handleQueryChange}
                placeholder="Search review queue"
                aria-label="Search binding review queue"
                className="border-foreground/10 bg-background/30 focus-visible:border-brand/50 focus-visible:ring-brand/15 pl-9"
              />
            </label>
          </div>
          <div className="max-h-135 space-y-1 overflow-y-auto p-2">
            {queueCommands.map((command) => (
              <AuditQueueRow
                key={command.id}
                command={command}
                selected={command.id === selectedCommand?.id}
                hasConflict={conflictCommandIds.has(command.id)}
                isCustomized={customizedCommandIds.has(command.id)}
                onSelect={setSelectedCommandId}
              />
            ))}
            {queueCommands.length === 0 ? (
              <div className="text-foreground-alt/50 px-4 py-16 text-center text-sm">
                No commands in this view match “{query}”.
              </div>
            ) : null}
          </div>
        </div>

        <aside className="bg-background/20 p-5 sm:p-7">
          {selectedCommand ? (
            <div>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <span className="text-brand text-[10px] font-semibold tracking-wider uppercase">
                    Review assignment
                  </span>
                  <h3 className="mt-2 text-xl font-semibold">
                    {selectedCommand.label}
                  </h3>
                  <p className="text-foreground-alt/55 mt-2 max-w-xl text-sm">
                    {selectedCommand.description}
                  </p>
                </div>
                <Badge variant="outline" className="border-foreground/10">
                  {selectedCommand.context}
                </Badge>
              </div>

              <div className="mt-6 grid gap-3 sm:grid-cols-2">
                <div className="border-foreground/8 rounded-lg border p-4">
                  <span className="text-foreground-alt/45 text-[10px] font-semibold tracking-wider uppercase">
                    Current
                  </span>
                  <BindingCapture
                    binding={selectedCommand.binding}
                    commandLabel={selectedCommand.label}
                    isCustomized={customizedCommandIds.has(selectedCommand.id)}
                    className="mt-3"
                    onChange={changeBinding}
                    onReset={reset}
                  />
                </div>
                <div className="border-foreground/8 rounded-lg border p-4">
                  <span className="text-foreground-alt/45 text-[10px] font-semibold tracking-wider uppercase">
                    Factory default
                  </span>
                  <Keycap
                    chord={selectedCommand.defaultBinding}
                    className="mt-3"
                    muted
                  />
                </div>
              </div>

              {conflictPartners.length > 0 ? (
                <div className="border-destructive/25 bg-destructive/5 mt-5 rounded-lg border p-4">
                  <div className="flex items-center gap-2 text-sm font-semibold">
                    <LuTriangleAlert className="text-destructive" />
                    Also assigned in an overlapping context
                  </div>
                  <div className="mt-3 space-y-2">
                    {conflictPartners.map((command) => (
                      <button
                        key={command.id}
                        type="button"
                        className="hover:bg-destructive/8 flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-xs"
                        onClick={() => setSelectedCommandId(command.id)}
                      >
                        <span>
                          {command.label}
                          <span className="text-foreground-alt/45 ml-2">
                            {command.context}
                          </span>
                        </span>
                        <Keycap chord={command.binding} muted />
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                <div className="border-brand/20 bg-brand/5 mt-5 flex items-center gap-3 rounded-lg border p-4 text-sm">
                  <LuCheck className="text-brand size-4" />
                  This assignment is clear in its active context.
                </div>
              )}

              <div className="border-foreground/8 mt-5 rounded-lg border p-4">
                <div className="flex items-center gap-2 text-sm font-semibold">
                  <LuWandSparkles className="text-brand" />
                  Available alternatives
                </div>
                <p className="text-foreground-alt/45 mt-1 text-xs">
                  These combinations are unused in {selectedCommand.context}.
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  {suggestions.map((binding) => (
                    <Button
                      key={binding}
                      type="button"
                      variant="outline"
                      size="sm"
                      className="border-foreground/10 bg-background/30 hover:border-brand/40 hover:bg-brand/10"
                      onClick={() => applySuggestion(binding)}
                    >
                      <Keycap chord={binding} />
                    </Button>
                  ))}
                </div>
              </div>
            </div>
          ) : null}
        </aside>
      </div>
    </section>
  )
}
