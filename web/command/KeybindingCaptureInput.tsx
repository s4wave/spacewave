import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { bindingDisplay } from './keybinding-editor-helpers.js'

export function KeybindingCaptureInput() {
  const {
    capture,
    captureError,
    handleCaptureKeyDown,
    pendingBinding,
    pendingConflict,
    pendingReplace,
  } = useKeybindingEditorContext()

  return (
    <>
      {pendingBinding && (
        <div className="border-brand/20 bg-brand/10 rounded border px-3 py-2 text-sm">
          <div className="flex items-start justify-between gap-3">
            <div>
              Pending {pendingReplace ? 'replacement' : 'addition'}:{' '}
              <span className="text-brand font-mono">
                {bindingDisplay(pendingBinding)}
              </span>
            </div>
            <div className="min-h-5 shrink-0">
              {pendingConflict && (
                <span className="text-warning text-xs">
                  Conflicts with{' '}
                  {pendingConflict.bindings
                    .map((binding) => binding.label)
                    .join(', ')}
                </span>
              )}
            </div>
          </div>
        </div>
      )}

      {capture && (
        <div
          role="button"
          tabIndex={0}
          className="border-brand/30 bg-brand/10 text-foreground ring-brand/20 rounded border px-3 py-3 text-sm ring-1 outline-none"
          onKeyDown={handleCaptureKeyDown}
        >
          <div className="text-brand flex items-center gap-2 text-xs font-medium">
            <span className="bg-brand h-2 w-2 rounded-full" />
            Recording · press keys now
          </div>
          <div className="mt-2">
            Press {capture.kind === 'combo' ? 'one combo' : 'sequence keys'}.
          </div>
          {capture.kind === 'sequence' && (
            <div className="text-brand/90 mt-1 font-mono text-xs">
              {capture.steps.join(' ')}
            </div>
          )}
        </div>
      )}

      {captureError && (
        <div className="text-warning text-xs">{captureError}</div>
      )}
    </>
  )
}
