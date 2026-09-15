import { LuKeyboard } from 'react-icons/lu'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { KeybindingCommandDetails } from './KeybindingCommandDetails.js'
import { KeybindingCommandList } from './KeybindingCommandList.js'
import { KeybindingEditorContext } from './KeybindingEditorContext.js'
import { useKeybindingEditorModel } from './useKeybindingEditorModel.js'
import type { KeybindingEditorProps } from './component.js'

export {
  type KeybindingEditorProps,
  type KeybindingEditorScope,
} from './component.js'

export function KeybindingEditor({
  open,
  onOpenChange,
  initialScope = 'local',
  initialCommandId,
}: KeybindingEditorProps) {
  const model = useKeybindingEditorModel({
    open,
    initialScope,
    initialCommandId,
  })

  return (
    <KeybindingEditorContext.Provider value={model}>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent variant="editor">
          <DialogHeader variant="editor">
            <DialogTitle variant="editor">
              <LuKeyboard className="text-brand size-4" />
              Keyboard shortcuts
            </DialogTitle>
            <DialogDescription variant="editor">
              Find a command, learn its shortcuts, or make the keyboard your
              own.
            </DialogDescription>
          </DialogHeader>
          <div className="sm:grid-cols-keybinding-editor grid min-h-0 flex-1 grid-cols-1 overflow-hidden">
            <KeybindingCommandList />
            <KeybindingCommandDetails />
          </div>
        </DialogContent>
      </Dialog>
    </KeybindingEditorContext.Provider>
  )
}
