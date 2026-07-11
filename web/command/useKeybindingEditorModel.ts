import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'

import { useCommands } from './CommandContext.js'
import type { ResolvedCommandBinding } from './KeybindingResolver.js'
import {
  buildCommandRows,
  canLayerOverrideBinding,
  layerStatusMessage,
  previewPendingConflict,
  replaceConflictingBindingOverride,
} from './keybinding-editor-helpers.js'
import { useKeybindingEditorLayers } from './useKeybindingEditorLayers.js'
import { useKeybindingRecorder } from './useKeybindingRecorder.js'
import type {
  CaptureState,
  KeybindingEditorContextValue,
  KeybindingEditorScope,
} from './component.js'

interface KeybindingEditorModelOptions {
  open: boolean
  initialScope: KeybindingEditorScope
  initialCommandId?: string
}

// useKeybindingEditorModel owns selection, recording, conflicts, and layer mutations.
export function useKeybindingEditorModel({
  open,
  initialScope,
  initialCommandId,
}: KeybindingEditorModelOptions): KeybindingEditorContextValue {
  const commands = useCommands()
  const {
    accountOverridesAvailable,
    spaceOverridesAvailable,
    controllers,
    overrideLayers,
    bindingGraph,
  } = useKeybindingEditorLayers(commands)
  const [query, setQuery] = useState('')
  const [selectedScope, setSelectedScope] =
    useState<KeybindingEditorScope>(initialScope)
  const [selectedCommandId, setSelectedCommandId] = useState<string | null>(
    initialCommandId ?? null,
  )
  const [capture, setCapture] = useState<CaptureState | null>(null)
  const [pendingBinding, setPendingBinding] = useState<CommandBinding | null>(
    null,
  )
  const [pendingReplace, setPendingReplace] = useState(true)
  const [captureError, setCaptureError] = useState<string | null>(null)
  const selectedController = controllers[selectedScope]
  const selectedLayerEditable =
    selectedController.available &&
    !selectedController.readOnly &&
    !selectedController.loading &&
    !selectedController.error
  const selectedSettingsEditable =
    selectedLayerEditable && selectedController.settingsEditable

  useEffect(() => {
    if (!open) return
    setSelectedScope(initialScope)
    setSelectedCommandId(initialCommandId ?? null)
    setPendingBinding(null)
    setCapture(null)
    setCaptureError(null)
  }, [open, initialScope, initialCommandId])

  const rows = useMemo(
    () => buildCommandRows(commands, bindingGraph, query),
    [commands, bindingGraph, query],
  )
  const selectedRow = useMemo(
    () => rows.find((row) => row.commandId === selectedCommandId) ?? rows[0],
    [rows, selectedCommandId],
  )
  const selectedBindings = selectedRow
    ? (bindingGraph.bindingsByCommandId.get(selectedRow.commandId) ?? [])
    : []
  const pendingConflict = useMemo(
    () =>
      selectedRow && pendingBinding && selectedLayerEditable
        ? previewPendingConflict(
            commands,
            overrideLayers,
            selectedController.scope,
            selectedController.label,
            selectedController.overrideSet,
            selectedRow.commandId,
            pendingBinding,
            pendingReplace,
          )
        : undefined,
    [
      commands,
      overrideLayers,
      pendingBinding,
      pendingReplace,
      selectedController.label,
      selectedController.overrideSet,
      selectedController.scope,
      selectedLayerEditable,
      selectedRow,
    ],
  )
  const commandConflicts = selectedRow
    ? bindingGraph.conflicts.filter((conflict) =>
        conflict.bindings.some(
          (binding) => binding.commandId === selectedRow.commandId,
        ),
      )
    : []
  const selectedLayerStatus = layerStatusMessage(selectedController)
  const startCapture = useCallback(
    (kind: 'combo' | 'sequence', replace: boolean) => {
      setCapture({
        kind,
        replace,
        steps: kind === 'sequence' ? ['Leader'] : [],
        heldModifiers: [],
      })
      setPendingBinding(null)
      setPendingReplace(replace)
      setCaptureError(null)
    },
    [],
  )
  const cancelCapture = useCallback(() => {
    setCapture(null)
    setPendingBinding(null)
    setCaptureError(null)
  }, [])

  useKeybindingRecorder({
    capture,
    selectedRow,
    selectedScope,
    setCapture,
    setPendingBinding,
    setPendingReplace,
    setCaptureError,
    cancelCapture,
  })

  const pendingConflictReplaceable = Boolean(
    pendingConflict?.bindings.every(
      (binding) =>
        binding.commandId === selectedRow?.commandId ||
        canLayerOverrideBinding(selectedScope, binding.sourceLayer),
    ),
  )
  const replacePendingConflict = useCallback(() => {
    if (
      !selectedLayerEditable ||
      !selectedRow ||
      !pendingBinding ||
      !pendingConflict ||
      !pendingConflictReplaceable
    ) {
      return
    }
    const changedCommandIds = [
      ...new Set([
        ...pendingConflict.bindings.flatMap((binding) =>
          binding.commandId === selectedRow.commandId
            ? []
            : [binding.commandId],
        ),
        selectedRow.commandId,
      ]),
    ]
    selectedController.setOverrideSet(
      replaceConflictingBindingOverride(
        selectedController.overrideSet,
        selectedController.scope,
        selectedRow.commandId,
        pendingBinding,
        pendingReplace,
        pendingConflict,
      ),
      changedCommandIds,
    )
    setPendingBinding(null)
    setCapture(null)
  }, [
    pendingBinding,
    pendingConflict,
    pendingConflictReplaceable,
    pendingReplace,
    selectedController,
    selectedLayerEditable,
    selectedRow,
  ])
  const savePendingBinding = useCallback(() => {
    if (
      !selectedLayerEditable ||
      !selectedRow ||
      !pendingBinding ||
      pendingConflict
    ) {
      return
    }
    if (pendingReplace) {
      selectedController.setCommandBindings(selectedRow.commandId, [
        pendingBinding,
      ])
    }
    if (!pendingReplace) {
      selectedController.addCommandBinding(
        selectedRow.commandId,
        pendingBinding,
      )
    }
    setPendingBinding(null)
    setCapture(null)
  }, [
    pendingBinding,
    pendingConflict,
    pendingReplace,
    selectedController,
    selectedLayerEditable,
    selectedRow,
  ])
  const clearSelectedBindings = useCallback(() => {
    if (!selectedLayerEditable || !selectedRow) return
    selectedController.clearCommandBindings(selectedRow.commandId)
    setPendingBinding(null)
  }, [selectedController, selectedLayerEditable, selectedRow])
  const resetSelectedCommand = useCallback(() => {
    if (!selectedLayerEditable || !selectedRow) return
    selectedController.resetCommand(selectedRow.commandId)
    setPendingBinding(null)
  }, [selectedController, selectedLayerEditable, selectedRow])
  const removeBinding = useCallback(
    (binding: ResolvedCommandBinding) => {
      if (
        !selectedLayerEditable ||
        !selectedRow ||
        !canLayerOverrideBinding(selectedScope, binding.sourceLayer)
      ) {
        return
      }
      if (binding.sourceLayer === selectedScope) {
        selectedController.removeCommandBinding(
          selectedRow.commandId,
          binding.bindingId,
        )
        return
      }
      selectedController.clearCommandBindingId(
        selectedRow.commandId,
        binding.bindingId,
      )
    },
    [selectedController, selectedLayerEditable, selectedRow, selectedScope],
  )
  const updateLeaderCombo = useCallback(
    (leaderCombo: string) => {
      if (!selectedSettingsEditable) return
      selectedController.setSettings({
        ...selectedController.overrideSet.settings,
        leaderCombo: leaderCombo || undefined,
      })
    },
    [selectedController, selectedSettingsEditable],
  )
  const updateWhichKeyDelay = useCallback(
    (value: string) => {
      if (!selectedSettingsEditable) return
      const whichKeyDelayMs = value === '' ? undefined : Number(value)
      selectedController.setSettings({
        ...selectedController.overrideSet.settings,
        whichKeyDelayMs:
          whichKeyDelayMs === undefined || Number.isNaN(whichKeyDelayMs)
            ? undefined
            : Math.max(0, whichKeyDelayMs),
      })
    },
    [selectedController, selectedSettingsEditable],
  )

  return {
    accountOverridesAvailable,
    spaceOverridesAvailable,
    rows,
    query,
    selectedCommandId,
    selectedScope,
    selectedRow,
    selectedBindings,
    selectedController,
    selectedLayerStatus,
    selectedLayerEditable,
    selectedSettingsEditable,
    commandConflicts,
    pendingConflict,
    pendingBinding,
    pendingReplace,
    capture,
    captureError,
    setQuery,
    setSelectedCommandId,
    setSelectedScope,
    startCapture,
    cancelCapture,
    replacePendingConflict,
    pendingConflictReplaceable,
    savePendingBinding,
    clearSelectedBindings,
    resetSelectedCommand,
    removeBinding,
    updateLeaderCombo,
    updateWhichKeyDelay,
  }
}
