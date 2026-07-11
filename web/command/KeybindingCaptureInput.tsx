import { Button } from '@s4wave/web/ui/button.js'

import { formatKeybinding } from './CommandPalette.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { bindingDisplay } from './keybinding-editor-helpers.js'

export function KeybindingCaptureInput() {
  const {
    cancelCapture,
    capture,
    captureError,
    pendingBinding,
    pendingConflict,
    pendingReplace,
    selectedRow,
  } = useKeybindingEditorContext()

  return (
    <>
      {pendingBinding && (
        <div className="border-brand/20 bg-brand/10 rounded border px-3 py-2 text-sm">
          <div className="flex items-start justify-between gap-3">
            <div>
              Pending {pendingReplace ? 'replacement' : 'addition'}:{' '}
              <span className="text-brand font-mono">
                {bindingDisplay(pendingBinding)
                  .split(' ')
                  .map(formatKeybinding)
                  .join(' ')}
              </span>
            </div>
            <div className="min-h-5 shrink-0">
              {pendingConflict && (
                <span className="text-warning text-xs">
                  Already used by{' '}
                  {pendingConflict.bindings
                    .flatMap((binding) =>
                      binding.commandId === selectedRow?.commandId
                        ? []
                        : [binding.label],
                    )
                    .join(', ')}
                </span>
              )}
            </div>
          </div>
        </div>
      )}

      {capture && (
        <div
          data-keybinding-recorder
          role="status"
          aria-live="polite"
          className="border-brand/30 bg-brand/10 text-foreground ring-brand/20 rounded border p-3 text-sm ring-1"
        >
          <div className="flex items-start justify-between gap-3">
            <div>
              <div className="text-brand flex items-center gap-2 text-xs font-medium">
                <span className="bg-brand size-2 rounded-full" />
                Recording… press keys
              </div>
              <div className="text-foreground-alt mt-1 text-xs">
                {capture.kind === 'combo'
                  ? 'The first non-modifier key completes the combo.'
                  : 'Press sequence keys, then Enter to finish.'}
              </div>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={cancelCapture}
            >
              Cancel
            </Button>
          </div>
          <div className="border-foreground/8 bg-background/40 text-brand mt-3 rounded border px-3 py-2 font-mono text-sm">
            {capture.steps.length > 0 &&
              `${capture.steps.map(formatKeybinding).join(' ')} `}
            {capture.heldModifiers.length > 0
              ? formatKeybinding(`${capture.heldModifiers.join('+')}+…`)
              : 'Waiting for input…'}
          </div>
          <div className="text-foreground-alt/60 mt-2 text-xs">
            Escape or clicking away cancels.
          </div>
        </div>
      )}

      {captureError && (
        <div className="text-warning text-xs">{captureError}</div>
      )}
    </>
  )
}
