import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from 'react'

import {
  LuBookOpen,
  LuBox,
  LuBriefcase,
  LuCpu,
  LuFolder,
  LuGitBranch,
  LuHammer,
  LuHardDrive,
  LuImage,
  LuLayoutGrid,
  LuListTodo,
  LuMessageSquare,
  LuMonitor,
  LuNotebookPen,
  LuPanelTop,
  LuPenLine,
  LuServer,
} from 'react-icons/lu'

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
import { ExperimentalBadge } from '@s4wave/web/ui/ExperimentalBadge.js'
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
import { getSubItemQuery } from './sub-item-navigation.js'

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
      case 'meta':
        display.push(isMacPlatform ? '\u2318' : 'Meta')
        break
      case 'Ctrl':
      case 'ctrl':
        display.push(isMacPlatform ? '\u2303' : 'Ctrl')
        break
      case 'Shift':
      case 'shift':
        display.push(isMacPlatform ? '\u21E7' : 'Shift')
        break
      case 'Alt':
      case 'alt':
      case 'Option':
      case 'option':
        display.push(isMacPlatform ? '\u2325' : 'Alt')
        break
      default:
        display.push(part)
    }
  }

  return display.join(isMacPlatform ? '' : '+')
}

export function formatKeybindingHint(bindings: string[]): string {
  return [...new Set(bindings.map(formatKeybinding))].join(' / ')
}

function commandSearchValue(
  cmd: CommandState,
  displayBindings: readonly string[] = [],
): string {
  const command = cmd.command
  const text = [
    command?.label,
    command?.description,
    command?.commandId,
    command?.menuPath,
    ...displayBindings,
  ]
    .filter(Boolean)
    .join(' ')
  return [text, keyboardShortcutAliases(text)].filter(Boolean).join(' ')
}

function keyboardShortcutAliases(text: string): string {
  const normalized = text.toLowerCase()
  if (!normalized.includes('keyboard') && !normalized.includes('shortcut')) {
    return ''
  }
  return 'keybind keybinding keybindings hotkey hotkeys'
}

const keyboardShortcutAliasHighlightTerms = [
  'keyboard',
  'shortcut',
  'keybinding',
  'hotkey',
]

function keyboardShortcutAliasMatches(text: string, query: string): boolean {
  const normalizedQuery = normalizeSearchText(query)
  if (!normalizedQuery) return false
  return normalizeSearchText(keyboardShortcutAliases(text)).includes(
    normalizedQuery,
  )
}

function highlightSemanticTerms(text: string, terms: readonly string[]) {
  const normalizedTerms = terms.flatMap(
    (term) => normalizeSearchText(term) || [],
  )
  if (normalizedTerms.length === 0) return null
  const termPattern = new RegExp(
    normalizedTerms
      .map((term) => term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
      .join('|'),
  )

  const content: ReactNode[] = []
  const tokenPattern = /[A-Za-z0-9]+/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  let highlighted = false

  while ((match = tokenPattern.exec(text))) {
    const token = match[0]
    const normalizedToken = normalizeSearchText(token)
    if (!termPattern.test(normalizedToken)) {
      continue
    }

    if (match.index > lastIndex) {
      content.push(text.slice(lastIndex, match.index))
    }
    content.push(
      <span key={match.index} className="text-brand font-semibold">
        {token}
      </span>,
    )
    lastIndex = match.index + token.length
    highlighted = true
  }

  if (!highlighted) return null
  if (lastIndex < text.length) {
    content.push(text.slice(lastIndex))
  }
  return content
}

function normalizeSearchText(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, '')
}

function commandMatchesQuery(
  cmd: CommandState,
  bindingGraph: KeybindingGraph,
  query: string,
): boolean {
  const normalizedQuery = normalizeSearchText(query)
  if (!normalizedQuery) return true
  const commandId = cmd.command?.commandId
  const displayBindings = commandId
    ? getCommandDisplayBindings(bindingGraph, commandId)
    : []
  return normalizeSearchText(commandSearchValue(cmd, displayBindings)).includes(
    normalizedQuery,
  )
}

