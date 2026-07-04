import { KeybindingBindingList } from './KeybindingBindingList.js'
import { KeybindingCaptureInput } from './KeybindingCaptureInput.js'
import { KeybindingConflictList } from './KeybindingConflictList.js'
import { KeybindingDiscoverySettings } from './KeybindingDiscoverySettings.js'
import { KeybindingEditorActions } from './KeybindingEditorActions.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'

export function KeybindingCommandDetails() {
  const {
    commandConflicts,
    pendingConflict,
    selectedLayerStatus,
    selectedRow,
  } = useKeybindingEditorContext()

  if (!selectedRow) {
    return (
      <section className="min-h-0 overflow-auto p-4">
        <div className="text-foreground-alt/40 text-sm">
          Select a command to edit its keybindings.
        </div>
      </section>
    )
  }

  const conflicts = pendingConflict ? [pendingConflict] : commandConflicts

  return (
    <section className="min-h-0 overflow-auto p-4 pb-6">
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

        <KeybindingDiscoverySettings />
        <KeybindingBindingList />
        {conflicts.length > 0 && (
          <KeybindingConflictList conflicts={conflicts} />
        )}
        <KeybindingCaptureInput />
        <KeybindingEditorActions />
      </div>
    </section>
  )
}
