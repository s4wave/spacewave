import { LuPlus, LuRotateCcw, LuTrash2 } from 'react-icons/lu'

import { Button } from '@s4wave/web/ui/button.js'

import { formatKeybinding } from './CommandPalette.js'
import { focusContextLabel } from './KeybindingResolver.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import {
  bindingDisplay,
  canLayerOverrideBinding,
} from './keybinding-editor-helpers.js'

export function KeybindingBindingsSection() {
  const {
    cancelCapture,
    capture,
    captureError,
    clearSelectedBindings,
    pendingBinding,
    pendingConflict,
    pendingConflictReplaceable,
    pendingReplace,
    removeBinding,
    replacePendingConflict,
    resetSelectedCommand,
    savePendingBinding,
    selectedBindings,
    selectedLayerEditable,
    selectedRow,
    selectedScope,
    startCapture,
  } = useKeybindingEditorContext()

  return (
    <div className="space-y-4">
      <div>
        <div className="mb-2 flex items-center justify-between gap-3">
          <h3 className="text-foreground text-xs font-medium">Shortcuts</h3>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!selectedLayerEditable || Boolean(capture)}
            onClick={() => startCapture('combo', selectedBindings.length > 0)}
          >
            <LuPlus />
            Record shortcut
          </Button>
        </div>
        {selectedBindings.length === 0 ? (
          <div className="text-foreground-alt border-foreground/8 rounded-md border border-dashed px-3 py-5 text-center text-sm">
            No keyboard shortcut. You can still run this command from the
            command palette or menu.
          </div>
        ) : (
          <div className="space-y-2">
            {selectedBindings.map((binding) => (
              <div
                key={`${binding.sourceLayer}:${binding.bindingId}:${binding.display}`}
                className="border-foreground/8 bg-background-card-alt/30 flex min-h-12 items-center justify-between gap-3 rounded-md border px-3 py-2"
              >
                <div className="min-w-0">
                  <div className="text-brand font-mono text-sm">
                    {binding.display.split(' ').map(formatKeybinding).join(' ')}
                  </div>
                  <div className="text-foreground-alt/70 text-xs">
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
                    !canLayerOverrideBinding(selectedScope, binding.sourceLayer)
                  }
                  onClick={() => removeBinding(binding)}
                >
                  Remove
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      {capture && (
        <div
          data-keybinding-recorder
          role="status"
          tabIndex={-1}
          aria-live="polite"
          className="border-brand/30 bg-brand/10 ring-brand/20 rounded-md border p-4 text-center ring-1"
        >
          <div className="text-brand text-xs font-semibold tracking-wide uppercase">
            Recording shortcut
          </div>
          <div className="text-foreground mt-3 min-h-12 text-lg font-medium">
            {capture.steps.length > 0 &&
              `${capture.steps.map(formatKeybinding).join(' ')} `}
            {capture.heldModifiers.length > 0
              ? formatKeybinding(`${capture.heldModifiers.join('+')}+…`)
              : 'Press the keys you want to use'}
          </div>
          <div className="text-foreground-alt mt-1 text-xs">
            Escape cancels. The first non-modifier key completes the shortcut.
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mt-3"
            onClick={cancelCapture}
          >
            Cancel
          </Button>
        </div>
      )}

      {pendingBinding && (
        <div className="border-brand/20 bg-brand/5 rounded-md border p-3">
          <div className="text-foreground-alt text-xs">
            {pendingReplace ? 'Replace with' : 'Add'}
          </div>
          <div className="text-brand mt-1 font-mono text-base">
            {bindingDisplay(pendingBinding)
              .split(' ')
              .map(formatKeybinding)
              .join(' ')}
          </div>
          {pendingConflict && (
            <div className="text-warning mt-2 text-xs">
              This shortcut is already used by{' '}
              {pendingConflict.bindings
                .flatMap((binding) =>
                  binding.commandId === selectedRow?.commandId
                    ? []
                    : [binding.label],
                )
                .join(', ')}
              .
            </div>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            {!pendingConflict && (
              <Button type="button" size="sm" onClick={savePendingBinding}>
                Save shortcut
              </Button>
            )}
            {pendingConflict && (
              <Button
                type="button"
                size="sm"
                disabled={!pendingConflictReplaceable}
                onClick={replacePendingConflict}
              >
                {pendingConflictReplaceable
                  ? 'Replace existing shortcut'
                  : 'Choose another scope or shortcut'}
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={cancelCapture}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}

      {captureError && (
        <div className="text-warning text-xs">{captureError}</div>
      )}

      <div className="border-foreground/8 flex flex-wrap gap-2 border-t pt-4">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!selectedLayerEditable}
          onClick={() => startCapture('sequence', false)}
        >
          Add key sequence
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={!selectedLayerEditable}
          onClick={resetSelectedCommand}
        >
          <LuRotateCcw />
          Use inherited shortcuts
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="text-error hover:text-error"
          disabled={!selectedLayerEditable || selectedBindings.length === 0}
          onClick={clearSelectedBindings}
        >
          <LuTrash2 />
          Disable shortcuts
        </Button>
      </div>
    </div>
  )
}
