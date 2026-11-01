import { LuKeyboard } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    display: 'grid',
    minHeight: 0,
    flex: 1,
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'minmax(18rem, .9fr) minmax(22rem, 1.1fr)',
    },
    overflow: 'hidden',
  },
})

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
              <LuKeyboard {...stylex.props(stylexStyles.rule1)} />
              Keyboard shortcuts
            </DialogTitle>
            <DialogDescription variant="editor">
              Find a command, learn its shortcuts, or make the keyboard your
              own.
            </DialogDescription>
          </DialogHeader>
          <div sx={stylexStyles.rule2}>
            <KeybindingCommandList />
            <KeybindingCommandDetails />
          </div>
        </DialogContent>
      </Dialog>
    </KeybindingEditorContext.Provider>
  )
}
