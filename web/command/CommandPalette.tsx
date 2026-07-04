import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react'

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
import { CommandFocusContext } from '@s4wave/sdk/command/command.pb.js'

import {
  type SubItem,
  useCommandContext,
  useCommands,
  useInvokeCommand,
} from './CommandContext.js'
import { useCommand } from './useCommand.js'
import {
  comboFromKeyboardEvent,
  getCommandDisplayBindings,
  normalizeKeyCombo,
  selectActiveSequenceNode,
  type KeybindingGraph,
  type KeybindingSequenceNode,
} from './KeybindingResolver.js'
import { useKeybindingGraph } from './useKeybindingGraph.js'

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

export function formatKeybindingHint(bindings: string[]): string {
  return bindings.map(formatKeybinding).join(' / ')
}

// GroupedCommands groups commands by the first segment of their menu path.
interface GroupedCommands {
  group: string
  commands: CommandState[]
}

type PaletteMode = 'chord' | 'filter'

interface ChordContinuation {
  key: string
  label: string
  commandId?: string
  conflict: boolean
  node: KeybindingSequenceNode
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
  bindingGraph,
  onSelect,
}: {
  cmd: CommandState
  bindingGraph: KeybindingGraph
  onSelect: (commandId: string) => void
}) {
  const commandId = cmd.command?.commandId
  if (!commandId) return null
  const enabled = isCommandEnabled(cmd)
  const displayBindings = getCommandDisplayBindings(bindingGraph, commandId)

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
      {displayBindings.length > 0 && (
        <CommandShortcut>
          {formatKeybindingHint(displayBindings)}
        </CommandShortcut>
      )}
    </CommandItem>
  )
}

