import { LuRotateCcw, LuTrash2 } from 'react-icons/lu'

import { Button } from '@s4wave/web/ui/button.js'

import { scopeLabels } from './component.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'

export function KeybindingEditorActions() {
  const {
    clearSelectedBindings,
    pendingBinding,
    pendingConflict,
    resetSelectedCommand,
    savePendingBinding,
    selectedController,
    selectedLayerEditable,
    selectedScope,
    startCapture,
  } = useKeybindingEditorContext()

  return (
    <>
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
          className="bg-brand text-background hover:bg-brand-highlight"
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
          variant="destructive"
          size="sm"
          disabled={!selectedLayerEditable}
          onClick={clearSelectedBindings}
        >
          <LuTrash2 className="size-3.5" />
          Disable command bindings
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!selectedLayerEditable}
          onClick={resetSelectedCommand}
        >
          <LuRotateCcw className="size-3.5" />
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
    </>
  )
}
