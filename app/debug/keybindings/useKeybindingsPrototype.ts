import { useCallback, useMemo, useState } from 'react'

import {
  MOCK_KEYBINDING_COMMANDS,
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
    const commandsByBinding = new Map<string, KeybindingCommand[]>()

    for (const command of commands) {
      if (!command.binding) continue
      const bindingKey = `${command.context}:${chordComparisonKey(command.binding)}`
      const matches = commandsByBinding.get(bindingKey)
      if (matches) matches.push(command)
      else commandsByBinding.set(bindingKey, [command])
    }

    const conflicts = new Set<string>()
    for (const matches of commandsByBinding.values()) {
      if (matches.length < 2) continue
      for (const command of matches) conflicts.add(command.id)
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
