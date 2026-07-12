import {
  CommandFocusContext,
  type Command,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'
import {
  defaultKeybindingSettings,
  resolveKeybindingSettings,
  type KeybindingOverrideLayer,
  type KeybindingOverrideScope,
} from './keybinding-overrides.js'

const legacyBindingId = 'legacy-keybinding'

export type KeybindingPlatform = 'mac' | 'other'

export type ResolvedBindingKind = 'combo' | 'sequence'

export interface KeybindingResolverOptions {
  platform?: KeybindingPlatform
  leaderCombo?: string
  overrideLayers?: KeybindingOverrideLayer[]
}

export interface SelectedKeybindingMatch {
  binding?: ResolvedCommandBinding
  conflict?: KeybindingConflict
}

export interface ResolvedCommandBinding {
  commandId: string
  label: string
  bindingId: string
  kind: ResolvedBindingKind
  context: CommandFocusContext
  combo?: string
  sequence?: string[]
  display: string
  command: Command
  source: CommandBinding
  sourceLayer: KeybindingOverrideScope | 'default'
  sourceLayerLabel: string
}

export interface KeybindingConflict {
  context: CommandFocusContext
  kind: ResolvedBindingKind
  key: string
  bindings: ResolvedCommandBinding[]
}

export interface KeybindingSequenceNode {
  step?: string
  children: Map<string, KeybindingSequenceNode>
  bindings: ResolvedCommandBinding[]
  conflicts: KeybindingConflict[]
}

export interface KeybindingGraph {
  bindingsByCommandId: Map<string, ResolvedCommandBinding[]>
  comboBindings: Map<string, ResolvedCommandBinding>
  comboConflicts: Map<string, KeybindingConflict>
  sequenceTrie: KeybindingSequenceNode
  conflicts: KeybindingConflict[]
  leaderCombo: string
  whichKeyDelayMs: number
}

export function resolveKeybindings(
  commands: CommandState[],
  opts: KeybindingResolverOptions = {},
): KeybindingGraph {
  const platform = opts.platform ?? detectPlatform()
  const overrideLayers = opts.overrideLayers ?? []
  const settings = resolveKeybindingSettings(overrideLayers, {
    ...defaultKeybindingSettings,
    leaderCombo: opts.leaderCombo ?? defaultKeybindingSettings.leaderCombo,
  })
  const leaderCombo = settings.leaderCombo
  const bindings = collectBindings(
    commands,
    platform,
    leaderCombo,
    overrideLayers,
  )
  const bindingsByCommandId = groupBindingsByCommandId(bindings)
  const comboBuckets = bucketBindings(
    bindings.filter((binding) => binding.kind === 'combo'),
    (binding) => binding.combo ?? '',
  )
  const sequenceBuckets = bucketBindings(
    bindings.filter((binding) => binding.kind === 'sequence'),
    (binding) => (binding.sequence ?? []).join(' '),
  )
  const comboConflicts = new Map<string, KeybindingConflict>()
  const comboBindings = new Map<string, ResolvedCommandBinding>()
  const conflicts: KeybindingConflict[] = []

  for (const [key, bucket] of comboBuckets) {
    if (bucket.length > 1) {
      const conflict = buildConflict(bucket, 'combo', key)
      comboConflicts.set(key, conflict)
      conflicts.push(conflict)
      continue
    }
    comboBindings.set(key, bucket[0])
  }

  const sequenceTrie = createSequenceNode()
  for (const [key, bucket] of sequenceBuckets) {
    if (bucket.length > 1) {
      const conflict = buildConflict(bucket, 'sequence', key)
      conflicts.push(conflict)
      insertSequence(sequenceTrie, bucket[0].sequence ?? [], [], conflict)
      continue
    }
    insertSequence(sequenceTrie, bucket[0].sequence ?? [], bucket, undefined)
  }

  return {
    bindingsByCommandId,
    comboBindings,
    comboConflicts,
    sequenceTrie,
    conflicts,
    leaderCombo,
    whichKeyDelayMs: settings.whichKeyDelayMs,
  }
}

export function normalizeKeyCombo(
  binding: string,
  platform: KeybindingPlatform = detectPlatform(),
): string {
  const parts = binding.split('+').flatMap((part) => {
    const trimmed = part.trim()
    return trimmed ? [trimmed] : []
  })
  let meta = false
  let ctrl = false
  let alt = false
  let shift = false
  let key = ''

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (i === parts.length - 1) {
      key = normalizeKey(part)
      continue
    }
    switch (part.toLowerCase()) {
      case 'cmdorctrl':
        if (platform === 'mac') {
          meta = true
        } else {
          ctrl = true
        }
        break
      case 'cmd':
      case 'meta':
        meta = true
        break
      case 'ctrl':
      case 'control':
        ctrl = true
        break
      case 'alt':
      case 'option':
        alt = true
        break
      case 'shift':
        shift = true
        break
    }
  }

  const normalized: string[] = []
  if (meta) normalized.push('meta')
  if (ctrl) normalized.push('ctrl')
  if (alt) normalized.push('alt')
  if (shift) normalized.push('shift')
  normalized.push(key)
  return normalized.join('+')
}

