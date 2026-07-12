import {
  useCallback,
  useId,
  useMemo,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from 'react'
import { LuCornerDownLeft, LuSearch, LuTriangleAlert } from 'react-icons/lu'
import { Badge } from '@s4wave/web/ui/badge.js'
import { Input } from '@s4wave/web/ui/input.js'

import { BindingCapture } from './BindingCapture.js'
import { FinderResult } from './FinderResult.js'
import { Keycap } from './Keycap.js'
import { commandMatchesQuery } from './keybindings-model.js'
import type { KeybindingVariantProps } from './variant-types.js'

export function CommandFinderVariant({
  commands,
  conflictCommandIds,
  customizedCommandIds,
  setBinding,
  resetBinding,
}: KeybindingVariantProps) {
  const [query, setQuery] = useState('')
  const [selectedCommandId, setSelectedCommandId] = useState(
    commands[0]?.id ?? '',
  )
  const listboxId = `${useId()}-results`

  const results = useMemo(
    () => commands.filter((command) => commandMatchesQuery(command, query)),
    [commands, query],
  )
  const selectedCommand = useMemo(
    () =>
      results.find((command) => command.id === selectedCommandId) ?? results[0],
    [results, selectedCommandId],
  )

  const handleQueryChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value
      setQuery(value)
    },
    [],
  )

  const handleSearchKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>) => {
      if (results.length === 0) return
      const currentIndex = results.findIndex(
        (command) => command.id === selectedCommand?.id,
      )

      if (event.key === 'ArrowDown') {
        event.preventDefault()
        setSelectedCommandId(results[(currentIndex + 1) % results.length].id)
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        const nextIndex =
          currentIndex <= 0 ? results.length - 1 : currentIndex - 1
        setSelectedCommandId(results[nextIndex].id)
      } else if (event.key === 'Enter') {
        event.preventDefault()
        setSelectedCommandId(results[Math.max(currentIndex, 0)].id)
      }
    },
    [results, selectedCommand?.id],
  )

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
    <section className="mx-auto max-w-5xl">
      <div className="border-foreground/8 bg-background-card/40 overflow-hidden rounded-xl border shadow-xl">
        <div className="border-foreground/8 flex items-center gap-3 border-b px-4 py-3">
          <LuSearch className="text-brand size-5 shrink-0" />
          <Input
            autoFocus
            value={query}
            onChange={handleQueryChange}
            onKeyDown={handleSearchKeyDown}
            placeholder="Type a command, context, or shortcut…"
            aria-label="Find a command to rebind"
            role="combobox"
            aria-autocomplete="list"
            aria-controls={listboxId}
            aria-expanded
            aria-activedescendant={
              selectedCommand
                ? `${listboxId}-option-${selectedCommand.id}`
                : undefined
            }
            className="h-auto border-0 bg-transparent px-0 py-1 text-base shadow-none focus-visible:ring-0"
          />
          <span className="border-foreground/10 text-foreground-alt/45 hidden rounded border px-2 py-1 text-[10px] sm:inline-flex">
            ↑↓ navigate
          </span>
        </div>

        <div className="grid min-h-120 lg:grid-cols-[minmax(0,1.1fr)_minmax(20rem,0.9fr)]">
          <div className="border-foreground/8 max-h-145 overflow-y-auto border-b p-2 lg:border-r lg:border-b-0">
            <div className="text-foreground-alt/40 flex items-center justify-between p-2 text-[10px] font-semibold tracking-wider uppercase">
              <span>Matches</span>
              <span>{results.length}</span>
            </div>
            <div
              id={listboxId}
              role="listbox"
              aria-label="Matching commands"
              className="space-y-1"
            >
              {results.map((command) => (
                <FinderResult
                  key={command.id}
                  id={`${listboxId}-option-${command.id}`}
                  command={command}
                  selected={command.id === selectedCommand?.id}
                  hasConflict={conflictCommandIds.has(command.id)}
                  onSelect={setSelectedCommandId}
                />
              ))}
            </div>
            {results.length === 0 ? (
              <div className="text-foreground-alt/50 px-4 py-16 text-center text-sm">
                No commands match “{query}”.
              </div>
            ) : null}
          </div>

          <aside className="bg-background/20 p-5 sm:p-7">
            {selectedCommand ? (
              <div className="sticky top-5">
                <span className="text-brand text-[10px] font-semibold tracking-wider uppercase">
                  Focused binding editor
                </span>
                <h2 className="mt-2 text-xl font-semibold">
                  {selectedCommand.label}
                </h2>
                <p className="text-foreground-alt/55 mt-2 text-sm leading-relaxed">
                  {selectedCommand.description}
                </p>

                <div className="mt-5 flex flex-wrap gap-2">
                  <Badge variant="outline" className="border-foreground/10">
                    {selectedCommand.category}
                  </Badge>
                  <Badge variant="secondary" className="bg-foreground/5">
                    {selectedCommand.context}
                  </Badge>
                  {conflictCommandIds.has(selectedCommand.id) ? (
                    <Badge
                      variant="destructive"
                      className="bg-destructive/15 text-destructive"
                    >
                      <LuTriangleAlert /> Conflicting assignment
                    </Badge>
                  ) : null}
                </div>

                <div className="border-foreground/8 bg-background-card-alt/50 mt-8 rounded-lg border p-4">
                  <label className="text-foreground-alt/50 text-[10px] font-semibold tracking-wider uppercase">
                    Active shortcut
                  </label>
                  <BindingCapture
                    binding={selectedCommand.binding}
                    commandLabel={selectedCommand.label}
                    isCustomized={customizedCommandIds.has(selectedCommand.id)}
                    className="mt-3"
                    onChange={changeBinding}
                    onReset={reset}
                  />
                  <div className="text-foreground-alt/45 mt-4 flex items-center gap-2 text-xs">
                    <LuCornerDownLeft className="size-3.5" />
                    Click the binding, then press a new key combination.
                  </div>
                </div>

                <div className="mt-5 grid grid-cols-2 gap-3 text-xs">
                  <div className="border-foreground/6 rounded-lg border p-3">
                    <span className="text-foreground-alt/45 block">
                      Default
                    </span>
                    <Keycap
                      chord={selectedCommand.defaultBinding}
                      className="mt-2"
                      muted
                    />
                  </div>
                  <div className="border-foreground/6 rounded-lg border p-3">
                    <span className="text-foreground-alt/45 block">
                      Command ID
                    </span>
                    <code className="text-foreground-alt/75 mt-2 block truncate font-mono text-[10px]">
                      {selectedCommand.id}
                    </code>
                  </div>
                </div>
              </div>
            ) : null}
          </aside>
        </div>
      </div>
    </section>
  )
}
