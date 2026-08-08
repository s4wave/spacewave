import { useCallback, useMemo } from 'react'
import { useAbortSignalEffect } from '@aptre/bldr-react'

import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'

import {
  addCommandBindingOverride,
  clearCommandBindingIdOverride,
  clearCommandBindingsOverride,
  clearKeybindingOverrideSet,
  createEmptyKeybindingOverrideSet,
  createKeybindingOverrideLayer,
  localKeybindingStoreKey,
  localKeybindingStoreNamespace,
  normalizeKeybindingOverrideSet,
  migrateLegacyKeybindingOverrideSet,
  removeLocalCommandBindingOverride,
  resetKeybindingCommandOverride,
  setCommandBindingsOverride,
  setKeybindingCommandOverride,
  setKeybindingOverrideSettings,
  type KeybindingCommandOverride,
  type KeybindingOverrideLayer,
  type KeybindingOverrideSet,
  type KeybindingOverrideSettings,
  keybindingMigrationError,
} from './keybinding-overrides.js'
import {
  CommandSurface,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'

export interface LocalKeybindingOverridesValue {
  error: Error | null
  overrideSet: KeybindingOverrideSet
  layer: KeybindingOverrideLayer
  setCommandOverride: (
    commandId: string,
    override: KeybindingCommandOverride | null,
  ) => void
  setSettings: (settings: KeybindingOverrideSettings) => void
  setOverrideSet: (
    overrideSet: KeybindingOverrideSet,
    changedCommandIds: string[],
  ) => void
  setCommandBindings: (commandId: string, bindings: CommandBinding[]) => void
  addCommandBinding: (commandId: string, binding: CommandBinding) => void
  clearCommandBindings: (commandId: string) => void
  clearCommandBindingId: (commandId: string, bindingId: string) => void
  removeLocalCommandBinding: (commandId: string, bindingId: string) => void
  resetCommand: (commandId: string) => void
  resetLayer: () => void
}

interface LocalStoredKeybindings {
  version: 2
  webOverrides: KeybindingOverrideSet
  tuiOverrides: KeybindingOverrideSet
}

function emptyLocalStoredKeybindings(): LocalStoredKeybindings {
  return {
    version: 2,
    webOverrides: createEmptyKeybindingOverrideSet(),
    tuiOverrides: createEmptyKeybindingOverrideSet(),
  }
}

export function useLocalKeybindingOverrides(
  surface: CommandSurface,
  canonicalCommandIds: ReadonlySet<string>,
): LocalKeybindingOverridesValue {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI)
    throw new Error('local keybindings require WEB or TUI surface')
  const namespace = useStateNamespace([...localKeybindingStoreNamespace])
  const [rawOverrideSet, setRawOverrideSet] = useStateAtom(
    namespace,
    localKeybindingStoreKey,
    emptyLocalStoredKeybindings() as unknown,
  )
  const migration = useMemo(() => {
    const raw = rawOverrideSet as Record<string, unknown>
    if (raw.version === 2 && 'webOverrides' in raw) {
      return {
        overrideSet: createEmptyKeybindingOverrideSet(),
        required: false,
        diagnostics: [],
      }
    }
    const legacy = normalizeKeybindingOverrideSet(rawOverrideSet)
    return migrateLegacyKeybindingOverrideSet(
      {
        version: 1,
        overrides: Object.entries(legacy.overrides).map(
          ([commandId, override]) => ({ commandId, ...override }),
        ),
        settings: (raw.settings && typeof raw.settings === 'object'
          ? raw.settings
          : undefined) as never,
      },
      canonicalCommandIds,
    )
  }, [rawOverrideSet, canonicalCommandIds])
  useAbortSignalEffect(
    (signal) => {
      if (!migration.required || migration.diagnostics.length || signal.aborted)
        return
      setRawOverrideSet((current: unknown) => {
        const raw = current as Record<string, unknown>
        if (raw.version === 2 && 'webOverrides' in raw) return current
        const legacy = normalizeKeybindingOverrideSet(current)
        const currentMigration = migrateLegacyKeybindingOverrideSet(
          {
            version: 1,
            overrides: Object.entries(legacy.overrides).map(
              ([commandId, override]) => ({ commandId, ...override }),
            ),
            settings: (raw.settings && typeof raw.settings === 'object'
              ? raw.settings
              : undefined) as never,
          },
          canonicalCommandIds,
        )
        if (currentMigration.diagnostics.length) return current
        const stored = emptyLocalStoredKeybindings()
        stored.webOverrides = currentMigration.overrideSet
        return stored
      })
    },
    [migration, setRawOverrideSet, canonicalCommandIds],
  )
  const stored =
    (rawOverrideSet as LocalStoredKeybindings).version === 2 &&
    'webOverrides' in (rawOverrideSet as object)
      ? (rawOverrideSet as LocalStoredKeybindings)
      : emptyLocalStoredKeybindings()
  const overrideSet = normalizeKeybindingOverrideSet(
    surface === CommandSurface.WEB ? stored.webOverrides : stored.tuiOverrides,
  )
  const layer = useMemo(
    () => createKeybindingOverrideLayer('local', 'Local', overrideSet),
    [overrideSet],
  )
  const replaceSelected = useCallback(
    (next: KeybindingOverrideSet) => {
      setRawOverrideSet((current: unknown) => {
        const value =
          (current as LocalStoredKeybindings).version === 2 &&
          'webOverrides' in (current as object)
            ? (current as LocalStoredKeybindings)
            : emptyLocalStoredKeybindings()
        return surface === CommandSurface.WEB
          ? { ...value, webOverrides: normalizeKeybindingOverrideSet(next) }
          : { ...value, tuiOverrides: normalizeKeybindingOverrideSet(next) }
      })
    },
    [setRawOverrideSet, surface],
  )

  const setSettings = useCallback(
    (settings: KeybindingOverrideSettings) => {
      replaceSelected(setKeybindingOverrideSettings(overrideSet, settings))
    },
    [overrideSet, replaceSelected],
  )

  const setOverrideSet = useCallback(
    (next: KeybindingOverrideSet) => {
      replaceSelected(normalizeKeybindingOverrideSet(next))
    },
    [replaceSelected],
  )

  const setCommandOverride = useCallback(
    (commandId: string, override: KeybindingCommandOverride | null) => {
      replaceSelected(
        setKeybindingCommandOverride(overrideSet, commandId, override),
      )
    },
    [overrideSet, replaceSelected],
  )

  const setCommandBindings = useCallback(
    (commandId: string, bindings: CommandBinding[]) => {
      replaceSelected(
        setCommandBindingsOverride(overrideSet, commandId, bindings),
      )
    },
    [overrideSet, replaceSelected],
  )

  const addCommandBinding = useCallback(
    (commandId: string, binding: CommandBinding) => {
      replaceSelected(
        addCommandBindingOverride(overrideSet, commandId, binding),
      )
    },
    [overrideSet, replaceSelected],
  )

  const clearCommandBindings = useCallback(
    (commandId: string) => {
      replaceSelected(clearCommandBindingsOverride(overrideSet, commandId))
    },
    [overrideSet, replaceSelected],
  )

  const clearCommandBindingId = useCallback(
    (commandId: string, bindingId: string) => {
      replaceSelected(
        clearCommandBindingIdOverride(overrideSet, commandId, bindingId),
      )
    },
    [overrideSet, replaceSelected],
  )

  const removeLocalCommandBinding = useCallback(
    (commandId: string, bindingId: string) => {
      replaceSelected(
        removeLocalCommandBindingOverride(overrideSet, commandId, bindingId),
      )
    },
    [overrideSet, replaceSelected],
  )

  const resetCommand = useCallback(
    (commandId: string) => {
      replaceSelected(resetKeybindingCommandOverride(overrideSet, commandId))
    },
    [overrideSet, replaceSelected],
  )

  const resetLayer = useCallback(() => {
    replaceSelected(clearKeybindingOverrideSet())
  }, [replaceSelected])

  return {
    error: keybindingMigrationError(migration.diagnostics),
    overrideSet,
    layer,
    setCommandOverride,
    setOverrideSet,
    setSettings,
    setCommandBindings,
    addCommandBinding,
    clearCommandBindings,
    clearCommandBindingId,
    removeLocalCommandBinding,
    resetCommand,
    resetLayer,
  }
}
