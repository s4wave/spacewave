import { useCallback, useMemo, useState } from 'react'
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
  removeLocalCommandBindingOverride,
  resetKeybindingCommandOverride,
  setCommandBindingsOverride,
  setKeybindingCommandOverride,
  setKeybindingOverrideSettings,
  type KeybindingCommandOverride,
  type KeybindingOverrideLayer,
  type KeybindingOverrideSet,
  type KeybindingOverrideSettings,
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
): SpaceKeybindingOverridesValue {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI)
    throw new Error('Space keybindings require WEB or TUI surface')
  const [writeFailure, setWriteFailure] = useState<{
    error: Error
    expected: unknown
    world: unknown
  } | null>(null)
  const context = SpaceContainerContext.useContextSafe()
  const rawOverrideSet = context?.spaceState.settings?.keybindingOverrides
  const overrideSet = useMemo(
    () => keybindingOverrideSetFromProto(rawOverrideSet, surface),
    [rawOverrideSet, surface],
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
    (
      next: KeybindingOverrideSet,
      abortSignal?: AbortSignal,
      writeSurface: CommandSurface = surface,
    ) => {
      if (!context || readOnly) return Promise.resolve()
      const expectedOverrideSet =
        context.spaceState.settings?.keybindingOverrides ?? {}
      const combined = mergeKeybindingOverridePartitions(
        expectedOverrideSet,
        next,
        writeSurface,
      )
      return applySpaceKeybindingOverrides(
        context.spaceWorld,
        context.spaceState.settings,
        combined,
        expectedOverrideSet,
        '',
        abortSignal,
      )
        .then(() => setWriteFailure(null))
        .catch((error: unknown) => {
          if (!abortSignal?.aborted)
            setWriteFailure({
              error: error instanceof Error ? error : new Error(String(error)),
              expected: rawOverrideSet,
              world: context.spaceWorld,
            })
        })
    },
    [context, rawOverrideSet, readOnly, surface],
  )
  const setSettings = useCallback(
    (settings: KeybindingOverrideSettings) => {
      void applyOverrideSet(
        setKeybindingOverrideSettings(overrideSet, settings),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const setCommandOverride = useCallback(
    (commandId: string, override: KeybindingCommandOverride | null) => {
      void applyOverrideSet(
        setKeybindingCommandOverride(overrideSet, commandId, override),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const setCommandBindings = useCallback(
    (commandId: string, bindings: CommandBinding[]) => {
      void applyOverrideSet(
        setCommandBindingsOverride(overrideSet, commandId, bindings),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const addCommandBinding = useCallback(
    (commandId: string, binding: CommandBinding) => {
      void applyOverrideSet(
        addCommandBindingOverride(overrideSet, commandId, binding),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const clearCommandBindings = useCallback(
    (commandId: string) => {
      void applyOverrideSet(
        clearCommandBindingsOverride(overrideSet, commandId),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const clearCommandBindingId = useCallback(
    (commandId: string, bindingId: string) => {
      void applyOverrideSet(
        clearCommandBindingIdOverride(overrideSet, commandId, bindingId),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const removeCommandBinding = useCallback(
    (commandId: string, bindingId: string) => {
      void applyOverrideSet(
        removeLocalCommandBindingOverride(overrideSet, commandId, bindingId),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const resetCommand = useCallback(
    (commandId: string) => {
      void applyOverrideSet(
        resetKeybindingCommandOverride(overrideSet, commandId),
      )
    },
    [applyOverrideSet, overrideSet],
  )

  const resetLayer = useCallback(() => {
    void applyOverrideSet(clearKeybindingOverrideSet())
  }, [applyOverrideSet])

  const activeWriteError =
    writeFailure !== null &&
    writeFailure.world === context?.spaceWorld &&
    writeFailure.expected === rawOverrideSet
      ? writeFailure.error
      : null

  return {
    overrideSet: available ? overrideSet : createEmptyKeybindingOverrideSet(),
    layer,
    available,
    readOnly,
    loading: context?.spaceWorldResource.loading ?? false,
    error: context?.spaceWorldResource.error ?? activeWriteError,
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