export function comboFromKeyboardEvent(event: KeyboardEvent): string {
  const parts: string[] = []
  if (event.metaKey) parts.push('meta')
  if (event.ctrlKey) parts.push('ctrl')
  if (event.altKey) parts.push('alt')
  if (event.shiftKey) parts.push('shift')
  parts.push(normalizeKey(event.key))
  return parts.join('+')
}

export function isModifierKey(event: KeyboardEvent): boolean {
  return (
    event.key === 'Shift' ||
    event.key === 'Control' ||
    event.key === 'Alt' ||
    event.key === 'Meta'
  )
}

export function createSequenceNode(step?: string): KeybindingSequenceNode {
  return {
    step,
    children: new Map(),
    bindings: [],
    conflicts: [],
  }
}

export function contextKey(context: CommandFocusContext, key: string): string {
  return `${context}:${key}`
}

export function getCommandDisplayBindings(
  graph: KeybindingGraph,
  commandId: string,
  kind?: ResolvedBindingKind,
): string[] {
  const commandBindings = graph.bindingsByCommandId.get(commandId) ?? []
  const bindings = kind
    ? commandBindings.filter((binding) => binding.kind === kind)
    : commandBindings
  const allBindings = [...graph.bindingsByCommandId.values()]
    .flat()
    .filter((binding) => !kind || binding.kind === kind)
  return bindings.map((binding) =>
    bindingNeedsContextLabel(binding, allBindings)
      ? `${binding.display} (${focusContextLabel(binding.context)})`
      : binding.display,
  )
}

// getCommandMenuBinding returns one canonical combo binding for a menu row.
export function getCommandMenuBinding(
  graph: KeybindingGraph,
  commandId: string,
): string | undefined {
  const commandBindings = graph.bindingsByCommandId.get(commandId) ?? []
  let binding: ResolvedCommandBinding | undefined
  for (const candidate of commandBindings) {
    if (candidate.kind !== 'combo') continue
    if (
      !binding ||
      bindingLayerPrecedence(candidate) >= bindingLayerPrecedence(binding)
    ) {
      binding = candidate
    }
  }
  if (!binding) return undefined

  const allBindings = [...graph.bindingsByCommandId.values()]
    .flat()
    .filter((candidate) => candidate.kind === 'combo')
  return bindingNeedsContextLabel(binding, allBindings)
    ? `${binding.display} (${focusContextLabel(binding.context)})`
    : binding.display
}

function bindingLayerPrecedence(binding: ResolvedCommandBinding): number {
  switch (binding.sourceLayer) {
    case 'space':
      return 3
    case 'account':
      return 2
    case 'local':
      return 1
    default:
      return 0
  }
}

export function focusContextLabel(context: CommandFocusContext): string {
  switch (context) {
    case CommandFocusContext.SHELL_TAB:
      return 'Shell Tab'
    case CommandFocusContext.EDITOR:
      return 'Editor'
    case CommandFocusContext.LIST:
      return 'List'
    case CommandFocusContext.CANVAS:
      return 'Canvas'
    case CommandFocusContext.MODAL:
      return 'Modal'
    case CommandFocusContext.TEXT_INPUT:
      return 'Text Input'
    default:
      return 'Global'
  }
}

export function selectActiveComboMatch(
  graph: KeybindingGraph,
  activeFocusContexts: readonly CommandFocusContext[],
  combo: string,
): SelectedKeybindingMatch | undefined {
  for (const context of reverseContexts(
    activeBindingContexts(activeFocusContexts),
  )) {
    const key = contextKey(context, combo)
    const conflict = graph.comboConflicts.get(key)
    if (conflict) return { conflict }
    const binding = graph.comboBindings.get(key)
    if (binding) return { binding }
  }
  return undefined
}

export function selectActiveSequenceNode(
  node: KeybindingSequenceNode,
  activeFocusContexts: readonly CommandFocusContext[],
): KeybindingSequenceNode | undefined {
  return selectSequenceNode(node, activeBindingContexts(activeFocusContexts))
}

interface LayeredCommandBinding {
  binding: CommandBinding
  sourceLayer: KeybindingOverrideScope | 'default'
  sourceLayerLabel: string
}

