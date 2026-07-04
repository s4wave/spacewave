import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react'
import { LuKeyboard, LuRotateCcw, LuSearch, LuTrash2 } from 'react-icons/lu'
import {
  CommandFocusContext,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { useCommands } from './CommandContext.js'
import { formatKeybindingHint } from './CommandPalette.js'
import {
  comboFromKeyboardEvent,
  focusContextLabel,
  getCommandDisplayBindings,
  isModifierKey,
  resolveKeybindings,
  type KeybindingConflict,
  type KeybindingGraph,
  type ResolvedCommandBinding,
} from './KeybindingResolver.js'
import {
  addCommandBindingOverride,
  createKeybindingOverrideLayer,
  keybindingBindingStorageId,
  setCommandBindingsOverride,
  type KeybindingOverrideLayer,
  type KeybindingOverrideScope,
  type KeybindingOverrideSet,
} from './keybinding-overrides.js'
import { useAccountKeybindingOverrides } from './useAccountKeybindingOverrides.js'
import { useLocalKeybindingOverrides } from './useLocalKeybindingOverrides.js'
import { useSpaceKeybindingOverrides } from './useSpaceKeybindingOverrides.js'

export type KeybindingEditorScope = 'local' | 'account' | 'space'

export interface KeybindingEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialScope?: KeybindingEditorScope
  initialCommandId?: string
}

interface CommandRow {
  state: CommandState
  commandId: string
  label: string
  menuPath: string
  displayBindings: string[]
}

interface CaptureState {
  kind: 'combo' | 'sequence'
  replace: boolean
  steps: string[]
}

interface KeybindingLayerController {
  scope: KeybindingEditorScope
  label: string
  overrideSet: KeybindingOverrideSet
  layer: KeybindingOverrideLayer | null
  available: boolean
  readOnly: boolean
  loading: boolean
  error: Error | null
  setCommandBindings: (commandId: string, bindings: CommandBinding[]) => void
  addCommandBinding: (commandId: string, binding: CommandBinding) => void
  clearCommandBindings: (commandId: string) => void
  clearCommandBindingId: (commandId: string, bindingId: string) => void
  removeCommandBinding: (commandId: string, bindingId: string) => void
  resetCommand: (commandId: string) => void
  resetLayer: () => void
}

