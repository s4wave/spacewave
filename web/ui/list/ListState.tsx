import { createContext } from 'react'
import { ListItem } from './ListItem.js'

export type SortDirection = 'asc' | 'desc'

export interface ListState {
  selectedIds?: string[]
  lastSelectedIndex?: number
  focusedIndex?: number
  sortKey?: string
  sortDirection?: SortDirection
}

export type SelectItemAction = {
  type: 'SELECT_ITEM'
  id?: string
  offset?: number
  range?: boolean
  toggle?: boolean
  focus?: boolean
  all?: boolean
}

export type SetSortAction = {
  type: 'SET_SORT'
  sortKey: string
}

export type UpdateIndicesAction = {
  type: 'UPDATE_INDICES'
  focusedIndex?: number
  lastSelectedIndex?: number
}

export type ListAction = SelectItemAction | SetSortAction | UpdateIndicesAction

export type ListDispatch = (action: ListAction) => void

// listReducer processes actions against the current sorted items array.
export function listReducer<T>(
  items: ListItem<T>[],
  state: ListState,
  action: ListAction,
): ListState {
  switch (action.type) {
    case 'SELECT_ITEM':
      return selectItemReducer(items, state, action)
    case 'SET_SORT':
      return setSortReducer(state, action)
    case 'UPDATE_INDICES':
      return updateIndicesReducer(state, action)
    default:
      return state
  }
}

export function setSortReducer(
  state: ListState,
  action: SetSortAction,
): ListState {
  const currentKey = state.sortKey
  const currentDirection = state.sortDirection ?? 'asc'

  if (action.sortKey === currentKey) {
    return {
      ...state,
      sortDirection: currentDirection === 'asc' ? 'desc' : 'asc',
    }
  }

  return {
    ...state,
    sortKey: action.sortKey,
    sortDirection: 'asc',
  }
}

export function updateIndicesReducer(
  state: ListState,
  action: UpdateIndicesAction,
): ListState {
  if (
    state.focusedIndex === action.focusedIndex &&
    state.lastSelectedIndex === action.lastSelectedIndex
  ) {
    return state
  }

  return {
    ...state,
    focusedIndex: action.focusedIndex,
    lastSelectedIndex: action.lastSelectedIndex,
  }
}

export function selectItemReducer<T>(
  items: ListItem<T>[],
  state: ListState,
  action: SelectItemAction,
): ListState {
  if (typeof action.all === 'boolean') {
    if (action.all) {
      return {
        ...state,
        selectedIds: items.map((v) => v.id),
        lastSelectedIndex: state.lastSelectedIndex ?? 0,
        focusedIndex: state.focusedIndex ?? 0,
      }
    }

    const next = { ...state }
    delete next.lastSelectedIndex
    delete next.selectedIds
    return next
  }

  let targetIndex = 0
  if (action.id !== undefined) {
    targetIndex = items.findIndex((item) => item.id === action.id)
  } else {
    const baseIndex = state.focusedIndex ?? state.lastSelectedIndex ?? 0
    targetIndex = baseIndex + (action.offset ?? 0)
  }

  if (targetIndex < 0 || targetIndex >= items.length) {
    return state
  }

  if (action.focus) {
    return {
      ...state,
      focusedIndex: targetIndex,
    }
  }

  let nextSelectedIds = state.selectedIds ?? []
  const nextFocusedIndex = targetIndex
  let nextLastSelectedIndex =
    state.lastSelectedIndex ?? state.focusedIndex ?? targetIndex
  const rangeStartIndex = state.lastSelectedIndex ?? state.focusedIndex

  if (rangeStartIndex !== undefined && action.range) {
    nextSelectedIds = selectRange(
      state.lastSelectedIndex ?? state.focusedIndex ?? 0,
      targetIndex,
      items,
    )
  } else if (action.toggle) {
    nextSelectedIds = toggleSelection(nextSelectedIds, items[targetIndex]?.id)
    nextLastSelectedIndex = targetIndex
  } else {
    const selectItemId = items[targetIndex]?.id
    nextSelectedIds = selectItemId ? [selectItemId] : []
    nextLastSelectedIndex = targetIndex
  }

  return {
    ...state,
    selectedIds: nextSelectedIds,
    lastSelectedIndex: nextLastSelectedIndex,
    focusedIndex: nextFocusedIndex,
  }
}

export function toggleSelection(
  selectedIds: string[] = [],
  id?: string,
): string[] {
  if (!id) return selectedIds
  const index = selectedIds.indexOf(id)
  if (index >= 0) {
    return selectedIds.filter((itemId) => itemId !== id)
  }
  return [...selectedIds, id]
}

export function selectRange<T>(
  startIndex: number,
  endIndex: number,
  items: ListItem<T>[],
): string[] {
  const range = [startIndex, endIndex].sort((a, b) => a - b)
  return items.slice(range[0], range[1] + 1).map((item) => item.id)
}

// translateIndicesToNewOrder finds new indices after sort order changes.
export function translateIndicesToNewOrder<T>(
  state: ListState,
  oldItems: ListItem<T>[],
  newItems: ListItem<T>[],
): { focusedIndex?: number; lastSelectedIndex?: number } | null {
  if (oldItems === newItems || oldItems.length === 0) return null

  let newFocusedIndex: number | undefined = state.focusedIndex
  let newLastSelectedIndex: number | undefined = state.lastSelectedIndex

  if (
    state.focusedIndex !== undefined &&
    state.focusedIndex < oldItems.length
  ) {
    const focusedId = oldItems[state.focusedIndex]?.id
    if (focusedId) {
      const idx = newItems.findIndex((item) => item.id === focusedId)
      newFocusedIndex = idx >= 0 ? idx : undefined
    }
  }

  if (
    state.lastSelectedIndex !== undefined &&
    state.lastSelectedIndex < oldItems.length
  ) {
    const lastSelectedId = oldItems[state.lastSelectedIndex]?.id
    if (lastSelectedId) {
      const idx = newItems.findIndex((item) => item.id === lastSelectedId)
      newLastSelectedIndex = idx >= 0 ? idx : undefined
    }
  }

  if (
    newFocusedIndex === state.focusedIndex &&
    newLastSelectedIndex === state.lastSelectedIndex
  ) {
    return null
  }

  return {
    focusedIndex: newFocusedIndex,
    lastSelectedIndex: newLastSelectedIndex,
  }
}

// ListStateContext provides state to row components.
export const ListStateContext = createContext<ListState | null>(null)

// ListDispatchContext provides dispatch to header/row components.
export const ListDispatchContext = createContext<ListDispatch | null>(null)