function collectBindings(
  commands: CommandState[],
  platform: KeybindingPlatform,
  leaderCombo: string,
  overrideLayers: KeybindingOverrideLayer[],
): ResolvedCommandBinding[] {
  const bindings: ResolvedCommandBinding[] = []
  const leaderStep = normalizeKeyCombo(leaderCombo, platform)

  for (const state of commands) {
    const command = state.command
    const commandId = command?.commandId
    if (!command || !commandId || !state.active || state.enabled === false) {
      continue
    }

    for (const layeredBinding of commandEffectiveBindings(
      command,
      commandId,
      overrideLayers,
    )) {
      const resolved = resolveCommandBinding(
        command,
        commandId,
        layeredBinding,
        platform,
        leaderStep,
      )
      if (resolved) bindings.push(resolved)
    }
  }

  return bindings
}

function commandDefaultBindings(command: Command): CommandBinding[] {
  if (command.defaultBindings?.length) return command.defaultBindings
  if (!command.keybinding) return []
  return [
    {
      id: legacyBindingId,
      binding: { case: 'combo', value: { combo: command.keybinding } },
      when: CommandFocusContext.GLOBAL,
    },
  ]
}

function commandEffectiveBindings(
  command: Command,
  commandId: string,
  overrideLayers: KeybindingOverrideLayer[],
): LayeredCommandBinding[] {
  let bindings: LayeredCommandBinding[] = commandDefaultBindings(command).map(
    (binding) => ({
      binding,
      sourceLayer: 'default',
      sourceLayerLabel: 'Default',
    }),
  )

  for (const layer of overrideLayers) {
    const override = layer.overrideSet.overrides[commandId]
    if (!override) continue
    if (override.disabled) {
      bindings = []
      continue
    }
    if (override.replaceBindings) bindings = []
    if (override.clearedBindingIds?.length) {
      const cleared = new Set(override.clearedBindingIds)
      bindings = bindings.filter(
        (binding) =>
          !cleared.has(binding.binding.id || defaultBindingId(binding.binding)),
      )
    }
    bindings.push(
      ...(override.bindings ?? []).map((binding) => ({
        binding,
        sourceLayer: layer.scope,
        sourceLayerLabel: layer.label,
      })),
    )
  }

  return bindings
}

function resolveCommandBinding(
  command: Command,
  commandId: string,
  layeredBinding: LayeredCommandBinding,
  platform: KeybindingPlatform,
  leaderStep: string,
): ResolvedCommandBinding | null {
  const binding = layeredBinding.binding
  const context =
    binding.when == null || binding.when === CommandFocusContext.UNSPECIFIED
      ? CommandFocusContext.GLOBAL
      : binding.when
  const bindingId = binding.id || defaultBindingId(binding)
  if (binding.binding?.case === 'sequence') {
    const sourceSequence = binding.binding.value.steps ?? []
    const sequence = sourceSequence.flatMap((step) => {
      const normalized =
        step === 'Leader' ? leaderStep : normalizeKeyCombo(step, platform)
      return normalized ? [normalized] : []
    })
    if (!sequence.length) return null
    return {
      commandId,
      label: command.label ?? commandId,
      bindingId,
      kind: 'sequence',
      context,
      sequence,
      display: sourceSequence.join(' ') || sequence.join(' '),
      command,
      source: binding,
      sourceLayer: layeredBinding.sourceLayer,
      sourceLayerLabel: layeredBinding.sourceLayerLabel,
    }
  }

  if (binding.binding?.case !== 'combo') return null
  const comboText = binding.binding.value.combo
  if (!comboText) return null
  const combo = normalizeKeyCombo(comboText, platform)
  if (!combo) return null
  return {
    commandId,
    label: command.label ?? commandId,
    bindingId,
    kind: 'combo',
    context,
    combo,
    display: comboText,
    command,
    source: binding,
    sourceLayer: layeredBinding.sourceLayer,
    sourceLayerLabel: layeredBinding.sourceLayerLabel,
  }
}

function defaultBindingId(binding: CommandBinding): string {
  if (binding.binding?.case === 'sequence') return 'default-sequence'
  return 'default-combo'
}

function activeBindingContexts(
  activeFocusContexts: readonly CommandFocusContext[],
): CommandFocusContext[] {
  const normalizedContexts = normalizeActiveFocusContexts(activeFocusContexts)
  if (
    !normalizedContexts.includes(CommandFocusContext.EDITOR) &&
    !normalizedContexts.includes(CommandFocusContext.TEXT_INPUT)
  ) {
    return normalizedContexts
  }
  return normalizedContexts.filter(
    (context) =>
      context === CommandFocusContext.EDITOR ||
      context === CommandFocusContext.TEXT_INPUT,
  )
}

