import { useCallback, useMemo, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { useCommands } from './CommandContext.js'
import { formatKeybinding } from './CommandPalette.js'

// GroupedShortcuts groups commands with keybindings by menu path.
interface GroupedShortcuts {
  group: string
  commands: CommandState[]
}

// groupByMenuPath groups commands that have keybindings by their
// first menu path segment.
function groupByMenuPath(
  commands: CommandState[],
  query: string,
): GroupedShortcuts[] {
  const q = query.toLowerCase()
  const groups = new Map<string, CommandState[]>()
  const groupOrder = ['File', 'Edit', 'View', 'Tools', 'Help']
  const seen = new Set<string>()

  for (const cmd of commands) {
    const commandId = cmd.command?.commandId
    if (
      !cmd.active ||
      !cmd.command?.keybinding ||
      !commandId ||
      seen.has(commandId)
    ) {
      continue
    }
    if (q) {
      const label = (cmd.command.label ?? '').toLowerCase()
      const binding = cmd.command.keybinding.toLowerCase()
      if (!label.includes(q) && !binding.includes(q)) continue
    }
    seen.add(commandId)
    const menuPath = cmd.command.menuPath
    const group = menuPath ? (menuPath.split('/')[0] ?? 'Other') : 'Other'
    let list = groups.get(group)
    if (!list) {
      list = []
      groups.set(group, list)
    }
    list.push(cmd)
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

export interface KeyboardShortcutsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// KeyboardShortcutsDialog renders a dialog listing all commands with keybindings.
export function KeyboardShortcutsDialog({
  open,
  onOpenChange,
}: KeyboardShortcutsDialogProps) {
  const commands = useCommands()
  const [query, setQuery] = useState('')

  const grouped = useMemo(
    () => groupByMenuPath(commands, query),
    [commands, query],
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
        <input
          className="bg-background border-foreground/8 text-foreground mb-3 w-full rounded border px-3 py-1.5 text-sm outline-none"
          placeholder="Filter shortcuts..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          autoFocus
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
              {g.commands.map((cmd) => (
                <div
                  key={cmd.command?.commandId}
                  className="flex items-center justify-between py-1"
                >
                  <span className="text-foreground text-sm">
                    {cmd.command?.label}
                  </span>
                  <kbd className="bg-foreground/5 text-foreground-alt rounded px-2 py-0.5 font-mono text-xs">
                    {formatKeybinding(cmd.command!.keybinding!)}
                  </kbd>
                </div>
              ))}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
