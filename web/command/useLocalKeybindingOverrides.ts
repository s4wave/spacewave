import { useCallback, useMemo } from 'react'
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
  removeLocalCommandBindingOverride,
  resetKeybindingCommandOverride,
  setCommandBindingsOverride,
  setKeybindingCommandOverride,
  setKeybindingOverrideSettings,
  type KeybindingCommandOverride,
  type KeybindingOverrideLayer,
  type KeybindingOverrideSet,
  type KeybindingOverrideSettings,
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
  webOverrides: KeybindingOverrideSet
  tuiOverrides: KeybindingOverrideSet
}

function emptyLocalStoredKeybindings(): LocalStoredKeybindings {
  return {
    webOverrides: createEmptyKeybindingOverrideSet(),
    tuiOverrides: createEmptyKeybindingOverrideSet(),
  }
}

export function useLocalKeybindingOverrides(
  surface: CommandSurface,
): LocalKeybindingOverridesValue {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI)
    throw new Error('local keybindings require WEB or TUI surface')
  const namespace = useStateNamespace([...localKeybindingStoreNamespace])
  const [rawOverrideSet, setRawOverrideSet] = useStateAtom(
    namespace,
    localKeybindingStoreKey,
    emptyLocalStoredKeybindings() as unknown,
  )
  const stored = rawOverrideSet as LocalStoredKeybindings
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
        const value = current as LocalStoredKeybindings
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
    error: null,
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
