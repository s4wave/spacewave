import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react'
import { LuKeyboard } from 'react-icons/lu'
import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { useCommands } from './CommandContext.js'
import { KeybindingCommandDetails } from './KeybindingCommandDetails.js'
import { KeybindingCommandList } from './KeybindingCommandList.js'
import { KeybindingEditorContext } from './KeybindingEditorContext.js'
import {
  comboFromKeyboardEvent,
  isModifierKey,
  resolveKeybindings,
  type ResolvedCommandBinding,
} from './KeybindingResolver.js'
import {
  bindingFromCombo,
  bindingFromSteps,
  bindingResolves,
  buildCommandRows,
  canLayerOverrideBinding,
  layerStatusMessage,
  previewPendingConflict,
} from './keybinding-editor-helpers.js'
import type { KeybindingOverrideLayer } from './keybinding-overrides.js'
import { useAccountKeybindingOverrides } from './useAccountKeybindingOverrides.js'
import { useLocalKeybindingOverrides } from './useLocalKeybindingOverrides.js'
import { useSpaceKeybindingOverrides } from './useSpaceKeybindingOverrides.js'
import type {
  CaptureState,
  KeybindingEditorContextValue,
  KeybindingEditorProps,
  KeybindingEditorScope,
  KeybindingLayerController,
} from './component.js'

export {
  type KeybindingEditorProps,
  type KeybindingEditorScope,
} from './component.js'