function normalizeActiveFocusContexts(
  activeFocusContexts: readonly CommandFocusContext[],
): CommandFocusContext[] {
  const normalized: CommandFocusContext[] = []
  const seen = new Set<CommandFocusContext>()
  for (const context of [CommandFocusContext.GLOBAL, ...activeFocusContexts]) {
    if (context === CommandFocusContext.UNSPECIFIED || seen.has(context)) {
      continue
    }
    seen.add(context)
    normalized.push(context)
  }
  return normalized
}

function reverseContexts(
  contexts: readonly CommandFocusContext[],
): CommandFocusContext[] {
  return [...contexts].reverse()
}

function selectSequenceNode(
  node: KeybindingSequenceNode,
  activeContexts: readonly CommandFocusContext[],
): KeybindingSequenceNode | undefined {
  const children = new Map<string, KeybindingSequenceNode>()
  for (const [step, child] of node.children) {
    const selectedChild = selectSequenceNode(child, activeContexts)
    if (selectedChild) children.set(step, selectedChild)
  }

  const terminal = selectSequenceTerminal(node, activeContexts)
  const bindings = terminal?.binding ? [terminal.binding] : []
  const conflicts = terminal?.conflict ? [terminal.conflict] : []
  if (!children.size && !bindings.length && !conflicts.length) return undefined

  return {
    step: node.step,
    children,
    bindings,
    conflicts,
  }
}

function selectSequenceTerminal(
  node: KeybindingSequenceNode,
  activeContexts: readonly CommandFocusContext[],
): SelectedKeybindingMatch | undefined {
  const conflictsByContext = new Map(
    node.conflicts.map((candidate) => [candidate.context, candidate]),
  )
  const bindingsByContext = new Map(
    node.bindings.map((candidate) => [candidate.context, candidate]),
  )
  for (const context of reverseContexts(activeContexts)) {
    const conflict = conflictsByContext.get(context)
    if (conflict) return { conflict }
    const binding = bindingsByContext.get(context)
    if (binding) return { binding }
  }
  return undefined
}

function bindingNeedsContextLabel(
  binding: ResolvedCommandBinding,
  bindings: ResolvedCommandBinding[],
): boolean {
  const key = bindingMatchKey(binding)
  for (const other of bindings) {
    if (other === binding) continue
    if (other.context === binding.context) continue
    if (bindingMatchKey(other) === key) return true
  }
  return false
}

function bindingMatchKey(binding: ResolvedCommandBinding): string {
  if (binding.kind === 'sequence') {
    return `${binding.kind}:${(binding.sequence ?? []).join(' ')}`
  }
  return `${binding.kind}:${binding.combo ?? ''}`
}

function groupBindingsByCommandId(
  bindings: ResolvedCommandBinding[],
): Map<string, ResolvedCommandBinding[]> {
  const grouped = new Map<string, ResolvedCommandBinding[]>()
  for (const binding of bindings) {
    let list = grouped.get(binding.commandId)
    if (!list) {
      list = []
      grouped.set(binding.commandId, list)
    }
    list.push(binding)
  }
  return grouped
}

function bucketBindings(
  bindings: ResolvedCommandBinding[],
  getKey: (binding: ResolvedCommandBinding) => string,
): Map<string, ResolvedCommandBinding[]> {
  const buckets = new Map<string, ResolvedCommandBinding[]>()
  for (const binding of bindings) {
    const key = contextKey(binding.context, getKey(binding))
    let bucket = buckets.get(key)
    if (!bucket) {
      bucket = []
      buckets.set(key, bucket)
    }
    bucket.push(binding)
  }
  return buckets
}

function buildConflict(
  bindings: ResolvedCommandBinding[],
  kind: ResolvedBindingKind,
  key: string,
): KeybindingConflict {
  return {
    context: bindings[0].context,
    kind,
    key: key.split(':').slice(1).join(':'),
    bindings,
  }
}

function insertSequence(
  root: KeybindingSequenceNode,
  sequence: string[],
  bindings: ResolvedCommandBinding[],
  conflict: KeybindingConflict | undefined,
): void {
  if (!sequence.length) return
  let node = root
  for (const step of sequence) {
    let child = node.children.get(step)
    if (!child) {
      child = createSequenceNode(step)
      node.children.set(step, child)
    }
    node = child
  }
  if (conflict) {
    node.conflicts.push(conflict)
    return
  }
  node.bindings.push(...bindings)
}

function normalizeKey(key: string): string {
  if (key === ' ') return 'space'
  return key.trim().toLowerCase()
}

function detectPlatform(): KeybindingPlatform {
  if (typeof navigator !== 'undefined' && navigator.platform.includes('Mac')) {
    return 'mac'
  }
  return 'other'
}
