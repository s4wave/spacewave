import {
  CommandFocusContext,
  type CommandBinding,
} from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import {
  addCommandBindingOverride,
  clearCommandBindingIdOverride,
  createKeybindingOverrideLayer,
  keybindingBindingStorageId,
  removeLocalCommandBindingOverride,
  setCommandBindingsOverride,
  type KeybindingOverrideLayer,
  type KeybindingOverrideScope,
  type KeybindingOverrideSet,
} from './keybinding-overrides.js'
import {
  getCommandDisplayBindings,
  resolveKeybindings,
  type KeybindingConflict,
  type KeybindingGraph,
} from './KeybindingResolver.js'
import type { CommandRow, KeybindingLayerController } from './component.js'

export function layerStatusMessage(
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

export function canLayerOverrideBinding(
  layer: KeybindingOverrideScope,
  sourceLayer: KeybindingOverrideScope | 'default',
): boolean {
  return layerPrecedence(layer) >= layerPrecedence(sourceLayer)
}

export function buildCommandRows(
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

export function previewPendingConflict(
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

export function replaceConflictingBindingOverride(
  currentOverrideSet: KeybindingOverrideSet,
  scope: KeybindingOverrideScope,
  commandId: string,
  binding: CommandBinding,
  replace: boolean,
  conflict: KeybindingConflict,
): KeybindingOverrideSet {
  let next = currentOverrideSet
  for (const current of conflict.bindings) {
    if (current.commandId === commandId) continue
    next =
      current.sourceLayer === scope
        ? removeLocalCommandBindingOverride(
            next,
            current.commandId,
            current.bindingId,
          )
        : clearCommandBindingIdOverride(
            next,
            current.commandId,
            current.bindingId,
          )
  }
  return replace
    ? setCommandBindingsOverride(next, commandId, [binding])
    : addCommandBindingOverride(next, commandId, binding)
}

export function bindingFromCombo(
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

export function bindingFromSteps(
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

export function bindingDisplay(binding: CommandBinding): string {
  if (binding.binding?.case === 'sequence') {
    return binding.binding.value.steps?.join(' ') ?? ''
  }
  if (binding.binding?.case === 'combo') {
    return binding.binding.value.combo ?? ''
  }
  return keybindingBindingStorageId(binding)
}

export function bindingResolves(
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

function layerPrecedence(layer: KeybindingOverrideScope | 'default'): number {
  if (layer === 'space') return 3
  if (layer === 'account') return 2
  if (layer === 'local') return 1
  return 0
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
