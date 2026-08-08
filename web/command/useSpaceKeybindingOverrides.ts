import { useCallback, useMemo } from 'react'
import { useAbortSignalEffect } from '@aptre/bldr-react'

import {
  CommandSurface,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
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
  mergeKeybindingOverridePartitions,
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

export function useSpaceKeybindingOverrides(
  surface: CommandSurface,
  canonicalCommandIds: ReadonlySet<string>,
): SpaceKeybindingOverridesValue {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI)
    throw new Error('Space keybindings require WEB or TUI surface')
  const context = SpaceContainerContext.useContextSafe()
  const rawOverrideSet = context?.spaceState.settings?.keybindingOverrides
  const overrideSet = useMemo(() => {
    if (!rawOverrideSet) return createEmptyKeybindingOverrideSet()
    if (rawOverrideSet.version === 1)
      return surface === CommandSurface.WEB
        ? migrateLegacyKeybindingOverrideSet(
            rawOverrideSet,
            canonicalCommandIds,
          ).overrideSet
        : createEmptyKeybindingOverrideSet()
    return keybindingOverrideSetFromProto(rawOverrideSet, surface)
  }, [rawOverrideSet, canonicalCommandIds, surface])
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
    (
      next: KeybindingOverrideSet,
      abortSignal?: AbortSignal,
      writeSurface: CommandSurface = surface,
    ) => {
      if (!context || readOnly) return Promise.resolve()
      const combined = mergeKeybindingOverridePartitions(
        context.spaceState.settings?.keybindingOverrides,
        next,
        writeSurface,
      )
      return applySpaceKeybindingOverrides(
        context.spaceWorld,
        context.spaceState.settings,
        combined,
        '',
        abortSignal,
      )
    },
    [context, readOnly, surface],
  )
  const migration = useMemo(
    () =>
      rawOverrideSet
        ? migrateLegacyKeybindingOverrideSet(
            rawOverrideSet,
            canonicalCommandIds,
          )
        : {
            overrideSet: createEmptyKeybindingOverrideSet(),
            required: false,
            diagnostics: [],
          },
    [rawOverrideSet, canonicalCommandIds],
  )
  useAbortSignalEffect(
    (signal) => {
      if (
        !context ||
        readOnly ||
        !migration.required ||
        migration.diagnostics.length
      )
        return
      void applyOverrideSet(migration.overrideSet, signal, CommandSurface.WEB)
    },
    [context, readOnly, migration, applyOverrideSet],
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
    error:
      context?.spaceWorldResource.error ??
      keybindingMigrationError(migration.diagnostics),
    setCommandOverride,
    setOverrideSet: (next) => {
      void applyOverrideSet(next)
    },
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
