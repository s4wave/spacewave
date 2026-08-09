import { useCallback, useMemo, useState } from 'react'

import {
  MOCK_KEYBINDING_COMMANDS,
  contextsOverlap,
  type KeybindingCommand,
} from './keybindings-model.js'
import { chordComparisonKey } from './keyboard-utils.js'

export interface KeybindingsPrototypeState {
  commands: readonly KeybindingCommand[]
  conflictCommandIds: ReadonlySet<string>
  conflictCount: number
  customizedCommandIds: ReadonlySet<string>
  setBinding: (commandId: string, binding: string) => void
  resetBinding: (commandId: string) => void
  resetAllBindings: () => void
}

function createInitialBindings(): Record<string, string> {
  return Object.fromEntries(
    MOCK_KEYBINDING_COMMANDS.map((command) => [
      command.id,
      command.defaultBinding,
    ]),
  )
}

export function useKeybindingsPrototype(): KeybindingsPrototypeState {
  const [bindings, setBindings] = useState<Record<string, string>>(
    createInitialBindings,
  )

  const commands = useMemo(
    () =>
      MOCK_KEYBINDING_COMMANDS.map((command) => ({
        ...command,
        binding: bindings[command.id] ?? command.defaultBinding,
      })),
    [bindings],
  )

  const conflictCommandIds = useMemo(() => {
    const conflicts = new Set<string>()
    for (let leftIndex = 0; leftIndex < commands.length; leftIndex++) {
      const left = commands[leftIndex]
      if (!left.binding) continue
      for (
        let rightIndex = leftIndex + 1;
        rightIndex < commands.length;
        rightIndex++
      ) {
        const right = commands[rightIndex]
        if (
          right.binding &&
          chordComparisonKey(left.binding) ===
            chordComparisonKey(right.binding) &&
          contextsOverlap(left.context, right.context)
        ) {
          conflicts.add(left.id)
          conflicts.add(right.id)
        }
      }
    }
    return conflicts
  }, [commands])

  const customizedCommandIds = useMemo(() => {
    const customized = new Set<string>()
    for (const command of commands) {
      if (command.binding !== command.defaultBinding) customized.add(command.id)
    }
    return customized
  }, [commands])

  const setBinding = useCallback((commandId: string, binding: string) => {
    setBindings((current) => {
      if (current[commandId] === binding) return current
      return { ...current, [commandId]: binding }
    })
  }, [])

  const resetBinding = useCallback((commandId: string) => {
    const command = MOCK_KEYBINDING_COMMANDS.find(
      (candidate) => candidate.id === commandId,
    )
    if (!command) return

    setBindings((current) => {
      if (current[commandId] === command.defaultBinding) return current
      return { ...current, [commandId]: command.defaultBinding }
    })
  }, [])

  const resetAllBindings = useCallback(() => {
    setBindings(createInitialBindings())
  }, [])

  return {
    commands,
    conflictCommandIds,
    conflictCount: conflictCommandIds.size,
    customizedCommandIds,
    setBinding,
    resetBinding,
    resetAllBindings,
  }
}
