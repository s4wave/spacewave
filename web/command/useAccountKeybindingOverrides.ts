import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'

import {
  addCommandBindingOverride,
  clearCommandBindingIdOverride,
  clearCommandBindingsOverride,
  createKeybindingOverrideLayer,
  keybindingCommandOverrideToProto,
  keybindingOverrideSettingsToProto,
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

export function useAccountKeybindingOverrides(): AccountKeybindingOverridesValue {
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
  const overrideSet = useMemo(
    () => keybindingOverrideSetFromProto(accountOverrides.value?.overrideSet),
    [accountOverrides.value?.overrideSet],
  )
  const available = Boolean(accountResource.value)
  const readOnly = !available || Boolean(accountOverrides.value?.readOnly)
  const layer = useMemo(
    () =>
      available
        ? createKeybindingOverrideLayer('account', 'Account', overrideSet)
        : null,
    [available, overrideSet],
  )

  const setOverrideSet = useCallback(
    (next: KeybindingOverrideSet, changedCommandIds: string[]) => {
      const account = accountResource.value
      if (!account || readOnly) return
      const normalized = normalizeKeybindingOverrideSet(next)
      for (const commandId of changedCommandIds) {
        const override = normalized.overrides[commandId]
        if (!override) {
          void account.removeKeybindingOverride({ commandId })
          continue
        }
        void account.upsertKeybindingOverride({
          override: keybindingCommandOverrideToProto(commandId, override),
        })
      }
    },
    [accountResource.value, readOnly],
  )

  const applyOverride = useCallback(
    (commandId: string, override: KeybindingCommandOverride | null) => {
      const next = setKeybindingCommandOverride(
        normalizeKeybindingOverrideSet(overrideSet),
        commandId,
        override,
      )
      setOverrideSet(next, [commandId])
    },
    [overrideSet, setOverrideSet],
  )

  const setSettings = useCallback(
    (settings: KeybindingOverrideSettings) => {
      const account = accountResource.value
      if (!account || readOnly) return
      const next = setKeybindingOverrideSettings(overrideSet, settings)
      void account.setKeybindingSettings({
        settings: keybindingOverrideSettingsToProto(next.settings),
      })
    },
    [accountResource.value, overrideSet, readOnly],
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
    const account = accountResource.value
    if (!account || readOnly) return
    for (const commandId of Object.keys(overrideSet.overrides)) {
      void account.removeKeybindingOverride({ commandId })
    }
    void account.setKeybindingSettings({
      settings: keybindingOverrideSettingsToProto({}),
    })
  }, [accountResource.value, overrideSet.overrides, readOnly])

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
      sessionResource.error ?? accountResource.error ?? accountOverrides.error,
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
