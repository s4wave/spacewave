import { useCallback, useMemo, useState, type ChangeEvent } from 'react'
import {
  LuKeyboard,
  LuMousePointer2,
  LuSearch,
  LuTriangleAlert,
} from 'react-icons/lu'

import { Badge } from '@s4wave/web/ui/badge.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

import { BindingCapture } from './BindingCapture.js'
import { KeyboardKey } from './KeyboardKey.js'
import { Keycap } from './Keycap.js'
import { commandUsesKey, keyboardRowsForPlatform } from './keyboard-map-data.js'
import {
  commandMatchesQuery,
  type KeybindingCommand,
} from './keybindings-model.js'
import { currentKeybindingPlatform } from './keyboard-utils.js'
import type { KeybindingVariantProps } from './variant-types.js'

export function KeyboardMapVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [query, setQuery] = useState('')
  const [selectedCommandId, setSelectedCommandId] = useState(
    () =>
      commands.find((command) => command.binding)?.id ?? commands[0]?.id ?? '',
  )
  const [inspectedKey, setInspectedKey] = useState<string | null>('K')
  const [hoveredKey, setHoveredKey] = useState<string | null>(null)
  const platform = currentKeybindingPlatform()
  const keyboardRows = keyboardRowsForPlatform(platform)

  const visibleCommands = useMemo(
    () => commands.filter((command) => commandMatchesQuery(command, query)),
    [commands, query],
  )
  const activeKey = hoveredKey ?? inspectedKey
  const activeKeyCommands = useMemo(
    () =>
      activeKey
        ? visibleCommands.filter((command) =>
            commandUsesKey(command, activeKey, platform),
          )
        : [],
    [activeKey, platform, visibleCommands],
  )
  const selectedCommand = useMemo(
    () =>
      activeKeyCommands.find((command) => command.id === selectedCommandId) ??
      activeKeyCommands[0],
    [activeKeyCommands, selectedCommandId],
  )

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => setQuery(event.target.value),
    [],
  )
  const inspectKey = useCallback(
    (key: string, mappedCommands: readonly KeybindingCommand[]) => {
      setInspectedKey(key)
      setSelectedCommandId(mappedCommands[0]?.id ?? '')
    },
    [],
  )
  const selectCommand = useCallback((commandId: string) => {
    setSelectedCommandId(commandId)
  }, [])
  const changeBinding = useCallback(
    (binding: string) => {
      if (selectedCommand) setBinding(selectedCommand.id, binding)
    },
    [selectedCommand, setBinding],
  )
  const reset = useCallback(() => {
    if (selectedCommand) resetBinding(selectedCommand.id)
  }, [resetBinding, selectedCommand])

  return (
    <section className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <LuKeyboard className="text-brand size-4" />
            <h2 className="text-sm font-semibold">Keyboard atlas</h2>
          </div>
          <p className="text-foreground-alt/55 mt-1 max-w-xl text-xs">
            Hover to inspect assignments. Click a highlighted key, choose its
            command, then record a replacement.
          </p>
        </div>
        <label className="relative block w-full sm:max-w-72">
          <LuSearch className="text-foreground-alt/40 pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={handleQueryChange}
            placeholder="Highlight matching commands"
            aria-label="Filter keyboard assignments"
            variant="searchPanel"
          />
        </label>
      </div>

      <div className="xl:grid-cols-keybindings grid gap-4">
        <div className="border-foreground/8 bg-background-card/30 overflow-x-auto rounded-xl border p-4 sm:p-6">
          <div className="mx-auto max-w-4xl min-w-180 space-y-2">
            {keyboardRows.map((row) => (
              <div key={row.id} className="flex gap-1.5">
                {row.keys.map((definition) => {
                  const mappedCommands = visibleCommands.filter((command) =>
                    commandUsesKey(command, definition.key, platform),
                  )
                  const hasConflict = mappedCommands.some((command) =>
                    conflictCommandIds.has(command.id),
                  )
                  return (
                    <KeyboardKey
                      key={definition.side ?? definition.key}
                      definition={definition}
                      mappedCommands={mappedCommands}
                      selected={activeKey === definition.key}
                      hasConflict={hasConflict}
                      onHover={setHoveredKey}
                      onSelect={inspectKey}
                    />
                  )
                })}
              </div>
            ))}
          </div>
          <div className="text-foreground-alt/45 mt-5 flex flex-wrap items-center gap-4 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="border-brand/30 bg-brand/8 size-3 rounded border" />
              Assigned
            </span>
            <span className="flex items-center gap-1.5">
              <span className="border-destructive/60 bg-destructive/10 size-3 rounded border" />
              Conflict
            </span>
            <span>{visibleCommands.length} commands represented</span>
          </div>
        </div>

        <aside className="border-foreground/8 bg-background-card/30 rounded-xl border p-4">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-foreground-alt/45 micro-ten font-semibold tracking-wider uppercase">
                Inspected key
              </span>
              <div className="mt-1 font-mono text-lg">{activeKey ?? '—'}</div>
            </div>
            <LuMousePointer2 className="text-brand size-5" />
          </div>

          <div className="mt-4 space-y-1">
            {activeKeyCommands.map((command) => (
              <Button
                key={command.id}
                type="button"
                variant={
                  command.id === selectedCommand?.id ? 'brandGhost' : 'ghost'
                }
                size="commandRow"
                className="h-auto w-full justify-between text-left"
                aria-pressed={command.id === selectedCommand?.id}
                onClick={() => selectCommand(command.id)}
              >
                <span className="min-w-0">
                  <span className="block truncate text-xs font-medium">
                    {command.label}
                  </span>
                  <span className="text-foreground-alt/45 micro-ten block font-normal">
                    {command.context}
                  </span>
                </span>
                <Keycap chord={command.binding} muted />
              </Button>
            ))}
            {activeKeyCommands.length === 0 ? (
              <p className="text-foreground-alt/45 py-5 text-center text-xs">
                No matching command uses this key.
              </p>
            ) : null}
          </div>

          {selectedCommand ? (
            <div className="border-foreground/8 mt-4 border-t pt-4">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <h3 className="text-sm font-semibold">
                    {selectedCommand.label}
                  </h3>
                  <p className="text-foreground-alt/50 mt-1 text-xs">
                    {selectedCommand.description}
                  </p>
                </div>
                {conflictCommandIds.has(selectedCommand.id) ? (
                  <Badge variant="conflict">
                    <LuTriangleAlert />
                  </Badge>
                ) : null}
              </div>
              <BindingCapture
                binding={selectedCommand.binding}
                commandLabel={selectedCommand.label}
                isCustomized={customizedCommandIds.has(selectedCommand.id)}
                className="mt-4"
                onChange={changeBinding}
                onReset={reset}
              />
            </div>
          ) : null}
        </aside>
      </div>
    </section>
  )
}