function CommandChordItem({
  continuation,
  onSelect,
}: {
  continuation: ChordContinuation
  onSelect: (continuation: ChordContinuation) => void
}) {
  return (
    <CommandItem
      value={`${continuation.key} ${continuation.label} ${
        continuation.commandId ?? ''
      }`}
      onSelect={() => onSelect(continuation)}
      className={cn(continuation.conflict && 'text-warning')}
    >
      <kbd className="bg-foreground/5 text-foreground min-w-10 rounded px-2 py-0.5 text-center font-mono text-xs">
        {formatResolvedKey(continuation.key)}
      </kbd>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{continuation.label}</span>
        {continuation.commandId && (
          <span className="text-foreground-alt truncate font-mono text-xs">
            {continuation.commandId}
          </span>
        )}
      </span>
      {continuation.conflict && <CommandShortcut>Conflict</CommandShortcut>}
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
  const [query, setQuery] = useState('')
  const [paletteMode, setPaletteMode] = useState<PaletteMode>('chord')
  const [chordPath, setChordPath] = useState<string[]>(['Leader'])
  const [chordNode, setChordNode] = useState<KeybindingSequenceNode | null>(
    null,
  )
  const abortRef = useRef<AbortController | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const commandsRef = useRef(commands)
  const grouped = useMemo(() => groupCommands(commands), [commands])
  const bindingGraph = useKeybindingGraph(commands)
  const leaderStep = useMemo(
    () => normalizeKeyCombo(bindingGraph.leaderCombo),
    [bindingGraph.leaderCombo],
  )
  const rootChordNode = useMemo(() => {
    const node = bindingGraph.sequenceTrie.children.get(leaderStep)
    return node
      ? (selectActiveSequenceNode(node, [CommandFocusContext.GLOBAL]) ?? null)
      : null
  }, [bindingGraph.sequenceTrie, leaderStep])
  const chordContinuations = useMemo(
    () => (chordNode ? continuationsFromNode(chordNode) : []),
    [chordNode],
  )

  const resetChord = useCallback(() => {
    setPaletteMode('chord')
    setQuery('')
    setChordPath(['Leader'])
    setChordNode(rootChordNode)
  }, [rootChordNode])

  const focusFilter = useCallback(() => {
    window.queueMicrotask(() => inputRef.current?.focus())
  }, [])

  const enterFilterMode = useCallback(
    (nextQuery = '') => {
      setPaletteMode('filter')
      setQuery(nextQuery)
      focusFilter()
    },
    [focusFilter],
  )

  const openMergedPalette = useCallback(() => {
    setSubItemCommandId(null)
    setSubItems([])
    setSubQuery('')
    resetChord()
    setOpen(true)
  }, [resetChord])

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
        setOpen(true)
        return
      }
      openMergedPalette()
    })
  }, [openMergedPalette, registerOpenCommand])

  useCommand({
    commandId: 'spacewave.view.palette',
    label: 'Command Palette',
    defaultBindings: [
      {
        id: 'global-palette',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
        when: CommandFocusContext.GLOBAL,
      },
      {
        id: 'global-palette-sequence',
        binding: { case: 'sequence', value: { steps: ['Leader', 'Space'] } },
        when: CommandFocusContext.GLOBAL,
      },
    ],
    menuPath: 'View/Command Palette',
    menuGroup: 10,
    menuOrder: 1,
    handler: openMergedPalette,
  })

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next)
    if (!next) {
      setSubItemCommandId(null)
      setSubItems([])
      setSubQuery('')
      setQuery('')
      setPaletteMode('chord')
      setChordPath(['Leader'])
      setChordNode(null)
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

  const handleSelect = useCallback(
    (commandId: string) => {
      const cmd = findActiveCommand(commands, commandId)
      if (cmd?.command?.hasSubItems) {
        setSubItemCommandId(commandId)
        setSubItems([])
        setSubQuery('')
        enterFilterMode('')
        return
      }
      invokeCommand(commandId)
      handleOpenChange(false)
    },
    [commands, enterFilterMode, invokeCommand, handleOpenChange],
  )

  const selectChordContinuation = useCallback(
    (continuation: ChordContinuation) => {
      if (continuation.conflict) {
        setChordNode(continuation.node)
        setChordPath((current) => [...current, continuation.key])
        return
      }
      const binding = continuation.node.bindings[0]
      if (binding) {
        invokeCommand(binding.commandId)
        handleOpenChange(false)
        return
      }
      setChordNode(continuation.node)
      setChordPath((current) => [...current, continuation.key])
    },
    [handleOpenChange, invokeCommand],
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
    resetChord()
  }, [resetChord])

  const handlePaletteQueryChange = useCallback(
    (next: string) => {
      setQuery(next)
      if (next === '') resetChord()
      else setPaletteMode('filter')
    },
    [resetChord],
  )

  const stepChordBack = useCallback(() => {
    setChordPath((current) => {
      if (current.length <= 1) return current
      const nextPath = current.slice(0, -1)
      const nextNode = nodeForPath(rootChordNode, nextPath.slice(1))
      setChordNode(nextNode)
      return nextPath
    })
  }, [rootChordNode])

  const handlePaletteKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (subItemCommandId) return
      if (paletteMode === 'filter') {
        if (event.key === 'Escape') {
          event.preventDefault()
          if (query) resetChord()
          else handleOpenChange(false)
          return
        }
        if (event.key === 'Backspace') {
          if (query.length > 1) {
            event.preventDefault()
            setQuery(query.slice(0, -1))
            return
          }
          if (query.length === 1) {
            event.preventDefault()
            resetChord()
          }
          return
        }
        return
      }

      if (event.key === 'Escape') {
        event.preventDefault()
        if (chordPath.length <= 1) handleOpenChange(false)
        else stepChordBack()
        return
      }
      if (event.key === 'Backspace') {
        event.preventDefault()
        stepChordBack()
        return
      }
      if (event.key === '/') {
        event.preventDefault()
        enterFilterMode('')
        return
      }
      if (!isPrintableKey(event.nativeEvent)) return

      const combo = comboFromKeyboardEvent(event.nativeEvent)
      const nextNode = chordNode?.children.get(combo)
      if (nextNode) {
        event.preventDefault()
        selectChordContinuation({
          key: combo,
          label: chordLabel(combo, nextNode),
          commandId: nextNode.bindings[0]?.commandId,
          conflict: nextNode.conflicts.length > 0,
          node: nextNode,
        })
        return
      }
      event.preventDefault()
      enterFilterMode(event.key)
    },
    [
      chordNode,
      chordPath.length,
      enterFilterMode,
      handleOpenChange,
      paletteMode,
      query,
      resetChord,
      selectChordContinuation,
      stepChordBack,
      subItemCommandId,
    ],
  )

  const activeSubItemCommand = subItemCommandId
    ? findActiveCommand(commands, subItemCommandId)
    : undefined
  const placeholder = activeSubItemCommand
    ? `Search ${activeSubItemCommand.command?.label ?? ''}...`
    : paletteMode === 'chord'
      ? 'Type a chord, /, or a command name...'
      : 'Type a command or search...'
  const inputValue = subItemCommandId ? subQuery : query
  const inputChange = subItemCommandId ? setSubQuery : handlePaletteQueryChange

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      showCloseButton={false}
    >
      <div onKeyDownCapture={handlePaletteKeyDown}>
        <CommandInput
          ref={inputRef}
          placeholder={placeholder}
          value={inputValue}
          onClick={() => !subItemCommandId && enterFilterMode(query)}
          onValueChange={inputChange}
        />
        <CommandList>
          {subItemCommandId ? (
            <>
              <CommandEmpty>No items found.</CommandEmpty>
              <CommandGroup
                heading={activeSubItemCommand?.command?.label ?? ''}
              >
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
          ) : (
            <>
              <CommandEmpty>No commands found.</CommandEmpty>
              <CommandGroup
                heading={`${paletteMode === 'chord' ? 'Chord' : 'Filter'} mode`}
              >
                <div className="text-foreground-alt px-2 py-1 text-xs">
                  Chord path: {chordPath.map(formatResolvedKey).join(' ')}
                </div>
                {paletteMode === 'chord' &&
                  chordContinuations.map((continuation) => (
                    <CommandChordItem
                      key={`${continuation.key}:${
                        continuation.commandId ?? 'branch'
                      }`}
                      continuation={continuation}
                      onSelect={selectChordContinuation}
                    />
                  ))}
              </CommandGroup>
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
                        bindingGraph={bindingGraph}
                      />
                    )
                  })}
                </CommandGroup>
              ))}
            </>
          )}
        </CommandList>
        <CommandFooter />
      </div>
    </CommandDialog>
  )
}

function continuationsFromNode(
  node: KeybindingSequenceNode,
): ChordContinuation[] {
  return [...node.children.entries()].map(([key, child]) => ({
    key,
    label: chordLabel(key, child),
    commandId: child.bindings[0]?.commandId,
    conflict: child.conflicts.length > 0,
    node: child,
  }))
}

function chordLabel(key: string, node: KeybindingSequenceNode): string {
  const binding = node.bindings[0]
  if (binding) return binding.label
  return `${formatResolvedKey(key)} commands`
}

function nodeForPath(
  root: KeybindingSequenceNode | null,
  path: string[],
): KeybindingSequenceNode | null {
  let node = root
  for (const step of path) {
    node = node?.children.get(step) ?? null
    if (!node) return null
  }
  return node
}

function isPrintableKey(event: KeyboardEvent): boolean {
  return (
    event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey
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
