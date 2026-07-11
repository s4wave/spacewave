import { useCallback, useMemo } from 'react'

import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'
import { applySpaceKeybindingOverrides } from '@s4wave/app/space/space-settings.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import {
  addCommandBindingOverride,
  clearCommandBindingIdOverride,
  clearCommandBindingsOverride,
  clearKeybindingOverrideSet,
  createEmptyKeybindingOverrideSet,
  createKeybindingOverrideLayer,
  keybindingOverrideSetFromProto,
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

export interface SpaceKeybindingOverridesValue {
  overrideSet: KeybindingOverrideSet
  layer: KeybindingOverrideLayer | null
  available: boolean
  readOnly: boolean
  loading: boolean
  error: Error | null
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
  removeCommandBinding: (commandId: string, bindingId: string) => void
  resetCommand: (commandId: string) => void
  resetLayer: () => void
}

export function useSpaceKeybindingOverrides(): SpaceKeybindingOverridesValue {
  const context = SpaceContainerContext.useContextSafe()
  const overrideSet = useMemo(
    () =>
      keybindingOverrideSetFromProto(
        context?.spaceState.settings?.keybindingOverrides,
      ),
    [context?.spaceState.settings?.keybindingOverrides],
  )
  const available = Boolean(context?.spaceWorld)
  const readOnly =
    !available ||
    (context?.spaceSharingState ? !context.spaceSharingState.canManage : false)
  const layer = useMemo(
    () =>
      available
        ? createKeybindingOverrideLayer('space', 'Space', overrideSet)
        : null,
    [available, overrideSet],
  )

  const applyOverrideSet = useCallback(
    (next: KeybindingOverrideSet) => {
      if (!context || readOnly) return
      void applySpaceKeybindingOverrides(
        context.spaceWorld,
        context.spaceState.settings,
        next,
      )
    },
    [context, readOnly],
  )

  const setSettings = useCallback(
    (settings: KeybindingOverrideSettings) => {
      applyOverrideSet(setKeybindingOverrideSettings(overrideSet, settings))
    },
    [applyOverrideSet, overrideSet],
  )

  const setCommandOverride = useCallback(
    (commandId: string, override: KeybindingCommandOverride | null) => {
      applyOverrideSet(
        setKeybindingCommandOverride(overrideSet, commandId, override),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const setCommandBindings = useCallback(
    (commandId: string, bindings: CommandBinding[]) => {
      applyOverrideSet(
        setCommandBindingsOverride(overrideSet, commandId, bindings),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const addCommandBinding = useCallback(
    (commandId: string, binding: CommandBinding) => {
      applyOverrideSet(
        addCommandBindingOverride(overrideSet, commandId, binding),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const clearCommandBindings = useCallback(
    (commandId: string) => {
      applyOverrideSet(clearCommandBindingsOverride(overrideSet, commandId))
    },
    [applyOverrideSet, overrideSet],
  )

  const clearCommandBindingId = useCallback(
    (commandId: string, bindingId: string) => {
      applyOverrideSet(
        clearCommandBindingIdOverride(overrideSet, commandId, bindingId),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const removeCommandBinding = useCallback(
    (commandId: string, bindingId: string) => {
      applyOverrideSet(
        removeLocalCommandBindingOverride(overrideSet, commandId, bindingId),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const resetCommand = useCallback(
    (commandId: string) => {
      applyOverrideSet(resetKeybindingCommandOverride(overrideSet, commandId))
    },
    [applyOverrideSet, overrideSet],
  )

  const resetLayer = useCallback(() => {
    applyOverrideSet(clearKeybindingOverrideSet())
  }, [applyOverrideSet])

  return {
    overrideSet: available ? overrideSet : createEmptyKeybindingOverrideSet(),
    layer,
    available,
    readOnly,
    loading: context?.spaceWorldResource.loading ?? false,
    error: context?.spaceWorldResource.error ?? null,
    setCommandOverride,
    setOverrideSet: applyOverrideSet,
    setSettings,
    setCommandBindings,
    addCommandBinding,
    clearCommandBindings,
    clearCommandBindingId,
    removeCommandBinding,
    resetCommand,
    resetLayer,
  }
}
