import {
  createContext,
  use,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  comboFromKeyboardEvent,
  isModifierKey,
  selectActiveComboMatch,
  selectActiveSequenceNode,
  type KeybindingConflict,
  type KeybindingSequenceNode,
} from './KeybindingResolver.js'
import { useCommands, useInvokeCommand } from './CommandContext.js'
import { useFocusContextResolver } from './FocusContext.js'
import { useKeybindingGraph } from './useKeybindingGraph.js'

export type KeyDispatcherMode = 'idle' | 'prefix'

export interface KeyDispatcherContinuation {
  key: string
  remainingKeys?: string[]
  label?: string
  commandId?: string
  conflict?: boolean
}

export interface KeyDispatcherPrefixState {
  mode: KeyDispatcherMode
  activePath: string[]
  continuations: KeyDispatcherContinuation[]
  conflicts: KeybindingConflict[]
  query?: string
  selectedIndex?: number
  whichKeyDelayMs: number
}

interface PrefixSession {
  activePath: string[]
  node: KeybindingSequenceNode
  query: string
  selectedIndex: number
  continuations: KeyDispatcherContinuation[]
}

const idlePrefixState: KeyDispatcherPrefixState = {
  mode: 'idle',
  activePath: [],
  continuations: [],
  conflicts: [],
  query: '',
  selectedIndex: 0,
  whichKeyDelayMs: 0,
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
  const graph = useKeybindingGraph(commands)
  const prefixRef = useRef<PrefixSession | null>(null)

  const clearPrefix = useEffectEvent(() => {
    prefixRef.current = null
    setPrefixState(idlePrefixState)
  })

  const publishPrefix = useEffectEvent(
    (
      activePath: string[],
      node: KeybindingSequenceNode,
      query = '',
      selectedIndex = 0,
    ) => {
      const continuations = continuationsFromNode(node).filter((continuation) =>
        continuationMatchesQuery(continuation, query),
      )
      const nextSelectedIndex =
        continuations.length === 0
          ? 0
          : Math.min(selectedIndex, continuations.length - 1)
      prefixRef.current = {
        activePath,
        node,
        query,
        selectedIndex: nextSelectedIndex,
        continuations,
      }
      setPrefixState({
        mode: 'prefix',
        activePath,
        continuations,
        conflicts: node.conflicts,
        query,
        selectedIndex: nextSelectedIndex,
        whichKeyDelayMs: graph.whichKeyDelayMs,
      })
    },
  )

  const handler = useEffectEvent((event: KeyboardEvent) => {
    if (document.documentElement.dataset.keybindingRecording === 'true') {
      if (prefixRef.current) clearPrefix()
      return
    }
    if (isModifierKey(event)) return
    const combo = comboFromKeyboardEvent(event)
    const prefix = prefixRef.current
    if (prefix) {
      event.preventDefault()
      if (event.key === 'Escape') {
        clearPrefix()
        return
      }
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        if (prefix.continuations.length === 0) return
        const offset = event.key === 'ArrowDown' ? 1 : -1
        const selectedIndex =
          (prefix.selectedIndex + offset + prefix.continuations.length) %
          prefix.continuations.length
        publishPrefix(
          prefix.activePath,
          prefix.node,
          prefix.query,
          selectedIndex,
        )
        return
      }
      if (event.key === 'Enter') {
        const commandId = prefix.continuations[prefix.selectedIndex]?.commandId
        if (!commandId) return
        clearPrefix()
        invokeCommand(commandId)
        return
      }
      if (event.key === 'Backspace' && prefix.query) {
        publishPrefix(prefix.activePath, prefix.node, prefix.query.slice(0, -1))
        return
      }
      if (prefix.query) {
        if (isPrintableKey(event)) {
          publishPrefix(
            prefix.activePath,
            prefix.node,
            prefix.query + event.key,
          )
        }
        return
      }

      const nextNode = prefix.node.children.get(combo)
      if (nextNode) {
        if (nextNode.conflicts.length) {
          publishPrefix([...prefix.activePath, combo], nextNode)
          return
        }
        const binding = nextNode.bindings[0]
        if (binding) {
          clearPrefix()
          invokeCommand(binding.commandId)
          return
        }
        publishPrefix([...prefix.activePath, combo], nextNode)
        return
      }
      if (isPrintableKey(event)) {
        publishPrefix(prefix.activePath, prefix.node, event.key)
        return
      }
      clearPrefix()
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
      publishPrefix([combo], prefixNode)
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
  remainingKeys: string[] = [],
  continuations: KeyDispatcherContinuation[] = [],
): KeyDispatcherContinuation[] {
  const bindings =
    node.bindings.length > 0
      ? node.bindings
      : node.conflicts.flatMap((conflict) => conflict.bindings)
  for (const binding of bindings) {
    continuations.push({
      key: remainingKeys.join(' '),
      remainingKeys,
      label: binding.label,
      commandId: binding.commandId,
      conflict: node.conflicts.length > 0,
    })
  }
  for (const [key, child] of node.children) {
    continuationsFromNode(child, [...remainingKeys, key], continuations)
  }
  return continuations
}

function continuationMatchesQuery(
  continuation: KeyDispatcherContinuation,
  query: string,
): boolean {
  const normalizedQuery = query.toLowerCase().replace(/[^a-z0-9]+/g, '')
  if (!normalizedQuery) return true
  const text = `${continuation.label} ${continuation.commandId}`
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '')
  let queryIndex = 0
  for (let textIndex = 0; textIndex < text.length; textIndex++) {
    if (text[textIndex] !== normalizedQuery[queryIndex]) continue
    queryIndex++
    if (queryIndex === normalizedQuery.length) return true
  }
  return false
}

function isPrintableKey(event: KeyboardEvent): boolean {
  return (
    event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey
  )
}