function highlightQueryText(
  text: string,
  query: string,
  semanticTerms: readonly string[] = [],
) {
  const trimmedQuery = query.trim()
  if (!trimmedQuery) return text

  const lowerText = text.toLowerCase()
  const lowerQuery = trimmedQuery.toLowerCase()
  const start = lowerText.indexOf(lowerQuery)
  if (start >= 0) {
    const end = start + trimmedQuery.length
    return [
      text.slice(0, start),
      <span key="match" className="text-brand font-semibold">
        {text.slice(start, end)}
      </span>,
      text.slice(end),
    ]
  }

  const semanticHighlight = highlightSemanticTerms(text, semanticTerms)
  if (semanticHighlight) return semanticHighlight

  const matched = new Set<number>()
  let queryIndex = 0
  for (let textIndex = 0; textIndex < text.length; textIndex++) {
    if (text[textIndex]?.toLowerCase() !== lowerQuery[queryIndex]) continue
    matched.add(textIndex)
    queryIndex++
    if (queryIndex === lowerQuery.length) break
  }
  if (queryIndex !== lowerQuery.length) return text

  return [...text].map((char, index) =>
    matched.has(index) ? (
      <span key={index} className="text-brand font-semibold">
        {char}
      </span>
    ) : (
      char
    ),
  )
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

// subItemIcons maps SubItem.iconName react-icons identifiers (the same
// vocabulary as ObjectWizard.IconName) to icon components.
const subItemIcons: Record<string, ComponentType<{ className?: string }>> = {
  LuBookOpen,
  LuBox,
  LuBriefcase,
  LuCpu,
  LuFolder,
  LuGitBranch,
  LuHammer,
  LuHardDrive,
  LuImage,
  LuLayoutGrid,
  LuListTodo,
  LuMessageSquare,
  LuMonitor,
  LuNotebookPen,
  LuPanelTop,
  LuPenLine,
  LuServer,
}

// SubItemIcon renders the icon slot for a palette sub-item. Unknown icon
// names fall back to a generic box so icon-bearing lists stay aligned.
function SubItemIcon({ iconName }: { iconName?: string }) {
  if (!iconName) return null
  const Icon = subItemIcons[iconName] ?? LuBox
  return <Icon className="text-foreground-alt size-4 shrink-0" />
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
  query,
  onSelect,
}: {
  cmd: CommandState
  bindingGraph: KeybindingGraph
  query: string
  onSelect: (commandId: string) => void
}) {
  const commandId = cmd.command?.commandId
  if (!commandId) return null
  const enabled = isCommandEnabled(cmd)
  const displayBindings = getCommandDisplayBindings(bindingGraph, commandId)
  const label = cmd.command?.label ?? commandId
  const searchValue = commandSearchValue(cmd, displayBindings)
  const aliasHighlightTerms = keyboardShortcutAliasMatches(searchValue, query)
    ? keyboardShortcutAliasHighlightTerms
    : []
  return (
    <CommandItem
      key={commandId}
      value={searchValue}
      onSelect={() => enabled && onSelect(commandId)}
      disabled={!enabled}
      className={cn(
        'min-h-12 rounded-none border-b border-foreground/6 px-3 py-2 data-[selected=true]:bg-brand/25',
        !enabled && 'cursor-default opacity-50',
      )}
    >
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm font-medium">
          {highlightQueryText(label, query, aliasHighlightTerms)}
        </span>
        {cmd.command?.description && (
          <span className="text-foreground-alt/60 truncate text-xs">
            {highlightQueryText(
              cmd.command.description,
              query,
              aliasHighlightTerms,
            )}
          </span>
        )}
      </span>
      {displayBindings.length > 0 && (
        <CommandShortcut className="text-brand/90 shrink-0 pl-4">
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
      <kbd className="bg-brand/10 text-brand min-w-10 rounded px-2 py-0.5 text-center font-mono text-xs">
        {formatResolvedKey(continuation.key)}
      </kbd>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{continuation.label}</span>
        {continuation.commandId && (
          <span className="text-foreground-alt/60 truncate font-mono text-xs">
            {continuation.commandId}
          </span>
        )}
      </span>
      {continuation.conflict && (
        <CommandShortcut className="text-warning">Conflict</CommandShortcut>
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
  const filteredGrouped = useMemo(() => {
    if (subItemCommandId || paletteMode === 'chord' || !query.trim()) {
      return grouped
    }
    return grouped.flatMap((group) => {
      const commands = group.commands.filter((cmd) =>
        commandMatchesQuery(cmd, bindingGraph, query),
      )
      return commands.length > 0 ? [{ ...group, commands }] : []
    })
  }, [bindingGraph, grouped, paletteMode, query, subItemCommandId])

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
      const nextQuery = getSubItemQuery(subItemId)
      if (nextQuery !== null) {
        setSubQuery(nextQuery)
        focusFilter()
        return
      }
      if (subItemCommandId) {
        invokeCommand(subItemCommandId, { subItemId })
      }
      handleOpenChange(false)
    },
    [focusFilter, subItemCommandId, invokeCommand, handleOpenChange],
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
        // The focused native input owns text editing in filter mode, including
        // selection deletes after select-all. Intercepting Backspace here to
        // slice one character broke select-all + delete; let the input edit its
        // own value and rely on handlePaletteQueryChange to restore chord mode
        // when onValueChange reports an empty query.
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
  const visibleCommandCount = filteredGrouped.reduce(
    (count, group) => count + group.commands.length,
    0,
  )
  const totalCommandCount = grouped.reduce(
    (count, group) => count + group.commands.length,
    0,
  )
  const resultSummary = subItemCommandId
    ? `${subItems.length} ${subItems.length === 1 ? 'item' : 'items'}`
    : paletteMode === 'chord' && !query
      ? `${chordContinuations.length} ${
          chordContinuations.length === 1 ? 'chord' : 'chords'
        } · ${totalCommandCount} commands`
      : `${visibleCommandCount} ${
          visibleCommandCount === 1 ? 'match' : 'matches'
        }`

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      showCloseButton={false}
      className="border-foreground/10 bg-background-card/95 top-auto bottom-4 max-h-[min(34rem,calc(100vh-4rem))] w-[min(64rem,calc(100vw-2rem))] translate-y-0 overflow-hidden shadow-none sm:max-w-none"
    >
      <div onKeyDownCapture={handlePaletteKeyDown}>
        <CommandInput
          ref={inputRef}
          placeholder={placeholder}
          value={inputValue}
          onClick={() => !subItemCommandId && enterFilterMode(query)}
          onValueChange={inputChange}
        />
        <div className="border-foreground/8 text-foreground-alt/60 flex h-7 items-center justify-between gap-3 border-b px-3 font-mono text-[10px]">
          <span>{resultSummary}</span>
          <span className="truncate">
            {subItemCommandId
              ? activeSubItemCommand?.command?.label
              : paletteMode === 'chord'
                ? `Chord · ${chordPath.map(formatResolvedKey).join(' ')}`
                : 'Filtering'}
          </span>
        </div>
        <CommandList className="max-h-[min(24rem,calc(100vh-12rem))] scroll-py-2 pb-0">
          {subItemCommandId ? (
            <>
              <CommandEmpty>No items found.</CommandEmpty>
              <CommandGroup
                className="!px-0 [&_[cmdk-group-heading]]:px-3"
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
                    <SubItemIcon iconName={item.iconName} />
                    <span className="flex flex-col">
                      <span className="flex items-center gap-1.5">
                        <span>{item.label}</span>
                        {item.experimental && <ExperimentalBadge />}
                      </span>
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
                className="!px-0 [&_[cmdk-group-heading]]:px-3"
                heading={`${paletteMode === 'chord' ? 'Chord' : 'Filter'} mode`}
              >
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
              {filteredGrouped.map((g) => (
                <CommandGroup
                  key={g.group}
                  className="!px-0 [&_[cmdk-group-heading]]:px-3"
                  heading={g.group}
                >
                  {g.commands.map((cmd) => {
                    const commandId = cmd.command?.commandId
                    if (!commandId) return null
                    return (
                      <CommandPaletteItem
                        key={commandId}
                        cmd={cmd}
                        onSelect={handleSelect}
                        bindingGraph={bindingGraph}
                        query={query}
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
