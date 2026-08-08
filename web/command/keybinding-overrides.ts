import {
  CommandSurface,
  KeybindingDisplayMode,
  type CommandBinding,
  type KeybindingCommandOverride as ProtoKeybindingCommandOverride,
  type KeybindingOverrideSet as ProtoKeybindingOverrideSet,
} from '@s4wave/sdk/command/command.pb.js'

export type KeybindingOverrideScope = 'local' | 'account' | 'space'

export interface KeybindingDisplaySettings {
  mode?: 'symbols' | 'text'
}

export interface KeybindingOverrideSettings {
  leaderCombo?: string
  whichKeyDelayMs?: number
  display?: KeybindingDisplaySettings
}

export interface EffectiveKeybindingSettings {
  leaderCombo: string
  whichKeyDelayMs: number
  display?: KeybindingDisplaySettings
}

export const defaultKeybindingSettings: EffectiveKeybindingSettings = {
  leaderCombo: 'Ctrl+Space',
  whichKeyDelayMs: 0,
}

export interface KeybindingCommandOverride {
  replaceBindings?: boolean
  disabled?: boolean
  clearedBindingIds?: string[]
  bindings?: CommandBinding[]
}

export interface KeybindingOverrideSet {
  version: 1 | 2
  overrides: Record<string, KeybindingCommandOverride>
  settings: KeybindingOverrideSettings
}

export interface KeybindingOverrideLayer {
  scope: KeybindingOverrideScope
  label: string
  overrideSet: KeybindingOverrideSet
}

export const localKeybindingStoreNamespace = ['keybindings'] as const
export const localKeybindingStoreKey = 'local'
export const localKeybindingStoreId = 'keybindings/local'

export function createEmptyKeybindingOverrideSet(): KeybindingOverrideSet {
  return { version: 2, overrides: {}, settings: {} }
}

export function createKeybindingOverrideLayer(
  scope: KeybindingOverrideScope,
  label: string,
  overrideSet: KeybindingOverrideSet,
): KeybindingOverrideLayer {
  return {
    scope,
    label,
    overrideSet: normalizeKeybindingOverrideSet(overrideSet),
  }
}

export type KeybindingMigrationDiagnosticCode =
  | 'duplicate-command-id'
  | 'empty-command-id'
  | 'unmapped-command-id'
export interface KeybindingMigrationDiagnostic {
  code: KeybindingMigrationDiagnosticCode
  commandId: string
  index: number
}
export interface KeybindingMigrationResult {
  overrideSet: KeybindingOverrideSet
  required: boolean
  diagnostics: KeybindingMigrationDiagnostic[]
}

export function keybindingMigrationError(
  diagnostics: readonly KeybindingMigrationDiagnostic[],
): Error | null {
  if (!diagnostics.length) return null
  const detail = diagnostics
    .map(
      (diagnostic) =>
        `${diagnostic.code}:${diagnostic.commandId || '<empty>'}@${diagnostic.index}`,
    )
    .join(', ')
  return new Error(`keybinding migration requires action: ${detail}`)
}

export function keybindingOverrideSetFromProto(
  value: ProtoKeybindingOverrideSet | null | undefined,
  surface: CommandSurface = CommandSurface.WEB,
): KeybindingOverrideSet {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI) {
    throw new Error('keybinding override parsing requires WEB or TUI surface')
  }
  if (!value) return createEmptyKeybindingOverrideSet()
  if (value.version === 1) {
    throw new Error('legacy keybinding override set requires migration')
  }
  return readKeybindingOverrideSet(value, surface)
}

