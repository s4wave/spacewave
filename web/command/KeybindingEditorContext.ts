import { createContext, use } from 'react'

import type { KeybindingEditorContextValue } from './component.js'

export const KeybindingEditorContext =
  createContext<KeybindingEditorContextValue | null>(null)

export function useKeybindingEditorContext(): KeybindingEditorContextValue {
  const value = use(KeybindingEditorContext)
  if (!value) {
    throw new Error('KeybindingEditorContext is missing')
  }
  return value
}
