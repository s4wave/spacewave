import type { IconType } from '@s4wave/web/images/icons.js'
import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
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
} from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: { cursor: 'default' },
  rule4: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule5: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    minWidth: `calc(${themeLayout['--spacing']} * 10)`,
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    textAlign: 'center',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-brand'],
  },
  rule8: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule9: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule10: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule11: {
    maxHeight: themeLayout['--max-height-command-results'],
    scrollPaddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule12: { display: 'flex', flexDirection: 'column' },
  rule13: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

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
    ...(command?.searchAliases ?? []),
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
  // Build an escaped match pattern from the normalized semantic terms.
  const normalizedTerms = terms.flatMap(
    (term) => normalizeSearchText(term) || [],
  )
  if (normalizedTerms.length === 0) return null
  const termPattern = new RegExp(
    normalizedTerms
      .map((term) => term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
      .join('|'),
  )

  // Retain unmatched text while tracking the next token boundary.
  const content: ReactNode[] = []
  const tokenPattern = /[A-Za-z0-9]+/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  let highlighted = false

  // Highlight each matching token without changing the original text.
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
      <span key={match.index} sx={stylexStyles.rule1}>
        {token}
      </span>,
    )
    lastIndex = match.index + token.length
    highlighted = true
  }

  // Return plain rendering when every token misses, or append the trailing text.
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
  // Search the active command label and its displayed keyboard bindings.
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
  // Keep the original label when the query is empty.
  const trimmedQuery = query.trim()
  if (!trimmedQuery) return text

  // Prefer one contiguous highlighted substring.
  const lowerText = text.toLowerCase()
  const lowerQuery = trimmedQuery.toLowerCase()
  const start = lowerText.indexOf(lowerQuery)
  if (start >= 0) {
    const end = start + trimmedQuery.length
    return [
      text.slice(0, start),
      <span key="match" sx={stylexStyles.rule1}>
        {text.slice(start, end)}
      </span>,
      text.slice(end),
    ]
  }

  // Use semantic aliases when the visible label lacks the literal query.
  const semanticHighlight = highlightSemanticTerms(text, semanticTerms)
  if (semanticHighlight) return semanticHighlight

  // Fall back to ordered character matching across the visible label.
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
      <span key={index} sx={stylexStyles.rule1}>
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
const subItemIcons: Record<string, IconType> = {
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
  return <Icon sx={stylexStyles.rule2} />
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
  // Deduplicate active commands while retaining their menu-group order.
  const groups = new Map<string, CommandState[]>()
  const groupOrder = ['File', 'Edit', 'View', 'Tools', 'Help']
  const seen = new Set<string>()

  // Place each active command into its first menu-path group.
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

  // Emit standard menu groups first, followed by remaining groups.
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
  // Resolve the enabled command label, bindings and search aliases.
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
      variant={enabled ? 'palette' : 'paletteDisabled'}
      sx={!enabled ? stylexStyles.rule3 : undefined}
    >
      <span sx={stylexStyles.rule4}>
        <span sx={stylexStyles.rule5}>
          {highlightQueryText(label, query, aliasHighlightTerms)}
        </span>
        {cmd.command?.description && (
          <span sx={stylexStyles.rule6}>
            {highlightQueryText(
              cmd.command.description,
              query,
              aliasHighlightTerms,
            )}
          </span>
        )}
      </span>
      {displayBindings.length > 0 && (
        <CommandShortcut variant="brand">
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
      variant={continuation.conflict ? 'conflict' : undefined}
    >
      <kbd sx={stylexStyles.rule7}>{formatResolvedKey(continuation.key)}</kbd>
      <span sx={stylexStyles.rule4}>
        <span sx={stylexStyles.rule8}>{continuation.label}</span>
        {continuation.commandId && (
          <span sx={stylexStyles.rule9}>{continuation.commandId}</span>
        )}
      </span>
      {continuation.conflict && (
        <CommandShortcut variant="warning">Conflict</CommandShortcut>
      )}
    </CommandItem>
  )
}

// countCommands totals the commands across groups.
function countCommands(groups: { commands: unknown[] }[]): number {
  return groups.reduce((count, group) => count + group.commands.length, 0)
}

// countLabel formats a count with its singular or plural noun.
function countLabel(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`
}

// palettePlaceholder describes what the palette input accepts. A command that
// lists sub-items takes the input as its own search.
function palettePlaceholder(
  mode: PaletteMode,
  subItemCommand: ReturnType<typeof findActiveCommand>,
): string {
  if (subItemCommand) {
    return `Search ${subItemCommand.command?.label ?? ''}...`
  }
  if (mode === 'chord') return 'Type a chord, /, or a command name...'
  return 'Type a command or search...'
}

// paletteResultSummary summarizes the visible results. A null subItemCount or
// chordCount means the palette is not listing sub-items or chords.
function paletteResultSummary({
  subItemCount,
  chordCount,
  totalCommandCount,
  visibleCommandCount,
}: {
  subItemCount: number | null
  chordCount: number | null
  totalCommandCount: number
  visibleCommandCount: number
}): string {
  if (subItemCount !== null) return countLabel(subItemCount, 'item', 'items')
  if (chordCount !== null) {
    return `${countLabel(chordCount, 'chord', 'chords')} · ${totalCommandCount} commands`
  }
  return countLabel(visibleCommandCount, 'match', 'matches')
}

// useCommandPaletteController owns palette mode, chord traversal, sub-item
// loading, query projection, and command invocation.
function useCommandPaletteController() {
  // Subscribe to commands and retain the palette opening state.
  const [open, setOpen] = useState(false)
  const commands = useCommands()
  const invokeCommand = useInvokeCommand()
  const { getSubItems, registerOpenCommand } = useCommandContext()

  // Retain sub-item loading and the current palette navigation state.
  const [subItemCommandId, setSubItemCommandId] = useState<string | null>(null)
  const [subItems, setSubItems] = useState<SubItem[]>([])
  const [subQuery, setSubQuery] = useState('')
  const [query, setQuery] = useState('')

  // Retain chord traversal independently of the sub-item query.
  const [paletteMode, setPaletteMode] = useState<PaletteMode>('chord')
  const [chordPath, setChordPath] = useState<string[]>(['Leader'])
  const [chordNode, setChordNode] = useState<KeybindingSequenceNode | null>(
    null,
  )

  // Retain request cancellation and the current command snapshot across callbacks.
  const abortRef = useRef<AbortController | null>(null)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const commandsRef = useRef(commands)
  const grouped = useMemo(() => groupCommands(commands), [commands])
  const bindingGraph = useKeybindingGraph(commands, {
    surface: CommandSurface.WEB,
  })
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

  // Filter menu groups only while the root palette accepts a text query.
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

  // Project visible groups from the active mode and query.
  const resetChord = useCallback(() => {
    setPaletteMode('chord')
    setQuery('')
    setChordPath(['Leader'])
    setChordNode(rootChordNode)
  }, [rootChordNode])

  // Return chord navigation to the current leader node.
  const focusFilter = useCallback(() => {
    window.queueMicrotask(() => inputRef.current?.focus())
  }, [])

  // Focus filtering after the next rendered input is available.
  const enterFilterMode = useCallback(
    (nextQuery = '') => {
      setPaletteMode('filter')
      setQuery(nextQuery)
      focusFilter()
    },
    [focusFilter],
  )

  // Open the root palette after clearing any prior sub-item selection.
  const openMergedPalette = useCallback(() => {
    // Clear sub-item navigation before opening the command root.
    setSubItemCommandId(null)
    setSubItems([])
    setSubQuery('')
    resetChord()
    setOpen(true)
  }, [resetChord])

  // Keep command callbacks reading the latest watched command list.
  useEffect(() => {
    commandsRef.current = commands
  }, [commands])

  // Register command-driven opening and its sub-item destination.
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

  // Register the global palette shortcut through the command owner.
  useCommand({
    commandId: 'spacewave.view.palette',
    label: 'Command Palette',
    defaultBindings: [
      {
        id: 'global-palette',
        surface: CommandSurface.WEB,
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+K' } },
        when: CommandFocusContext.GLOBAL,
      },
      {
        id: 'global-palette-sequence',
        surface: CommandSurface.WEB,
        binding: { case: 'sequence', value: { steps: ['Leader', 'Space'] } },
        when: CommandFocusContext.GLOBAL,
      },
    ],
    menuPath: 'View/Command Palette',
    menuGroup: 10,
    menuOrder: 1,
    handler: openMergedPalette,
  })

  // Clear transient palette state and abort sub-item loading on close.
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

  // Load the active sub-item query until it changes or unmounts.
  useEffect(() => {
    // Leave the loader idle when the root command list is active.
    if (!subItemCommandId) return

    // Cancel the previous sub-item query before starting its replacement.
    abortRef.current?.abort()
    const abort = new AbortController()
    abortRef.current = abort

    // Publish results only while their request remains current.
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

  // Select a command destination or invoke its terminal action.
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

  // Follow a chord branch until it resolves to a command.
  const selectChordContinuation = useCallback(
    (continuation: ChordContinuation) => {
      // Keep conflicting continuations open for explicit selection.
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

  // Follow a sub-item navigation query or invoke the selected item.
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

  // Return from sub-items and cancel their outstanding request.
  const handleBack = useCallback(() => {
    // Discard sub-item state before restoring the root chord.
    setSubItemCommandId(null)
    setSubItems([])
    setSubQuery('')
    abortRef.current?.abort()
    abortRef.current = null
    resetChord()
  }, [resetChord])

  // Switch between text filtering and the empty-query chord view.
  const handlePaletteQueryChange = useCallback(
    (next: string) => {
      setQuery(next)
      if (next === '') resetChord()
      else setPaletteMode('filter')
    },
    [resetChord],
  )

  // Move back one chord segment while retaining the leader root.
  const stepChordBack = useCallback(() => {
    if (chordPath.length <= 1) return
    const nextPath = chordPath.slice(0, -1)
    setChordPath(nextPath)
    setChordNode(nodeForPath(rootChordNode, nextPath.slice(1)))
  }, [chordPath, rootChordNode])

  // Route keyboard input according to the current palette mode.
  const handlePaletteKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      // Let the native input own edits while filter mode handles Escape.
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

      // Handle root chord navigation before interpreting printable keys.
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

      // Resolve a printable key against the current chord node.
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

  // Project the visible input, placeholder and result count from palette state.
  const activeSubItemCommand = subItemCommandId
    ? findActiveCommand(commands, subItemCommandId)
    : undefined
  const placeholder = palettePlaceholder(paletteMode, activeSubItemCommand)
  const inputValue = subItemCommandId ? subQuery : query
  const inputChange = subItemCommandId ? setSubQuery : handlePaletteQueryChange
  const resultSummary = paletteResultSummary({
    subItemCount: subItemCommandId ? subItems.length : null,
    chordCount:
      paletteMode === 'chord' && !query ? chordContinuations.length : null,
    totalCommandCount: countCommands(grouped),
    visibleCommandCount: countCommands(filteredGrouped),
  })

  return {
    activeSubItemCommand,
    bindingGraph,
    chordContinuations,
    chordPath,
    enterFilterMode,
    filteredGrouped,
    handleBack,
    handleOpenChange,
    handlePaletteKeyDown,
    handleSelect,
    handleSubItemSelect,
    inputChange,
    inputRef,
    inputValue,
    open,
    paletteMode,
    placeholder,
    query,
    resultSummary,
    selectChordContinuation,
    subItemCommandId,
    subItems,
  }
}

// CommandPalette renders a searchable command palette dialog with chord and
// sub-item navigation modes.
export function CommandPalette() {
  const {
    activeSubItemCommand,
    bindingGraph,
    chordContinuations,
    chordPath,
    enterFilterMode,
    filteredGrouped,
    handleBack,
    handleOpenChange,
    handlePaletteKeyDown,
    handleSelect,
    handleSubItemSelect,
    inputChange,
    inputRef,
    inputValue,
    open,
    paletteMode,
    placeholder,
    query,
    resultSummary,
    selectChordContinuation,
    subItemCommandId,
    subItems,
  } = useCommandPaletteController()

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      showCloseButton={false}
      variant="palette"
    >
      <div onKeyDownCapture={handlePaletteKeyDown}>
        <CommandInput
          ref={inputRef}
          placeholder={placeholder}
          value={inputValue}
          onClick={() => !subItemCommandId && enterFilterMode(query)}
          onValueChange={inputChange}
        />
        <div sx={stylexStyles.rule10}>
          <span>{resultSummary}</span>
          <span sx={stylexStyles.rule8}>
            {subItemCommandId
              ? activeSubItemCommand?.command?.label
              : paletteMode === 'chord'
                ? `Chord · ${chordPath.map(formatResolvedKey).join(' ')}`
                : 'Filtering'}
          </span>
        </div>
        <CommandList variant="palette" sx={stylexStyles.rule11}>
          {subItemCommandId ? (
            <>
              <CommandEmpty>No items found.</CommandEmpty>
              <CommandGroup
                variant="palette"
                heading={activeSubItemCommand?.command?.label ?? ''}
              >
                <CommandItem
                  value="__back__"
                  onSelect={handleBack}
                  variant="back"
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
                    <span sx={stylexStyles.rule12}>
                      <span sx={stylexStyles.rule13}>
                        <span>{item.label}</span>
                        {item.experimental && <ExperimentalBadge />}
                      </span>
                      {item.description && (
                        <span sx={stylexStyles.rule14}>{item.description}</span>
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
                variant="palette"
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
                <CommandGroup key={g.group} variant="palette" heading={g.group}>
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
