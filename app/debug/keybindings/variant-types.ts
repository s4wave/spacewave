import type { KeybindingCommand } from './keybindings-model.js'

export interface KeybindingVariantProps {
  commands: readonly KeybindingCommand[]
  conflictCommandIds: ReadonlySet<string>
  customizedCommandIds: ReadonlySet<string>
  setBinding: (commandId: string, binding: string) => void
  resetBinding: (commandId: string) => void
}
