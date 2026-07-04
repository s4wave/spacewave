import { Button } from '@s4wave/web/ui/button.js'

import { focusContextLabel } from './KeybindingResolver.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { canLayerOverrideBinding } from './keybinding-editor-helpers.js'

export function KeybindingBindingList() {
  const {
    selectedBindings,
    selectedLayerEditable,
    selectedScope,
    removeBinding,
  } = useKeybindingEditorContext()

  return (
    <div className="space-y-2">
      <div className="text-foreground text-xs font-medium">
        Effective bindings
      </div>
      {selectedBindings.length === 0 ? (
        <div className="text-foreground-alt/40 border-foreground/8 rounded border px-3 py-2 text-sm">
          No keyboard binding. The command still works from the palette and
          menus.
        </div>
      ) : (
        selectedBindings.map((binding) => (
          <div
            key={`${binding.sourceLayer}:${binding.bindingId}:${binding.display}`}
            className="border-foreground/8 flex items-center justify-between gap-3 rounded border px-3 py-2"
          >
            <div className="min-w-0">
              <div className="text-brand font-mono text-sm">
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
                !canLayerOverrideBinding(selectedScope, binding.sourceLayer)
              }
              onClick={() => removeBinding(binding)}
            >
              Clear
            </Button>
          </div>
        ))
      )}
    </div>
  )
}