export function KeybindingEditor({
  open,
  onOpenChange,
  initialScope = 'local',
  initialCommandId,
}: KeybindingEditorProps) {
  const commands = useCommands()
  const localOverrides = useLocalKeybindingOverrides()
  const accountOverrides = useAccountKeybindingOverrides()
  const spaceOverrides = useSpaceKeybindingOverrides()
  const localController: KeybindingLayerController = {
    scope: 'local',
    label: 'Local',
    overrideSet: localOverrides.overrideSet,
    layer: localOverrides.layer,
    available: true,
    readOnly: false,
    loading: false,
    error: null,
    settingsEditable: true,
    setSettings: localOverrides.setSettings,
    setCommandBindings: localOverrides.setCommandBindings,
    addCommandBinding: localOverrides.addCommandBinding,
    clearCommandBindings: localOverrides.clearCommandBindings,
    clearCommandBindingId: localOverrides.clearCommandBindingId,
    removeCommandBinding: localOverrides.removeLocalCommandBinding,
    resetCommand: localOverrides.resetCommand,
    resetLayer: localOverrides.resetLayer,
  }
  const accountController: KeybindingLayerController = {
    scope: 'account',
    label: 'Account',
    overrideSet: accountOverrides.overrideSet,
    layer: accountOverrides.layer,
    available: accountOverrides.available,
    readOnly: accountOverrides.readOnly,
    loading: accountOverrides.loading,
    error: accountOverrides.error,
    settingsEditable: true,
    setSettings: accountOverrides.setSettings,
    setCommandBindings: accountOverrides.setCommandBindings,
    addCommandBinding: accountOverrides.addCommandBinding,
    clearCommandBindings: accountOverrides.clearCommandBindings,
    clearCommandBindingId: accountOverrides.clearCommandBindingId,
    removeCommandBinding: accountOverrides.removeCommandBinding,
    resetCommand: accountOverrides.resetCommand,
    resetLayer: accountOverrides.resetLayer,
  }
  const spaceController: KeybindingLayerController = {
    scope: 'space',
    label: 'Space',
    overrideSet: spaceOverrides.overrideSet,
    layer: spaceOverrides.layer,
    available: spaceOverrides.available,
    readOnly: spaceOverrides.readOnly,
    loading: spaceOverrides.loading,
    error: spaceOverrides.error,
    settingsEditable: true,
    setSettings: spaceOverrides.setSettings,
    setCommandBindings: spaceOverrides.setCommandBindings,
    addCommandBinding: spaceOverrides.addCommandBinding,
    clearCommandBindings: spaceOverrides.clearCommandBindings,
    clearCommandBindingId: spaceOverrides.clearCommandBindingId,
    removeCommandBinding: spaceOverrides.removeCommandBinding,
    resetCommand: spaceOverrides.resetCommand,
    resetLayer: spaceOverrides.resetLayer,
  }
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
  const layerControllers: Record<
    KeybindingEditorScope,
    KeybindingLayerController
  > = {
    local: localController,
    account: accountController,
    space: spaceController,
  }
  const selectedController = layerControllers[selectedScope]
  const selectedLayerEditable =
    selectedController.available &&
    !selectedController.readOnly &&
    !selectedController.loading &&
    !selectedController.error
  const selectedSettingsEditable =
    selectedLayerEditable && selectedController.settingsEditable
  const overrideLayers = useMemo(
    () =>
      [
        localController.layer,
        accountController.layer,
        spaceController.layer,
      ].filter((layer): layer is KeybindingOverrideLayer => Boolean(layer)),
    [localController.layer, accountController.layer, spaceController.layer],
  )
  const bindingGraph = useMemo(
    () => resolveKeybindings(commands, { overrideLayers }),
    [commands, overrideLayers],
  )

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
      })
      setPendingBinding(null)
      setPendingReplace(replace)
      setCaptureError(null)
    },
    [],
  )

  const handleCaptureKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLElement>) => {
      if (!capture || !selectedRow) return
      event.preventDefault()
      event.stopPropagation()
      if (event.key === 'Escape') {
        setCapture(null)
        setCaptureError(null)
        return
      }
      if (isModifierKey(event.nativeEvent)) return
      if (capture.kind === 'sequence' && event.key === 'Enter') {
        if (capture.steps.length > 1) {
          setPendingBinding(
            bindingFromSteps(
              selectedRow.commandId,
              selectedController.scope,
              capture.steps,
            ),
          )
          setCapture(null)
        }
        return
      }

      const combo = comboFromKeyboardEvent(event.nativeEvent)
      if (!combo) return
      const nextSteps = [...capture.steps, combo]
      const binding =
        capture.kind === 'combo'
          ? bindingFromCombo(
              selectedRow.commandId,
              selectedController.scope,
              combo,
            )
          : bindingFromSteps(
              selectedRow.commandId,
              selectedController.scope,
              nextSteps,
            )
      if (!bindingResolves(selectedRow.state, binding)) {
        setCaptureError('That binding could not be parsed.')
        return
      }
      setPendingBinding(binding)
      setPendingReplace(capture.replace)
      setCaptureError(null)
      if (capture.kind === 'combo') {
        setCapture(null)
        return
      }
      setCapture({ ...capture, steps: nextSteps })
    },
    [capture, selectedController.scope, selectedRow],
  )

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

  const contextValue: KeybindingEditorContextValue = {
    accountOverridesAvailable: accountOverrides.available,
    spaceOverridesAvailable: spaceOverrides.available,
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
    handleCaptureKeyDown,
    savePendingBinding,
    clearSelectedBindings,
    resetSelectedCommand,
    removeBinding,
    updateLeaderCombo,
    updateWhichKeyDelay,
  }

  return (
    <KeybindingEditorContext.Provider value={contextValue}>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="flex h-[min(44rem,calc(100vh-4rem))] w-[min(64rem,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] flex-col overflow-hidden p-0 sm:!max-w-4xl">
          <DialogHeader className="border-foreground/8 shrink-0 border-b px-4 py-3">
            <DialogTitle className="flex items-center gap-2 text-sm font-semibold tracking-tight">
              <LuKeyboard className="text-brand h-4 w-4" />
              Keyboard Shortcuts
            </DialogTitle>
          </DialogHeader>
          <div className="grid min-h-0 flex-1 grid-cols-[18rem_minmax(0,1fr)] overflow-hidden">
            <KeybindingCommandList />
            <KeybindingCommandDetails />
          </div>
        </DialogContent>
      </Dialog>
    </KeybindingEditorContext.Provider>
  )
}
