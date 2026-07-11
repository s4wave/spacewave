import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'
import type { CommandState } from '@s4wave/sdk/command/registry/registry.pb.js'

import type {
  KeybindingOverrideLayer,
  KeybindingOverrideScope,
  KeybindingOverrideSet,
  KeybindingOverrideSettings,
} from './keybinding-overrides.js'
import type {
  KeybindingConflict,
  ResolvedCommandBinding,
} from './KeybindingResolver.js'

export type KeybindingEditorScope = KeybindingOverrideScope

export interface KeybindingEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialScope?: KeybindingEditorScope
  initialCommandId?: string
}

export interface CommandRow {
  state: CommandState
  commandId: string
  label: string
  menuPath: string
  displayBindings: string[]
}

export interface CaptureState {
  kind: 'combo' | 'sequence'
  replace: boolean
  steps: string[]
  heldModifiers: string[]
}

export interface KeybindingLayerController {
  scope: KeybindingEditorScope
  label: string
  overrideSet: KeybindingOverrideSet
  layer: KeybindingOverrideLayer | null
  available: boolean
  readOnly: boolean
  loading: boolean
  error: Error | null
  settingsEditable: boolean
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

export interface KeybindingEditorContextValue {
  accountOverridesAvailable: boolean
  spaceOverridesAvailable: boolean
  rows: CommandRow[]
  query: string
  selectedCommandId: string | null
  selectedScope: KeybindingEditorScope
  selectedRow: CommandRow | undefined
  selectedBindings: ResolvedCommandBinding[]
  selectedController: KeybindingLayerController
  selectedLayerStatus: string | null
  selectedLayerEditable: boolean
  selectedSettingsEditable: boolean
  commandConflicts: KeybindingConflict[]
  pendingConflict: KeybindingConflict | undefined
  pendingBinding: CommandBinding | null
  pendingReplace: boolean
  capture: CaptureState | null
  captureError: string | null
  setQuery: (query: string) => void
  setSelectedCommandId: (commandId: string) => void
  setSelectedScope: (scope: KeybindingEditorScope) => void
  startCapture: (kind: 'combo' | 'sequence', replace: boolean) => void
  cancelCapture: () => void
  replacePendingConflict: () => void
  pendingConflictReplaceable: boolean
  savePendingBinding: () => void
  clearSelectedBindings: () => void
  resetSelectedCommand: () => void
  removeBinding: (binding: ResolvedCommandBinding) => void
  updateLeaderCombo: (leaderCombo: string) => void
  updateWhichKeyDelay: (value: string) => void
}

export const scopeLabels: Record<KeybindingEditorScope, string> = {
  local: 'Local',
  account: 'Account',
  space: 'Space',
}
