import { createContext } from 'react'
import { TreeNode } from './TreeNode.js'

export interface TreeState {
  selectedIds: Set<string>
  focusedId?: string
  lastSelectedId?: string
  expandedIds: Set<string>
}

export type SelectNodeAction = {
  type: 'SELECT_NODE'
  id?: string
  offset?: number
  range?: boolean
  toggle?: boolean
  focus?: boolean
}

export type TreeAction =
  | SelectNodeAction
  | { type: 'TOGGLE_EXPAND'; id: string }

export type TreeDispatch = (action: TreeAction) => void

export function findNodeById<T>(
  nodes: TreeNode<T>[],
  targetId: string,
): TreeNode<T> | undefined {
  if (!targetId) return undefined

  function search<T>(node: TreeNode<T>): TreeNode<T> | undefined {
    if (node.id === targetId) return node
    if (!node.children) return undefined

    for (const child of node.children) {
      const found = search(child)
      if (found) return found
    }
    return undefined
  }

  for (const node of nodes) {
    const found = search<T>(node)
    if (found) return found
  }
  return undefined
}

export function findParentNode<T>(
  nodes: TreeNode<T>[],
  targetId: string,
): TreeNode<T> | undefined {
  function search<T>(node: TreeNode<T>): TreeNode<T> | undefined {
    if (node.children) {
      for (const child of node.children) {
        if (child.id === targetId) {
          return node
        }
        const found = search(child)
        if (found) return found
      }
    }
    return undefined
  }

  for (const node of nodes) {
    const found = search<T>(node)
    if (found) return found
  }
  return undefined
}

export function getVisibleNodes<T = void>(
  nodes: TreeNode<T>[],
  expandedIds: Set<string>,
): TreeNode<T>[] {
  const visible: TreeNode<T>[] = []

  function traverse(node: TreeNode<T>) {
    visible.push(node)
    if (node.children && expandedIds.has(node.id)) {
      node.children.forEach(traverse)
    }
  }

  nodes.forEach(traverse)
  return visible
}

export function treeReducer<T>(
  nodes: TreeNode<T>[],
  state: TreeState,
  action: TreeAction,
): TreeState {
  switch (action.type) {
    case 'SELECT_NODE': {
      const visibleNodes = getVisibleNodes(nodes, state.expandedIds)

      let targetId = action.id
      if (!targetId && action.offset !== undefined) {
        const currentIndex =
          state.focusedId ?
            visibleNodes.findIndex((n) => n.id === state.focusedId)
          : state.lastSelectedId ?
            visibleNodes.findIndex((n) => n.id === state.lastSelectedId)
          : -1

        const nextIndex = Math.max(
          0,
          Math.min(currentIndex + action.offset, visibleNodes.length - 1),
        )

        if (nextIndex >= 0 && nextIndex < visibleNodes.length) {
          targetId = visibleNodes[nextIndex].id
        }
      }
      if (!targetId && visibleNodes.length > 0) {
        targetId = visibleNodes[0].id
      }
      if (!targetId) return state

      if (action.focus) {
        return {
          ...state,
          focusedId: targetId,
        }
      }

      let newSelectedIds: Set<string>

      if (action.range && state.lastSelectedId) {
        const startIdx = visibleNodes.findIndex(
          (n) => n.id === state.lastSelectedId,
        )
        const endIdx = visibleNodes.findIndex((n) => n.id === targetId)
        if (startIdx === -1 || endIdx === -1) return state

        newSelectedIds = new Set(state.selectedIds)

        const [start, end] =
          startIdx < endIdx ? [startIdx, endIdx] : [endIdx, startIdx]
        for (let i = start; i <= end; i++) {
          newSelectedIds.add(visibleNodes[i].id)
        }
      } else {
        newSelectedIds = new Set(action.toggle ? state.selectedIds : [])

        if (action.toggle) {
          if (newSelectedIds.has(targetId)) {
            newSelectedIds.delete(targetId)
          } else {
            newSelectedIds.add(targetId)
          }
        } else {
          newSelectedIds.add(targetId)
        }
      }

      return {
        ...state,
        selectedIds: newSelectedIds,
        focusedId: targetId,
        lastSelectedId: action.focus ? state.lastSelectedId : targetId,
      }
    }

    case 'TOGGLE_EXPAND': {
      const newExpanded = new Set(state.expandedIds)
      const isExpanding = !newExpanded.has(action.id)

      if (isExpanding) {
        newExpanded.add(action.id)
        return {
          ...state,
          expandedIds: newExpanded,
        }
      }

      newExpanded.delete(action.id)

      const newSelectedIds = new Set<string>()
      newSelectedIds.add(action.id)

      return {
        ...state,
        expandedIds: newExpanded,
        focusedId: action.id,
        selectedIds: newSelectedIds,
        lastSelectedId: action.id,
      }
    }

    default:
      return state
  }
}

// TreeStateContext provides state to row components.
export const TreeStateContext = createContext<TreeState | null>(null)

// TreeDispatchContext provides dispatch to row components.
export const TreeDispatchContext = createContext<TreeDispatch | null>(null)
