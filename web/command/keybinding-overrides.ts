import {
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

export interface KeybindingCommandOverride {
  replaceBindings?: boolean
  disabled?: boolean
  clearedBindingIds?: string[]
  bindings?: CommandBinding[]
}

export interface KeybindingOverrideSet {
  version: 1
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
  return { version: 1, overrides: {}, settings: {} }
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

export function keybindingOverrideSetFromProto(
  value: ProtoKeybindingOverrideSet | null | undefined,
): KeybindingOverrideSet {
  if (!value) return createEmptyKeybindingOverrideSet()
  const overrides: Record<string, KeybindingCommandOverride> = {}
  for (const rawOverride of value.overrides ?? []) {
    const commandId = rawOverride.commandId
    if (!commandId) continue
    const override = normalizeCommandOverride(rawOverride)
    if (!isKeybindingCommandOverrideEmpty(override)) {
      overrides[commandId] = override
    }
  }
  return {
    version: 1,
    overrides,
    settings: normalizeKeybindingSettings(value.settings),
  }
}

export function keybindingOverrideSetToProto(
  value: KeybindingOverrideSet,
): ProtoKeybindingOverrideSet {
  const normalized = normalizeKeybindingOverrideSet(value)
  return {
    version: normalized.version,
    overrides: Object.entries(normalized.overrides).map(
      ([commandId, override]) =>
        keybindingCommandOverrideToProto(commandId, override),
    ),
    settings: keybindingSettingsToProto(normalized.settings),
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
    version: 1,
    overrides,
    settings: normalizeKeybindingSettings(value.settings),
  }
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

function keybindingSettingsToProto(
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
