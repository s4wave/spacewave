import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import {
  CommandDialog,
  CommandEmpty,
  CommandFooter,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from '@s4wave/web/ui/command.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import {
  type SubItem,
  useCommandContext,
  useCommands,
  useInvokeCommand,
} from './CommandContext.js'
import { useCommand } from './useCommand.js'

// isMacPlatform detects whether the current platform is macOS.
const isMacPlatform =
  typeof navigator !== 'undefined' && navigator.platform.includes('Mac')

// formatKeybinding converts a keybinding string to a display string.
// On Mac: CmdOrCtrl -> special char, Shift -> special char, etc.
export function formatKeybinding(binding: string): string {
  const parts = binding.split('+')
  const display: string[] = []

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (i === parts.length - 1) {
      display.push(part.length === 1 ? part.toUpperCase() : part)
      continue
    }
    switch (part) {
      case 'CmdOrCtrl':
        display.push(isMacPlatform ? '\u2318' : 'Ctrl')
        break
      case 'Cmd':
        display.push('\u2318')
        break
      case 'Ctrl':
        display.push(isMacPlatform ? '\u2303' : 'Ctrl')
        break
      case 'Shift':
        display.push(isMacPlatform ? '\u21E7' : 'Shift')
        break
      case 'Alt':
        display.push(isMacPlatform ? '\u2325' : 'Alt')
        break
      default:
        display.push(part)
    }
  }

  return display.join(isMacPlatform ? '' : '+')
}

// GroupedCommands groups commands by the first segment of their menu path.
interface GroupedCommands {
  group: string
  commands: CommandState[]
}

function dedupeSubItems(items: SubItem[]): SubItem[] {
  const deduped = new Map<string, SubItem>()
  for (const item of items) {
    if (!item.id) continue
    if (!deduped.has(item.id)) {
      deduped.set(item.id, item)
    }
  }
  return [...deduped.values()]
}

function findActiveCommand(
  commands: CommandState[],
  commandId: string,
): CommandState | undefined {
  return commands.find(
    (cmd) => cmd.active && cmd.command?.commandId === commandId,
  )
}

