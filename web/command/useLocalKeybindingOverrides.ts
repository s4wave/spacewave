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
import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'

export interface LocalKeybindingOverridesValue {
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

export function useLocalKeybindingOverrides(): LocalKeybindingOverridesValue {
  const namespace = useStateNamespace([...localKeybindingStoreNamespace])
  const [rawOverrideSet, setRawOverrideSet] = useStateAtom(
    namespace,
    localKeybindingStoreKey,
    createEmptyKeybindingOverrideSet(),
  )
  const overrideSet = useMemo(
    () => normalizeKeybindingOverrideSet(rawOverrideSet),
    [rawOverrideSet],
  )
  const layer = useMemo(
    () => createKeybindingOverrideLayer('local', 'Local', overrideSet),
    [overrideSet],
  )

  const setSettings = useCallback(
    (settings: KeybindingOverrideSettings) => {
      setRawOverrideSet((current) =>
        setKeybindingOverrideSettings(current, settings),
      )
    },
    [setRawOverrideSet],
  )

  const setOverrideSet = useCallback(
    (next: KeybindingOverrideSet) => {
      setRawOverrideSet(normalizeKeybindingOverrideSet(next))
    },
    [setRawOverrideSet],
  )

  const setCommandOverride = useCallback(
    (commandId: string, override: KeybindingCommandOverride | null) => {
      setRawOverrideSet((current) =>
        setKeybindingCommandOverride(current, commandId, override),
      )
    },
    [setRawOverrideSet],
  )

  const setCommandBindings = useCallback(
    (commandId: string, bindings: CommandBinding[]) => {
      setRawOverrideSet((current) =>
        setCommandBindingsOverride(current, commandId, bindings),
      )
    },
    [setRawOverrideSet],
  )

  const addCommandBinding = useCallback(
    (commandId: string, binding: CommandBinding) => {
      setRawOverrideSet((current) =>
        addCommandBindingOverride(current, commandId, binding),
      )
    },
    [setRawOverrideSet],
  )

  const clearCommandBindings = useCallback(
    (commandId: string) => {
      setRawOverrideSet((current) =>
        clearCommandBindingsOverride(current, commandId),
      )
    },
    [setRawOverrideSet],
  )

  const clearCommandBindingId = useCallback(
    (commandId: string, bindingId: string) => {
      setRawOverrideSet((current) =>
        clearCommandBindingIdOverride(current, commandId, bindingId),
      )
    },
    [setRawOverrideSet],
  )

  const removeLocalCommandBinding = useCallback(
    (commandId: string, bindingId: string) => {
      setRawOverrideSet((current) =>
        removeLocalCommandBindingOverride(current, commandId, bindingId),
      )
    },
    [setRawOverrideSet],
  )

  const resetCommand = useCallback(
    (commandId: string) => {
      setRawOverrideSet((current) =>
        resetKeybindingCommandOverride(current, commandId),
      )
    },
    [setRawOverrideSet],
  )

  const resetLayer = useCallback(() => {
    setRawOverrideSet(clearKeybindingOverrideSet())
  }, [setRawOverrideSet])

  return {
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