const scopeLabels: Record<KeybindingEditorScope, string> = {
  local: 'Local',
  account: 'Account',
  space: 'Space',
}

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
      } else {
        setCapture({ ...capture, steps: nextSteps })
      }
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
    } else {
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[min(44rem,calc(100vh-4rem))] max-w-4xl overflow-hidden p-0">
        <DialogHeader className="border-foreground/8 border-b px-4 py-3">
          <DialogTitle className="flex items-center gap-2 text-sm font-semibold tracking-tight">
            <LuKeyboard className="h-4 w-4" />
            Keyboard Shortcuts
          </DialogTitle>
        </DialogHeader>
        <div className="grid min-h-0 grid-cols-[18rem_minmax(0,1fr)] overflow-hidden">
          <aside className="border-foreground/8 flex min-h-0 flex-col border-r">
            <div className="border-foreground/8 border-b p-3">
              <label className="text-foreground-alt mb-1 block text-xs font-medium">
                Scope
              </label>
              <select
                className="bg-background border-foreground/10 text-foreground w-full rounded border px-2 py-1.5 text-sm outline-none"
                value={selectedScope}
                onChange={(event) =>
                  setSelectedScope(event.target.value as KeybindingEditorScope)
                }
              >
                <option value="local">Local</option>
                <option value="account" disabled={!accountOverrides.available}>
                  {accountOptionLabel(accountOverrides.available)}
                </option>
                <option value="space" disabled={!spaceOverrides.available}>
                  {spaceOptionLabel(spaceOverrides.available)}
                </option>
              </select>
            </div>
            <div className="border-foreground/8 flex items-center gap-2 border-b px-3 py-2">
              <LuSearch className="text-foreground-alt h-3.5 w-3.5" />
              <input
                className="placeholder:text-foreground-alt/50 text-foreground min-w-0 flex-1 bg-transparent text-sm outline-none"
                placeholder="Search commands..."
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                onInput={(event) => setQuery(event.currentTarget.value)}
              />
            </div>
            <div className="min-h-0 flex-1 overflow-auto p-2">
              {rows.length === 0 && (
                <div className="text-foreground-alt/40 px-2 py-4 text-sm">
                  No commands found.
                </div>
              )}
              {rows.map((row) => (
                <button
                  key={row.commandId}
                  type="button"
                  className={cn(
                    'hover:bg-foreground/5 flex w-full flex-col gap-1 rounded px-2 py-2 text-left transition-colors',
                    row.commandId === selectedRow?.commandId &&
                      'bg-brand/10 text-brand',
                  )}
                  onClick={() => setSelectedCommandId(row.commandId)}
                >
                  <span className="text-foreground text-xs font-medium">
                    {row.label}
                  </span>
                  <span className="text-foreground-alt/50 truncate text-[0.6rem]">
                    {row.commandId}
                  </span>
                  {row.displayBindings.length > 0 && (
                    <span className="text-foreground-alt/60 font-mono text-[0.6rem]">
                      {formatKeybindingHint(row.displayBindings)}
                    </span>
                  )}
                </button>
              ))}
            </div>
          </aside>
          <section className="min-h-0 overflow-auto p-4">
            {!selectedRow ? (
              <div className="text-foreground-alt/40 text-sm">
                Select a command to edit its keybindings.
              </div>
            ) : (
              <div className="space-y-4">
                <div>
                  <div className="text-foreground text-sm font-semibold">
                    {selectedRow.label}
                  </div>
                  <div className="text-foreground-alt/50 text-xs">
                    {selectedRow.commandId}
                  </div>
                  {selectedRow.menuPath && (
                    <div className="text-foreground-alt/50 text-xs">
                      Menu: {selectedRow.menuPath}
                    </div>
                  )}
                </div>

                {selectedLayerStatus && (
                  <div className="border-warning/30 bg-warning/10 text-warning rounded border px-3 py-2 text-xs">
                    {selectedLayerStatus}
                  </div>
                )}

                <div className="space-y-2">
                  <div className="text-foreground text-xs font-medium">
                    Effective bindings
                  </div>
                  {selectedBindings.length === 0 ? (
                    <div className="text-foreground-alt/40 border-foreground/8 rounded border px-3 py-2 text-sm">
                      No keyboard binding. The command still works from the
                      palette and menus.
                    </div>
                  ) : (
                    selectedBindings.map((binding) => (
                      <div
                        key={`${binding.sourceLayer}:${binding.bindingId}:${binding.display}`}
                        className="border-foreground/8 flex items-center justify-between gap-3 rounded border px-3 py-2"
                      >
                        <div className="min-w-0">
                          <div className="text-foreground text-sm">
                            {binding.display}
                          </div>
                          <div className="text-foreground-alt/50 text-xs">
                            {binding.sourceLayerLabel} ·{' '}
                            {focusContextLabel(binding.context)}
                          </div>
                        </div>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={
                            !selectedLayerEditable ||
                            !canLayerOverrideBinding(
                              selectedScope,
                              binding.sourceLayer,
                            )
                          }
                          onClick={() => removeBinding(binding)}
                        >
                          Clear
                        </Button>
                      </div>
                    ))
                  )}
                </div>

                {(commandConflicts.length > 0 || pendingConflict) && (
                  <ConflictList
                    conflicts={
                      pendingConflict ? [pendingConflict] : commandConflicts
                    }
                  />
                )}

                {pendingBinding && (
                  <div className="border-brand/20 bg-brand/10 rounded border px-3 py-2 text-sm">
                    Pending {pendingReplace ? 'replacement' : 'addition'}:{' '}
                    <span className="font-mono">
                      {bindingDisplay(pendingBinding)}
                    </span>
                  </div>
                )}

                {capture && (
                  <div
                    role="button"
                    tabIndex={0}
                    className="border-brand/30 bg-brand/10 text-foreground rounded border px-3 py-4 text-sm outline-none"
                    onKeyDown={handleCaptureKeyDown}
                  >
                    Press{' '}
                    {capture.kind === 'combo' ? 'one combo' : 'sequence keys'}.
                    {capture.kind === 'sequence' && (
                      <div className="text-foreground-alt/60 mt-1 font-mono text-xs">
                        {capture.steps.join(' ')}
                      </div>
                    )}
                  </div>
                )}

                {captureError && (
                  <div className="text-warning text-xs">{captureError}</div>
                )}

                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={() => startCapture('combo', true)}
                  >
                    Replace with combo
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={() => startCapture('combo', false)}
                  >
                    Add combo
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={() => startCapture('sequence', true)}
                  >
                    Replace with Leader sequence
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={() => startCapture('sequence', false)}
                  >
                    Add Leader sequence
                  </Button>
                </div>

                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    disabled={
                      !selectedLayerEditable ||
                      !pendingBinding ||
                      Boolean(pendingConflict)
                    }
                    onClick={savePendingBinding}
                  >
                    Save binding
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={clearSelectedBindings}
                  >
                    <LuTrash2 className="h-3.5 w-3.5" />
                    Disable command bindings
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={resetSelectedCommand}
                  >
                    <LuRotateCcw className="h-3.5 w-3.5" />
                    Reset command
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!selectedLayerEditable}
                    onClick={selectedController.resetLayer}
                  >
                    Reset {scopeLabels[selectedScope]} layer
                  </Button>
                </div>
              </div>
            )}
          </section>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function accountOptionLabel(available: boolean): string {
  return available ? 'Account' : 'Account (unavailable)'
}

function spaceOptionLabel(available: boolean): string {
  return available ? 'Space' : 'Space (unavailable)'
}

