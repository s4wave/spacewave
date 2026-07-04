import {
  createContext,
  use,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  comboFromKeyboardEvent,
  isModifierKey,
  resolveKeybindings,
  selectActiveComboMatch,
  selectActiveSequenceNode,
  type KeybindingConflict,
  type KeybindingSequenceNode,
} from './KeybindingResolver.js'
import { useCommands, useInvokeCommand } from './CommandContext.js'
import { useFocusContextResolver } from './FocusContext.js'

export type KeyDispatcherMode = 'idle' | 'prefix'

export interface KeyDispatcherContinuation {
  key: string
  label?: string
  commandId?: string
  conflict?: boolean
}

export interface KeyDispatcherPrefixState {
  mode: KeyDispatcherMode
  activePath: string[]
  continuations: KeyDispatcherContinuation[]
  conflicts: KeybindingConflict[]
}

interface PrefixSession {
  activePath: string[]
  node: KeybindingSequenceNode
}

const idlePrefixState: KeyDispatcherPrefixState = {
  mode: 'idle',
  activePath: [],
  continuations: [],
  conflicts: [],
}

const KeyDispatcherContext =
  createContext<KeyDispatcherPrefixState>(idlePrefixState)

export function useKeyDispatcherState(): KeyDispatcherPrefixState {
  return use(KeyDispatcherContext)
}

export function KeyDispatcher({ children }: { children?: ReactNode }) {
  const commands = useCommands()
  const invokeCommand = useInvokeCommand()
  const resolveFocusContexts = useFocusContextResolver()
  const [prefixState, setPrefixState] =
    useState<KeyDispatcherPrefixState>(idlePrefixState)
  const graph = useMemo(() => resolveKeybindings(commands), [commands])
  const prefixRef = useRef<PrefixSession | null>(null)

  const clearPrefix = useEffectEvent(() => {
    prefixRef.current = null
    setPrefixState(idlePrefixState)
  })

  const beginPrefix = useEffectEvent(
    (activePath: string[], node: KeybindingSequenceNode) => {
      prefixRef.current = { activePath, node }
      setPrefixState({
        mode: 'prefix',
        activePath,
        continuations: continuationsFromNode(node),
        conflicts: node.conflicts,
      })
    },
  )

  const handler = useEffectEvent((event: KeyboardEvent) => {
    if (isModifierKey(event)) return
    const combo = comboFromKeyboardEvent(event)
    const prefix = prefixRef.current
    if (prefix) {
      event.preventDefault()
      if (event.key === 'Escape') {
        clearPrefix()
        return
      }
      const nextNode = prefix.node.children.get(combo)
      if (!nextNode) {
        clearPrefix()
        return
      }
      if (nextNode.conflicts.length) {
        beginPrefix([...prefix.activePath, combo], nextNode)
        return
      }
      const binding = nextNode.bindings[0]
      if (binding) {
        clearPrefix()
        invokeCommand(binding.commandId)
        return
      }
      beginPrefix([...prefix.activePath, combo], nextNode)
      return
    }

    const activeFocusContexts = resolveFocusContexts(event.target)
    const comboMatch = selectActiveComboMatch(graph, activeFocusContexts, combo)
    if (comboMatch?.conflict) {
      event.preventDefault()
      return
    }
    if (comboMatch?.binding) {
      event.preventDefault()
      invokeCommand(comboMatch.binding.commandId)
      return
    }
    const sequenceNode = graph.sequenceTrie.children.get(combo)
    const prefixNode = sequenceNode
      ? selectActiveSequenceNode(sequenceNode, activeFocusContexts)
      : undefined
    if (prefixNode) {
      event.preventDefault()
      beginPrefix([combo], prefixNode)
    }
  })

  useEffect(() => {
    document.addEventListener('keydown', handler, true)
    window.addEventListener('blur', clearPrefix)
    return () => {
      document.removeEventListener('keydown', handler, true)
      window.removeEventListener('blur', clearPrefix)
    }
  }, [])

  return (
    <KeyDispatcherContext.Provider value={prefixState}>
      {children ?? null}
    </KeyDispatcherContext.Provider>
  )
}

function continuationsFromNode(
  node: KeybindingSequenceNode,
): KeyDispatcherContinuation[] {
  const continuations: KeyDispatcherContinuation[] = []
  for (const [key, child] of node.children) {
    const binding = child.bindings[0]
    continuations.push({
      key,
      label: binding?.label,
      commandId: binding?.commandId,
      conflict: child.conflicts.length > 0,
    })
  }
  return continuations
}
