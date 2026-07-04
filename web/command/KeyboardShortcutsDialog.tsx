import { useCallback, useMemo, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { useCommands } from './CommandContext.js'
import { formatKeybindingHint } from './CommandPalette.js'
import {
  getCommandDisplayBindings,
  type KeybindingGraph,
} from './KeybindingResolver.js'
import { useKeybindingGraph } from './useKeybindingGraph.js'

interface ShortcutCommand {
  state: CommandState
  displayBindings: string[]
}

// GroupedShortcuts groups commands with keybindings by menu path.
interface GroupedShortcuts {
  group: string
  commands: ShortcutCommand[]
}

// groupByMenuPath groups commands that have keybindings by their
// first menu path segment.
function groupByMenuPath(
  commands: CommandState[],
  bindingGraph: KeybindingGraph,
  query: string,
): GroupedShortcuts[] {
  const q = query.toLowerCase()
  const groups = new Map<string, ShortcutCommand[]>()
  const groupOrder = ['File', 'Edit', 'View', 'Tools', 'Help']
  const seen = new Set<string>()

  for (const cmd of commands) {
    const commandId = cmd.command?.commandId
    if (!cmd.active || !commandId || seen.has(commandId)) {
      continue
    }
    const displayBindings = getCommandDisplayBindings(bindingGraph, commandId)
    if (!displayBindings.length) continue
    if (q) {
      const label = (cmd.command?.label ?? '').toLowerCase()
      const bindings = displayBindings.join(' ').toLowerCase()
      if (!containsText(label, q) && !containsText(bindings, q)) continue
    }
    seen.add(commandId)
    const menuPath = cmd.command?.menuPath
    const group = menuPath ? (menuPath.split('/')[0] ?? 'Other') : 'Other'
    let list = groups.get(group)
    if (!list) {
      list = []
      groups.set(group, list)
    }
    list.push({ state: cmd, displayBindings })
  }

  const result: GroupedShortcuts[] = []
  for (const name of groupOrder) {
    const list = groups.get(name)
    if (list) {
      result.push({ group: name, commands: list })
      groups.delete(name)
    }
  }
  for (const [name, list] of groups) {
    result.push({ group: name, commands: list })
  }
  return result
}

function containsText(value: string, query: string): boolean {
  return value.includes(query)
}

export interface KeyboardShortcutsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onEditCommand?: (commandId?: string) => void
}

// KeyboardShortcutsDialog renders a dialog listing all commands with keybindings.
export function KeyboardShortcutsDialog({
  open,
  onOpenChange,
  onEditCommand,
}: KeyboardShortcutsDialogProps) {
  const commands = useCommands()
  const [query, setQuery] = useState('')
  const bindingGraph = useKeybindingGraph(commands)
  const handleFilterRef = useCallback(
    (node: HTMLInputElement | null) => {
      if (open) node?.focus()
    },
    [open],
  )

  const grouped = useMemo(
    () => groupByMenuPath(commands, bindingGraph, query),
    [commands, bindingGraph, query],
  )

  const handleOpenChange = useCallback(
    (next: boolean) => {
      onOpenChange(next)
      if (!next) setQuery('')
    },
    [onOpenChange],
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Keyboard Shortcuts</DialogTitle>
        </DialogHeader>
        {onEditCommand && (
          <button
            type="button"
            className="border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5 text-foreground mb-3 rounded border px-3 py-1.5 text-sm transition-colors"
            onClick={() => onEditCommand()}
          >
            Edit Keyboard Shortcuts
          </button>
        )}
        <input
          ref={handleFilterRef}
          className="bg-background border-foreground/8 text-foreground mb-3 w-full rounded border px-3 py-1.5 text-sm outline-none"
          placeholder="Filter shortcuts..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="max-h-[400px] overflow-auto">
          {grouped.length === 0 && (
            <div className="text-foreground-alt py-4 text-center text-sm">
              No shortcuts found.
            </div>
          )}
          {grouped.map((g) => (
            <div key={g.group} className="mb-3">
              <div className="text-foreground-alt mb-1 text-xs font-medium tracking-wider uppercase">
                {g.group}
              </div>
              {g.commands.map(({ state: cmd, displayBindings }) => (
                <div
                  key={cmd.command?.commandId}
                  className="hover:bg-foreground/5 flex items-center justify-between gap-3 rounded px-1 py-1 transition-colors"
                >
                  <span className="text-foreground text-sm">
                    {cmd.command?.label}
                  </span>
                  <kbd className="bg-foreground/5 text-foreground-alt rounded px-2 py-0.5 font-mono text-xs">
                    {formatKeybindingHint(displayBindings)}
                  </kbd>
                  {onEditCommand && (
                    <button
                      type="button"
                      className="text-brand hover:text-brand-highlight text-xs"
                      onClick={() => onEditCommand(cmd.command?.commandId)}
                    >
                      Edit
                    </button>
                  )}
                </div>
              ))}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
