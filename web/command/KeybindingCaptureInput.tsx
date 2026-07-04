import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { bindingDisplay } from './keybinding-editor-helpers.js'

export function KeybindingCaptureInput() {
  const {
    capture,
    captureError,
    handleCaptureKeyDown,
    pendingBinding,
    pendingReplace,
  } = useKeybindingEditorContext()

  return (
    <>
      {pendingBinding && (
        <div className="border-brand/20 bg-brand/10 rounded border px-3 py-2 text-sm">
          Pending {pendingReplace ? 'replacement' : 'addition'}:{' '}
          <span className="font-mono">{bindingDisplay(pendingBinding)}</span>
        </div>
      )}

      {capture && (
        <div
          role="button"
          tabIndex={0}
          className="border-brand/30 bg-brand/10 text-foreground rounded border px-3 py-4 text-sm outline-none"
          onKeyDown={handleCaptureKeyDown}
        >
          Press {capture.kind === 'combo' ? 'one combo' : 'sequence keys'}.
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
    </>
  )
}