// groupCommands groups active commands by the first menu path segment.
function groupCommands(commands: CommandState[]): GroupedCommands[] {
  const groups = new Map<string, CommandState[]>()
  const groupOrder = ['File', 'Edit', 'View', 'Tools', 'Help']
  const seen = new Set<string>()

  for (const cmd of commands) {
    if (!cmd.active) continue
    const commandId = cmd.command?.commandId
    if (!commandId || seen.has(commandId)) continue
    seen.add(commandId)
    const menuPath = cmd.command?.menuPath
    const group = menuPath ? (menuPath.split('/')[0] ?? 'Other') : 'Other'
    let list = groups.get(group)
    if (!list) {
      list = []
      groups.set(group, list)
    }
    list.push(cmd)
  }

  const result: GroupedCommands[] = []
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

// isCommandEnabled checks if a command is enabled.
function isCommandEnabled(cmd: CommandState): boolean {
  return cmd.enabled !== false
}

// CommandPaletteItem renders a single command in the palette.
function CommandPaletteItem({
  cmd,
  onSelect,
}: {
  cmd: CommandState
  onSelect: (commandId: string) => void
}) {
  const commandId = cmd.command?.commandId
  if (!commandId) return null
  const enabled = isCommandEnabled(cmd)

  return (
    <CommandItem
      key={commandId}
      value={`${cmd.command?.label ?? ''} ${commandId}`}
      onSelect={() => enabled && onSelect(commandId)}
      disabled={!enabled}
      className={cn(!enabled && 'cursor-default opacity-50')}
    >
      <span className="flex flex-col">
        <span>{cmd.command?.label}</span>
        {cmd.command?.description && (
          <span className="text-foreground-alt text-xs">
            {cmd.command.description}
          </span>
        )}
      </span>
      {cmd.command?.keybinding && (
        <CommandShortcut>
          {formatKeybinding(cmd.command.keybinding)}
        </CommandShortcut>
      )}
    </CommandItem>
  )
}

// CommandPalette renders a searchable command palette dialog.
// Supports sub-item navigation: selecting a command with has_sub_items
// replaces the command list with a filtered sub-item list.
export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const commands = useCommands()
  const invokeCommand = useInvokeCommand()
  const { getSubItems, registerOpenCommand } = useCommandContext()

  const [subItemCommandId, setSubItemCommandId] = useState<string | null>(null)
  const [subItems, setSubItems] = useState<SubItem[]>([])
  const [subQuery, setSubQuery] = useState('')
  const abortRef = useRef<AbortController | null>(null)

  const commandsRef = useRef(commands)
  useEffect(() => {
    commandsRef.current = commands
  }, [commands])

  useEffect(() => {
    return registerOpenCommand((commandId: string) => {
      const cmd = findActiveCommand(commandsRef.current, commandId)
      if (cmd?.command?.hasSubItems) {
        setSubItemCommandId(commandId)
        setSubItems([])
        setSubQuery('')
      }
      setOpen(true)
    })
  }, [registerOpenCommand])

  useCommand({
    commandId: 'spacewave.view.palette',
    label: 'Command Palette',
    keybinding: 'CmdOrCtrl+K',
    menuPath: 'View/Command Palette',
    menuGroup: 10,
    menuOrder: 1,
    handler: useCallback(() => setOpen(true), []),
  })

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next)
    if (!next) {
      setSubItemCommandId(null)
      setSubItems([])
      setSubQuery('')
      abortRef.current?.abort()
      abortRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!subItemCommandId) return

    abortRef.current?.abort()
    const abort = new AbortController()
    abortRef.current = abort

    getSubItems(subItemCommandId, subQuery, abort.signal)
      .then((items) => {
        if (!abort.signal.aborted) {
          setSubItems(dedupeSubItems(items))
        }
      })
      .catch(() => {})

    return () => {
      abort.abort()
    }
  }, [subItemCommandId, subQuery, getSubItems])

  const grouped = useMemo(() => groupCommands(commands), [commands])

  const handleSelect = useCallback(
    (commandId: string) => {
      const cmd = findActiveCommand(commands, commandId)
      if (cmd?.command?.hasSubItems) {
        setSubItemCommandId(commandId)
        setSubItems([])
        setSubQuery('')
        return
      }
      invokeCommand(commandId)
      handleOpenChange(false)
    },
    [commands, invokeCommand, handleOpenChange],
  )

  const handleSubItemSelect = useCallback(
    (subItemId: string) => {
      if (subItemCommandId) {
        invokeCommand(subItemCommandId, { subItemId })
      }
      handleOpenChange(false)
    },
    [subItemCommandId, invokeCommand, handleOpenChange],
  )

  const handleBack = useCallback(() => {
    setSubItemCommandId(null)
    setSubItems([])
    setSubQuery('')
    abortRef.current?.abort()
    abortRef.current = null
  }, [])

  const activeSubItemCommand =
    subItemCommandId ? findActiveCommand(commands, subItemCommandId) : undefined
  const placeholder =
    activeSubItemCommand ?
      `Search ${activeSubItemCommand.command?.label ?? ''}...`
    : 'Type a command or search...'

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      showCloseButton={false}
    >
      <CommandInput
        placeholder={placeholder}
        value={subItemCommandId ? subQuery : undefined}
        onValueChange={subItemCommandId ? setSubQuery : undefined}
      />
      <CommandList>
        {subItemCommandId ?
          <>
            <CommandEmpty>No items found.</CommandEmpty>
            <CommandGroup heading={activeSubItemCommand?.command?.label ?? ''}>
              <CommandItem
                value="__back__"
                onSelect={handleBack}
                className="text-foreground-alt"
              >
                &larr; Back to commands
              </CommandItem>
              {subItems.map((item) => (
                <CommandItem
                  key={item.id}
                  value={`${item.label} ${item.id}`}
                  onSelect={() => handleSubItemSelect(item.id)}
                >
                  <span className="flex flex-col">
                    <span>{item.label}</span>
                    {item.description && (
                      <span className="text-foreground-alt text-xs">
                        {item.description}
                      </span>
                    )}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        : <>
            <CommandEmpty>No commands found.</CommandEmpty>
            {grouped.map((g) => (
              <CommandGroup key={g.group} heading={g.group}>
                {g.commands.map((cmd) => {
                  const commandId = cmd.command?.commandId
                  if (!commandId) return null
                  return (
                    <CommandPaletteItem
                      key={commandId}
                      cmd={cmd}
                      onSelect={handleSelect}
                    />
                  )
                })}
              </CommandGroup>
            ))}
          </>
        }
      </CommandList>
      <CommandFooter />
    </CommandDialog>
  )
}
