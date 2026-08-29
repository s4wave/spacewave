import {
  useEffect,
  useEffectEvent,
  type Dispatch,
  type SetStateAction,
} from 'react'
import type { CommandBinding } from '@s4wave/sdk/command/command.pb.js'

import { comboFromKeyboardEvent, isModifierKey } from './KeybindingResolver.js'
import {
  bindingFromCombo,
  bindingFromSteps,
  bindingResolves,
} from './keybinding-editor-helpers.js'
import type {
  CaptureState,
  CommandRow,
  KeybindingEditorScope,
} from './component.js'

interface KeybindingRecorderOptions {
  capture: CaptureState | null
  selectedRow: CommandRow | undefined
  selectedScope: KeybindingEditorScope
  setCapture: Dispatch<SetStateAction<CaptureState | null>>
  setPendingBinding: Dispatch<SetStateAction<CommandBinding | null>>
  setPendingReplace: Dispatch<SetStateAction<boolean>>
  setCaptureError: Dispatch<SetStateAction<string | null>>
  cancelCapture: () => void
}

// useKeybindingRecorder owns keyboard and pointer events for one recording session.
export function useKeybindingRecorder({
  capture,
  selectedRow,
  selectedScope,
  setCapture,
  setPendingBinding,
  setPendingReplace,
  setCaptureError,
  cancelCapture,
}: KeybindingRecorderOptions): void {
  const handleKeyDown = useEffectEvent((event: KeyboardEvent) => {
    if (!capture || !selectedRow) return
    event.preventDefault()
    event.stopPropagation()
    event.stopImmediatePropagation()
    if (event.key === 'Escape') {
      cancelCapture()
      return
    }
    if (isModifierKey(event)) {
      setCapture({
        ...capture,
        heldModifiers: heldModifiersFromKeyboardEvent(event),
      })
      return
    }
    if (capture.kind === 'sequence' && event.key === 'Enter') {
      if (capture.steps.length > 1) {
        setPendingBinding(
          bindingFromSteps(selectedRow.commandId, selectedScope, capture.steps),
        )
        setCapture(null)
      }
      return
    }

    const combo = comboFromKeyboardEvent(event)
    const nextSteps = [...capture.steps, combo]
    const binding =
      capture.kind === 'combo'
        ? bindingFromCombo(selectedRow.commandId, selectedScope, combo)
        : bindingFromSteps(selectedRow.commandId, selectedScope, nextSteps)
    if (!bindingResolves(selectedRow.state, binding)) {
      setCaptureError('That binding could not be parsed.')
      return
    }
    setPendingBinding(binding)
    setPendingReplace(capture.replace)
    setCaptureError(null)
    if (capture.kind === 'combo') {
      setCapture(null)
      return
    }
    setCapture({ ...capture, steps: nextSteps, heldModifiers: [] })
  })

  const handleKeyUp = useEffectEvent((event: KeyboardEvent) => {
    if (!capture || !isModifierKey(event)) return
    event.preventDefault()
    event.stopPropagation()
    event.stopImmediatePropagation()
    setCapture({
      ...capture,
      heldModifiers: heldModifiersFromKeyboardEvent(event),
    })
  })

  const recording = Boolean(capture)
  useEffect(() => {
    if (!recording) return
    document.documentElement.dataset.keybindingRecording = 'true'
    document.addEventListener('keydown', handleKeyDown, true)
    document.addEventListener('keyup', handleKeyUp, true)
    document.querySelector<HTMLElement>('[data-keybinding-recorder]')?.focus()
    return () => {
      delete document.documentElement.dataset.keybindingRecording
      document.removeEventListener('keydown', handleKeyDown, true)
      document.removeEventListener('keyup', handleKeyUp, true)
    }
  }, [recording])
}

function heldModifiersFromKeyboardEvent(event: KeyboardEvent): string[] {
  const modifiers: string[] = []
  if (event.metaKey) modifiers.push('meta')
  if (event.ctrlKey) modifiers.push('ctrl')
  if (event.altKey) modifiers.push('alt')
  if (event.shiftKey) modifiers.push('shift')
  return modifiers
}