// migrateLegacyKeybindingOverrideSet is the only reader of the v1 partition.
export function migrateLegacyKeybindingOverrideSet(
  value: ProtoKeybindingOverrideSet,
  canonicalCommandIds: ReadonlySet<string>,
): KeybindingMigrationResult {
  if (value.version !== 1) {
    return {
      overrideSet: readKeybindingOverrideSet(value, CommandSurface.WEB),
      required: false,
      diagnostics: [],
    }
  }
  const diagnostics: KeybindingMigrationDiagnostic[] = []
  const seen = new Set<string>()
  for (const [index, row] of (value.overrides ?? []).entries()) {
    const commandId = row.commandId ?? ''
    if (!commandId)
      diagnostics.push({ code: 'empty-command-id', commandId: '', index })
    else if (seen.has(commandId))
      diagnostics.push({ code: 'duplicate-command-id', commandId, index })
    else if (!canonicalCommandIds.has(commandId))
      diagnostics.push({ code: 'unmapped-command-id', commandId, index })
    seen.add(commandId)
  }
  diagnostics.sort(
    (a, b) =>
      a.commandId.localeCompare(b.commandId) ||
      a.code.localeCompare(b.code) ||
      a.index - b.index,
  )
  return {
    overrideSet: readOverridePartition(
      value.overrides ?? [],
      value.settings,
      CommandSurface.WEB,
      true,
    ),
    required: true,
    diagnostics,
  }
}

function readKeybindingOverrideSet(
  value: ProtoKeybindingOverrideSet,
  surface: CommandSurface,
): KeybindingOverrideSet {
  return surface === CommandSurface.WEB
    ? readOverridePartition(
        value.webOverrides ?? [],
        value.webSettings,
        CommandSurface.WEB,
      )
    : readOverridePartition(
        value.tuiOverrides ?? [],
        value.tuiSettings,
        CommandSurface.TUI,
      )
}

function readOverridePartition(
  rawOverrides: readonly ProtoKeybindingCommandOverride[],
  settings: ProtoKeybindingOverrideSet['settings'],
  surface: CommandSurface,
  legacy = false,
): KeybindingOverrideSet {
  const overrides: Record<string, KeybindingCommandOverride> = {}
  for (const rawOverride of rawOverrides) {
    const commandId = rawOverride.commandId
    if (!commandId) continue
    const override = normalizeCommandOverride(rawOverride)
    if (override.bindings) {
      override.bindings = override.bindings.map((binding) => {
        if (legacy) return { ...binding, surface: CommandSurface.WEB }
        if (binding.surface !== surface)
          throw new Error(
            `binding surface must match ${surface === CommandSurface.WEB ? 'WEB' : 'TUI'}`,
          )
        return binding
      })
    }
    if (!isKeybindingCommandOverrideEmpty(override))
      overrides[commandId] = override
  }
  return {
    version: 2,
    overrides,
    settings: normalizeKeybindingSettings(settings),
  }
}

export function keybindingOverrideSetToProto(
  value: KeybindingOverrideSet,
  surface: CommandSurface = CommandSurface.WEB,
): ProtoKeybindingOverrideSet {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI) {
    throw new Error(
      'keybinding override serialization requires WEB or TUI surface',
    )
  }
  const normalized = normalizeKeybindingOverrideSet(value)
  const overrides = Object.entries(normalized.overrides).map(
    ([commandId, override]) =>
      keybindingCommandOverrideToProto(commandId, override),
  )
  for (const override of overrides) {
    for (const binding of override.bindings ?? []) {
      if (binding.surface !== surface)
        throw new Error('binding surface must match selected command surface')
    }
  }
  return {
    version: 2,
    overrides: [],
    settings: undefined,
    webOverrides: surface === CommandSurface.WEB ? overrides : [],
    tuiOverrides: surface === CommandSurface.TUI ? overrides : [],
    webSettings:
      surface === CommandSurface.WEB
        ? keybindingOverrideSettingsToProto(normalized.settings)
        : undefined,
    tuiSettings:
      surface === CommandSurface.TUI
        ? keybindingOverrideSettingsToProto(normalized.settings)
        : undefined,
  }
}

export function mergeKeybindingOverridePartitions(
  current: ProtoKeybindingOverrideSet | null | undefined,
  next: KeybindingOverrideSet,
  surface: CommandSurface,
): ProtoKeybindingOverrideSet {
  const selected = keybindingOverrideSetToProto(next, surface)
  return {
    version: 2,
    overrides: [],
    settings: undefined,
    webOverrides:
      surface === CommandSurface.WEB
        ? (selected.webOverrides ?? [])
        : (current?.webOverrides ?? []),
    tuiOverrides:
      surface === CommandSurface.TUI
        ? (selected.tuiOverrides ?? [])
        : (current?.tuiOverrides ?? []),
    webSettings:
      surface === CommandSurface.WEB
        ? selected.webSettings
        : current?.webSettings,
    tuiSettings:
      surface === CommandSurface.TUI
        ? selected.tuiSettings
        : current?.tuiSettings,
  }
}

