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
        <DialogContent className="bg-background-card flex h-dvh w-screen max-w-none flex-col gap-0 overflow-hidden rounded-none border-0 p-0 sm:h-[min(45rem,calc(100vh-2rem))] sm:w-[min(64rem,calc(100vw-2rem))] sm:!max-w-5xl sm:rounded-lg sm:border">
          <DialogHeader className="border-foreground/8 shrink-0 border-b px-4 py-3 pr-12 text-left sm:px-5 sm:py-4">
            <DialogTitle className="flex items-center gap-2 text-base font-semibold tracking-tight">
              <LuKeyboard className="text-brand size-4" />
              Keyboard shortcuts
            </DialogTitle>
            <DialogDescription className="text-foreground-alt/70 text-xs">
              Find a command, learn its shortcuts, or make the keyboard your
              own.
            </DialogDescription>
          </DialogHeader>
          <div className="grid min-h-0 flex-1 grid-cols-1 overflow-hidden sm:grid-cols-[17rem_minmax(0,1fr)]">
            <KeybindingCommandList />
            <KeybindingCommandDetails />
          </div>
        </DialogContent>
      </Dialog>
    </KeybindingEditorContext.Provider>
  )
}
