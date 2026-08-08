import { useCallback, useMemo, useState } from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import {
  CommandSurface,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'

import {
  addCommandBindingOverride,
  clearCommandBindingIdOverride,
  clearCommandBindingsOverride,
  createEmptyKeybindingOverrideSet,
  createKeybindingOverrideLayer,
  keybindingOverrideSetFromProto,
  normalizeKeybindingOverrideSet,
  removeLocalCommandBindingOverride,
  resetKeybindingCommandOverride,
  setCommandBindingsOverride,
  setKeybindingOverrideSettings,
  setKeybindingCommandOverride,
  type KeybindingCommandOverride,
  type KeybindingOverrideLayer,
  type KeybindingOverrideSet,
  type KeybindingOverrideSettings,
  mergeKeybindingOverridePartitions,
} from './keybinding-overrides.js'

export interface AccountKeybindingOverridesValue {
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

export function useAccountKeybindingOverrides(
  surface: CommandSurface,
): AccountKeybindingOverridesValue {
  if (surface !== CommandSurface.WEB && surface !== CommandSurface.TUI)
    throw new Error('account keybindings require WEB or TUI surface')
  const [writeFailure, setWriteFailure] = useState<{
    error: Error
    expected: unknown
  } | null>(null)
  const sessionResource = SessionContext.useContext()
  const sessionInfo = useSessionInfo(sessionResource.value)
  const accountResource = useMountAccount(
    sessionInfo.providerId,
    sessionInfo.accountId,
    Boolean(sessionInfo.providerId && sessionInfo.accountId),
  )
  const accountOverrides = useStreamingResource(
    accountResource,
    (account, signal) => account.watchKeybindingOverrides({}, signal),
    [],
  )
  const available = Boolean(accountResource.value && accountOverrides.value)
  const readOnly = !available || Boolean(accountOverrides.value?.readOnly)
  const overrideSet = useMemo(
    () =>
      keybindingOverrideSetFromProto(
        accountOverrides.value?.overrideSet,
        surface,
      ),
    [accountOverrides.value?.overrideSet, surface],
  )
  const layer = useMemo(
    () =>
      available
        ? createKeybindingOverrideLayer('account', 'Account', overrideSet)
        : null,
    [available, overrideSet],
  )

  const setOverrideSet = useCallback(
    (next: KeybindingOverrideSet) => {
      const account = accountResource.value
      if (!account || readOnly) return
      const normalized = normalizeKeybindingOverrideSet(next)
      const combined = mergeKeybindingOverridePartitions(
        accountOverrides.value?.overrideSet,
        normalized,
        surface,
      )
      const expectedOverrideSet = accountOverrides.value?.overrideSet
      if (!expectedOverrideSet) return
      void account
        .replaceKeybindingOverrideSet({
          expectedOverrideSet,
          overrideSet: combined,
        })
        .then(() => setWriteFailure(null))
        .catch((error: unknown) =>
          setWriteFailure({
            error: error instanceof Error ? error : new Error(String(error)),
            expected: expectedOverrideSet,
          }),
        )
    },
    [
      accountResource.value,
      accountOverrides.value?.overrideSet,
      readOnly,
      surface,
    ],
  )

  const applyOverride = useCallback(
    (commandId: string, override: KeybindingCommandOverride | null) => {
      const next = setKeybindingCommandOverride(
        normalizeKeybindingOverrideSet(overrideSet),
        commandId,
        override,
      )
      setOverrideSet(next)
    },
    [overrideSet, setOverrideSet],
  )

  const setSettings = useCallback(
    (settings: KeybindingOverrideSettings) => {
      setOverrideSet(setKeybindingOverrideSettings(overrideSet, settings))
    },
    [overrideSet, setOverrideSet],
  )

  const setCommandBindings = useCallback(
    (commandId: string, bindings: CommandBinding[]) => {
      const next = setCommandBindingsOverride(overrideSet, commandId, bindings)
      applyOverride(commandId, next.overrides[commandId] ?? null)
    },
    [applyOverride, overrideSet],
  )

  const addCommandBinding = useCallback(
    (commandId: string, binding: CommandBinding) => {
      const next = addCommandBindingOverride(overrideSet, commandId, binding)
      applyOverride(commandId, next.overrides[commandId] ?? null)
    },
    [applyOverride, overrideSet],
  )

  const clearCommandBindings = useCallback(
    (commandId: string) => {
      const next = clearCommandBindingsOverride(overrideSet, commandId)
      applyOverride(commandId, next.overrides[commandId] ?? null)
    },
    [applyOverride, overrideSet],
  )

  const clearCommandBindingId = useCallback(
    (commandId: string, bindingId: string) => {
      const next = clearCommandBindingIdOverride(
        overrideSet,
        commandId,
        bindingId,
      )
      applyOverride(commandId, next.overrides[commandId] ?? null)
    },
    [applyOverride, overrideSet],
  )

  const removeCommandBinding = useCallback(
    (commandId: string, bindingId: string) => {
      const next = removeLocalCommandBindingOverride(
        overrideSet,
        commandId,
        bindingId,
      )
      applyOverride(commandId, next.overrides[commandId] ?? null)
    },
    [applyOverride, overrideSet],
  )

  const resetCommand = useCallback(
    (commandId: string) => {
      const next = resetKeybindingCommandOverride(overrideSet, commandId)
      applyOverride(commandId, next.overrides[commandId] ?? null)
    },
    [applyOverride, overrideSet],
  )

  const resetLayer = useCallback(() => {
    setOverrideSet(createEmptyKeybindingOverrideSet())
  }, [setOverrideSet])

  const activeWriteError =
    writeFailure !== null &&
    writeFailure.expected === accountOverrides.value?.overrideSet
      ? writeFailure.error
      : null

  return {
    overrideSet,
    layer,
    available,
    readOnly,
    loading:
      sessionResource.loading ||
      accountResource.loading ||
      accountOverrides.loading,
    error:
      sessionResource.error ??
      accountResource.error ??
      accountOverrides.error ??
      activeWriteError,
    setCommandOverride: applyOverride,
    setOverrideSet,
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