export function keybindingCommandOverrideToProto(
  commandId: string,
  override: KeybindingCommandOverride,
): ProtoKeybindingCommandOverride {
  const normalized = normalizeCommandOverride(override)
  return {
    commandId,
    replaceBindings: normalized.replaceBindings,
    disabled: normalized.disabled,
    clearedBindingIds: normalized.clearedBindingIds ?? [],
    bindings: normalized.bindings ?? [],
  }
}

export function normalizeKeybindingOverrideSet(
  value: unknown,
): KeybindingOverrideSet {
  if (!isRecord(value)) return createEmptyKeybindingOverrideSet()
  const rawOverrides = isRecord(value.overrides) ? value.overrides : {}
  const overrides: Record<string, KeybindingCommandOverride> = {}

  for (const [commandId, rawOverride] of Object.entries(rawOverrides)) {
    if (!commandId || !isRecord(rawOverride)) continue
    const override = normalizeCommandOverride(rawOverride)
    if (!isKeybindingCommandOverrideEmpty(override)) {
      overrides[commandId] = override
    }
  }

  return {
    version: 2,
    overrides,
    settings: normalizeKeybindingSettings(value.settings),
  }
}

export function setKeybindingOverrideSettings(
  overrideSet: KeybindingOverrideSet,
  settings: KeybindingOverrideSettings,
): KeybindingOverrideSet {
  const normalized = normalizeKeybindingOverrideSet(overrideSet)
  return {
    ...normalized,
    settings: normalizeKeybindingSettings(settings),
  }
}

export function resolveKeybindingSettings(
  overrideLayers: readonly KeybindingOverrideLayer[] = [],
  defaults: EffectiveKeybindingSettings = defaultKeybindingSettings,
): EffectiveKeybindingSettings {
  let settings: EffectiveKeybindingSettings = { ...defaults }
  for (const layer of overrideLayers) {
    const layerSettings = layer.overrideSet.settings
    settings = {
      leaderCombo: layerSettings.leaderCombo ?? settings.leaderCombo,
      whichKeyDelayMs:
        layerSettings.whichKeyDelayMs ?? settings.whichKeyDelayMs,
      display: layerSettings.display ?? settings.display,
    }
  }
  return settings
}

export function setKeybindingCommandOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
  override: KeybindingCommandOverride | null,
): KeybindingOverrideSet {
  const normalized = normalizeKeybindingOverrideSet(overrideSet)
  const overrides = { ...normalized.overrides }
  if (!commandId) return normalized

  const nextOverride = override ? normalizeCommandOverride(override) : null
  if (!nextOverride || isKeybindingCommandOverrideEmpty(nextOverride)) {
    delete overrides[commandId]
  } else {
    overrides[commandId] = nextOverride
  }

  return { ...normalized, overrides }
}

export function resetKeybindingCommandOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
): KeybindingOverrideSet {
  return setKeybindingCommandOverride(overrideSet, commandId, null)
}

export function clearKeybindingOverrideSet(): KeybindingOverrideSet {
  return createEmptyKeybindingOverrideSet()
}

export function setCommandBindingsOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
  bindings: CommandBinding[],
): KeybindingOverrideSet {
  return setKeybindingCommandOverride(overrideSet, commandId, {
    replaceBindings: true,
    bindings,
  })
}

export function addCommandBindingOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
  binding: CommandBinding,
): KeybindingOverrideSet {
  const normalized = normalizeKeybindingOverrideSet(overrideSet)
  const current = normalized.overrides[commandId] ?? {}
  return setKeybindingCommandOverride(normalized, commandId, {
    ...current,
    bindings: [...(current.bindings ?? []), binding],
  })
}

export function clearCommandBindingsOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
): KeybindingOverrideSet {
  return setCommandBindingsOverride(overrideSet, commandId, [])
}

export function clearCommandBindingIdOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
  bindingId: string,
): KeybindingOverrideSet {
  const normalized = normalizeKeybindingOverrideSet(overrideSet)
  const current = normalized.overrides[commandId] ?? {}
  return setKeybindingCommandOverride(normalized, commandId, {
    ...current,
    clearedBindingIds: [...(current.clearedBindingIds ?? []), bindingId],
  })
}

