import { LuKeyboard } from 'react-icons/lu'

import {
  Dialog,
  DialogContent,
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
        <DialogContent className="flex h-[min(44rem,calc(100vh-4rem))] w-[min(64rem,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] flex-col overflow-hidden p-0 sm:!max-w-4xl">
          <DialogHeader className="border-foreground/8 shrink-0 border-b px-4 py-3">
            <DialogTitle className="flex items-center gap-2 text-sm font-semibold tracking-tight">
              <LuKeyboard className="text-brand size-4" />
              Keyboard Shortcuts
            </DialogTitle>
          </DialogHeader>
          <div className="grid min-h-0 flex-1 grid-cols-[18rem_minmax(0,1fr)] overflow-hidden">
            <KeybindingCommandList />
            <KeybindingCommandDetails />
          </div>
        </DialogContent>
      </Dialog>
    </KeybindingEditorContext.Provider>
  )
}