function layerStatusMessage(
  controller: KeybindingLayerController,
): string | null {
  if (!controller.available) {
    return `${controller.label} overrides are unavailable in this context.`
  }
  if (controller.loading) return `Loading ${controller.label} overrides.`
  if (controller.error) {
    return `${controller.label} overrides could not load: ${controller.error.message}`
  }
  if (controller.readOnly) {
    return `${controller.label} overrides are read-only in this context.`
  }
  return null
}

function canLayerOverrideBinding(
  layer: KeybindingOverrideScope,
  sourceLayer: KeybindingOverrideScope | 'default',
): boolean {
  return layerPrecedence(layer) >= layerPrecedence(sourceLayer)
}

function layerPrecedence(layer: KeybindingOverrideScope | 'default'): number {
  if (layer === 'space') return 3
  if (layer === 'account') return 2
  if (layer === 'local') return 1
  return 0
}

function ConflictList({ conflicts }: { conflicts: KeybindingConflict[] }) {
  return (
    <div className="border-warning/30 bg-warning/10 rounded border px-3 py-2">
      <div className="text-warning text-xs font-medium">Conflict</div>
      <div className="mt-1 space-y-1">
        {conflicts.map((conflict) => (
          <div
            key={`${conflict.context}:${conflict.kind}:${conflict.key}`}
            className="text-foreground-alt text-xs"
          >
            {focusContextLabel(conflict.context)} {conflict.kind}{' '}
            <span className="font-mono">{conflict.key}</span> is used by{' '}
            {conflict.bindings.map((binding) => binding.label).join(', ')}.
          </div>
        ))}
      </div>
    </div>
  )
}

function buildCommandRows(
  commands: CommandState[],
  bindingGraph: KeybindingGraph,
  query: string,
): CommandRow[] {
  const normalizedQuery = query.trim().toLowerCase()
  return commands
    .flatMap((state) => {
      const command = state.command
      const commandId = command?.commandId
      if (!command || !commandId || !state.active) return []
      const displayBindings = getCommandDisplayBindings(bindingGraph, commandId)
      const row: CommandRow = {
        state,
        commandId,
        label: command.label ?? commandId,
        menuPath: command.menuPath ?? '',
        displayBindings,
      }
      if (!normalizedQuery) return [row]
      const searchable = [
        row.label,
        row.commandId,
        row.menuPath,
        ...displayBindings,
      ]
        .join(' ')
        .toLowerCase()
      return searchable.includes(normalizedQuery) ? [row] : []
    })
    .sort((a, b) => a.label.localeCompare(b.label))
}

function previewPendingConflict(
  commands: CommandState[],
  overrideLayers: KeybindingOverrideLayer[],
  scope: KeybindingOverrideScope,
  label: string,
  currentOverrideSet: KeybindingOverrideSet,
  commandId: string,
  binding: CommandBinding,
  replace: boolean,
): KeybindingConflict | undefined {
  const overrideSet = replace
    ? setCommandBindingsOverride(currentOverrideSet, commandId, [binding])
    : addCommandBindingOverride(currentOverrideSet, commandId, binding)
  const draftLayer = createKeybindingOverrideLayer(scope, label, overrideSet)
  const graph = resolveKeybindings(commands, {
    overrideLayers: overrideLayers.map((layer) =>
      layer.scope === scope ? draftLayer : layer,
    ),
  })
  return graph.conflicts.find((conflict) =>
    conflict.bindings.some((candidate) => candidate.commandId === commandId),
  )
}

function bindingFromCombo(
  commandId: string,
  scope: KeybindingOverrideScope,
  combo: string,
): CommandBinding {
  return {
    id: bindingId(commandId, scope, 'combo', combo),
    binding: { case: 'combo', value: { combo } },
    when: CommandFocusContext.GLOBAL,
  }
}

function bindingFromSteps(
  commandId: string,
  scope: KeybindingOverrideScope,
  steps: string[],
): CommandBinding {
  return {
    id: bindingId(commandId, scope, 'sequence', steps.join(' ')),
    binding: { case: 'sequence', value: { steps } },
    when: CommandFocusContext.GLOBAL,
  }
}

function bindingId(
  commandId: string,
  scope: KeybindingOverrideScope,
  kind: string,
  value: string,
): string {
  return `${scope}-${commandId}-${kind}-${value}`.replace(
    /[^a-z0-9._:-]+/gi,
    '-',
  )
}

function bindingDisplay(binding: CommandBinding): string {
  if (binding.binding?.case === 'sequence') {
    return binding.binding.value.steps?.join(' ') ?? ''
  }
  if (binding.binding?.case === 'combo')
    return binding.binding.value.combo ?? ''
  return keybindingBindingStorageId(binding)
}

function bindingResolves(
  state: CommandState,
  binding: CommandBinding,
): boolean {
  const commandId = state.command?.commandId
  if (!commandId) return false
  const graph = resolveKeybindings([
    {
      ...state,
      command: {
        ...state.command,
        defaultBindings: [binding],
        keybinding: '',
      },
      active: true,
      enabled: true,
    },
  ])
  return Boolean(graph.bindingsByCommandId.get(commandId)?.length)
}