export function removeLocalCommandBindingOverride(
  overrideSet: KeybindingOverrideSet,
  commandId: string,
  bindingId: string,
): KeybindingOverrideSet {
  const normalized = normalizeKeybindingOverrideSet(overrideSet)
  const current = normalized.overrides[commandId]
  if (!current) return normalized
  return setKeybindingCommandOverride(normalized, commandId, {
    ...current,
    bindings: (current.bindings ?? []).filter((binding) => {
      const id = binding.id || keybindingBindingStorageId(binding)
      return id !== bindingId
    }),
  })
}

export function keybindingBindingStorageId(binding: CommandBinding): string {
  if (binding.id) return binding.id
  if (binding.binding?.case === 'sequence') {
    return `sequence:${(binding.binding.value.steps ?? []).join(' ')}`
  }
  if (binding.binding?.case === 'combo') {
    return `combo:${binding.binding.value.combo}`
  }
  return 'binding'
}

export function isKeybindingCommandOverrideEmpty(
  override: KeybindingCommandOverride,
): boolean {
  return (
    !override.replaceBindings &&
    !override.disabled &&
    !override.clearedBindingIds?.length &&
    !override.bindings?.length
  )
}

function normalizeCommandOverride(value: {
  replaceBindings?: unknown
  disabled?: unknown
  clearedBindingIds?: unknown
  bindings?: unknown
}): KeybindingCommandOverride {
  const override: KeybindingCommandOverride = {}
  if (value.replaceBindings === true) override.replaceBindings = true
  if (value.disabled === true) override.disabled = true
  if (Array.isArray(value.clearedBindingIds)) {
    override.clearedBindingIds = uniqueStrings(value.clearedBindingIds)
  }
  if (Array.isArray(value.bindings)) {
    override.bindings = value.bindings.filter(isCommandBinding)
  }
  return override
}

function normalizeKeybindingSettings(
  value: unknown,
): KeybindingOverrideSettings {
  if (!isRecord(value)) return {}
  const settings: KeybindingOverrideSettings = {}
  if (typeof value.leaderCombo === 'string')
    settings.leaderCombo = value.leaderCombo
  if (typeof value.whichKeyDelayMs === 'number') {
    settings.whichKeyDelayMs = value.whichKeyDelayMs
  }
  if (isRecord(value.display)) {
    const mode = value.display.mode
    if (mode === 'symbols' || mode === 'text') settings.display = { mode }
    const protoMode =
      typeof mode === 'number' ? displayModeFromProto(mode) : undefined
    if (protoMode) settings.display = { mode: protoMode }
  }
  return settings
}

export function keybindingOverrideSettingsToProto(
  settings: KeybindingOverrideSettings,
): NonNullable<ProtoKeybindingOverrideSet['settings']> {
  const mode = settings.display?.mode
  return {
    leaderCombo: settings.leaderCombo,
    whichKeyDelayMs: settings.whichKeyDelayMs,
    display: mode ? { mode: displayModeToProto(mode) } : undefined,
  }
}

function displayModeToProto(mode: 'symbols' | 'text'): KeybindingDisplayMode {
  return mode === 'symbols'
    ? KeybindingDisplayMode.SYMBOLS
    : KeybindingDisplayMode.TEXT
}

function displayModeFromProto(
  mode: KeybindingDisplayMode | undefined,
): KeybindingDisplaySettings['mode'] | undefined {
  if (mode === KeybindingDisplayMode.SYMBOLS) return 'symbols'
  if (mode === KeybindingDisplayMode.TEXT) return 'text'
  return undefined
}

function uniqueStrings(values: unknown[]): string[] {
  return [...new Set(values.filter((value): value is string => Boolean(value)))]
}

function isCommandBinding(value: unknown): value is CommandBinding {
  if (!isRecord(value)) return false
  const binding = value.binding
  if (!isRecord(binding)) return false
  if (binding.case === 'combo') return isRecord(binding.value)
  if (binding.case === 'sequence') return isRecord(binding.value)
  return false
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object')
}
