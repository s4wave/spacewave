import { useCallback, useEffect, useMemo, useRef } from 'react'
import { List, useListRef, type RowComponentProps } from 'react-window'
import {
  coarsePointerQuery,
  useMediaQuery,
} from '@s4wave/web/hooks/useMediaQuery.js'
import { isExpandable, TreeNode } from './TreeNode.js'
import {
  findNodeById,
  findParentNode,
  getVisibleNodes,
  treeReducer,
  TreeAction,
  TreeState,
  TreeStateHandle,
  TreeStateContext,
  TreeDispatchContext,
} from './TreeState.js'
import { TreeRow, type TreeRowProps } from './TreeRow.js'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule2: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: { minHeight: 0 },
})

export interface TreeProps<T = void> {
  nodes: TreeNode<T>[]
  // state is the expansion, selection, and focus the Tree shows and changes,
  // from useTreeState.
  state: TreeStateHandle
  placeholder?: React.ReactNode
  className?: string
  onRowDefaultAction?: (nodes: TreeNode<T>[]) => void
  onRowContextMenu?: (node: TreeNode<T>, event: React.MouseEvent) => void
}

// treeRowHeight and coarseTreeRowHeight are the heights of a row slot for a
// fine and a coarse pointer: the expand button, the row padding, and the bottom
// border. The coarse button keeps the 44px target of web/style/app.css.
const treeRowHeight = 29
const coarseTreeRowHeight = 53

// TreeRowEntry is a visible node and its depth.
interface TreeRowEntry<T> {
  node: TreeNode<T>
  level: number
}

// flattenTree returns the visible nodes in display order.
function flattenTree<T>(
  nodes: TreeNode<T>[],
  expandedIds: Set<string>,
): TreeRowEntry<T>[] {
  const rows: TreeRowEntry<T>[] = []
  const visit = (node: TreeNode<T>, level: number) => {
    rows.push({ node, level })
    if (node.children && expandedIds.has(node.id)) {
      for (const child of node.children) visit(child, level + 1)
    }
  }
  for (const node of nodes) visit(node, 0)
  return rows
}

// TreeListRowProps are the props the list passes to every rendered row.
type TreeListRowProps<T> = Pick<
  TreeRowProps<T>,
  'onRowDefaultAction' | 'onRowContextMenu' | 'onRowDragStart'
> & {
  rows: TreeRowEntry<T>[]
  state: TreeState
}

// TreeListRow renders the row at index inside the list's positioned slot.
function TreeListRow<T>({
  index,
  style,
  rows,
  state,
  onRowDefaultAction,
  onRowContextMenu,
  onRowDragStart,
}: RowComponentProps<TreeListRowProps<T>>) {
  const { node, level } = rows[index]
  return (
    <div role="none" style={style}>
      <TreeRow
        node={node}
        level={level}
        index={index}
        isSelected={state.selectedIds.has(node.id)}
        isFocused={state.focusedId === node.id}
        isExpanded={state.expandedIds.has(node.id)}
        onRowDefaultAction={onRowDefaultAction}
        onRowContextMenu={onRowContextMenu}
        onRowDragStart={onRowDragStart}
      />
    </div>
  )
}

// withDefaultFocus focuses the first node while no node has focus.
function withDefaultFocus(state: TreeState, firstId?: string): TreeState {
  if (state.focusedId != null || firstId == null) return state
  return {
    ...state,
    focusedId: firstId,
    lastSelectedId: state.lastSelectedId ?? firstId,
  }
}

// Tree renders a hierarchical tree with keyboard navigation and selection. It
// renders only the rows near the visible part of the tree.
export function Tree<T>({
  nodes,
  state: [storedState, update],
  placeholder,
  className,
  onRowDefaultAction,
  onRowContextMenu,
}: TreeProps<T>) {
  const firstNodeID = nodes[0]?.id
  const state = useMemo(
    () => withDefaultFocus(storedState, firstNodeID),
    [storedState, firstNodeID],
  )

  const rows = useMemo(
    () => flattenTree(nodes, state.expandedIds),
    [nodes, state.expandedIds],
  )

  // Handlers read the latest nodes, rows, and state through refs, so they stay
  // stable and the rows they reach skip renders.
  const nodesRef = useRef<TreeNode<T>[]>(nodes)
  const rowsRef = useRef(rows)
  const stateRef = useRef(state)
  useEffect(() => {
    nodesRef.current = nodes
    rowsRef.current = rows
    stateRef.current = state
  }, [nodes, rows, state])

  const dispatch = useCallback(
    (action: TreeAction) =>
      update((prev) =>
        treeReducer<T>(
          nodesRef.current,
          withDefaultFocus(prev, nodesRef.current[0]?.id),
          action,
        ),
      ),
    [update],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const { key } = e
      const metaKey = e.metaKey || e.ctrlKey
      const shiftKey = e.shiftKey

      const focusedNode = state.focusedId
        ? findNodeById(nodes, state.focusedId)
        : undefined

      switch (key) {
        case 'Enter': {
          if (focusedNode) {
            dispatch({ type: 'SELECT_NODE', id: focusedNode.id })
            if (onRowDefaultAction) {
              const selectedNodes = Array.from(state.selectedIds).flatMap(
                (id) => {
                  const node = findNodeById(nodes, id)
                  return node ? [node] : []
                },
              )
              if (selectedNodes.length > 0) {
                onRowDefaultAction(selectedNodes)
              }
            } else if (isExpandable(focusedNode)) {
              dispatch({ type: 'TOGGLE_EXPAND', id: focusedNode.id })
            }
          }
          break
        }

        case ' ': {
          if (focusedNode) {
            if (isExpandable(focusedNode)) {
              dispatch({ type: 'TOGGLE_EXPAND', id: focusedNode.id })
            } else {
              dispatch({
                type: 'SELECT_NODE',
                id: focusedNode.id,
                toggle: true,
              })
            }
          }
          break
        }

        case 'Home': {
          if (nodes.length > 0) {
            dispatch({ type: 'SELECT_NODE', id: nodes[0].id, focus: metaKey })
          }
          break
        }

        case 'End': {
          const visibleNodes = getVisibleNodes(nodes, state.expandedIds)
          if (visibleNodes.length > 0) {
            const lastNode = visibleNodes[visibleNodes.length - 1]
            dispatch({ type: 'SELECT_NODE', id: lastNode.id, focus: metaKey })
          }
          break
        }

        case 'k':
        case 'ArrowUp': {
          dispatch({
            type: 'SELECT_NODE',
            offset: -1,
            focus: metaKey,
            range: shiftKey,
          })
          break
        }

        case 'j':
        case 'ArrowDown': {
          dispatch({
            type: 'SELECT_NODE',
            offset: 1,
            focus: metaKey,
            range: shiftKey,
          })
          break
        }

        case 'h':
        case 'ArrowLeft': {
          if (!focusedNode) break

          if (state.expandedIds.has(focusedNode.id)) {
            dispatch({ type: 'TOGGLE_EXPAND', id: focusedNode.id })
          } else {
            const parent = findParentNode(nodes, focusedNode.id)
            if (parent) {
              dispatch({
                type: 'SELECT_NODE',
                id: parent.id,
                focus: metaKey,
              })
            }
          }
          break
        }

        case 'l':
        case 'ArrowRight': {
          if (!focusedNode || !isExpandable(focusedNode)) break

          const firstChild = focusedNode.children?.[0]
          if (!state.expandedIds.has(focusedNode.id)) {
            dispatch({ type: 'TOGGLE_EXPAND', id: focusedNode.id })
            if (firstChild) {
              dispatch({
                type: 'SELECT_NODE',
                id: firstChild.id,
                focus: metaKey,
              })
            }
          } else if (firstChild) {
            dispatch({
              type: 'SELECT_NODE',
              id: firstChild.id,
              focus: metaKey,
            })
          }
          break
        }
        default:
          return
      }

      e.preventDefault()
    },
    [dispatch, state, nodes, onRowDefaultAction],
  )

  // A dragged row that is not selected becomes the selection it drags.
  const handleRowDragStart = useCallback(
    (e: React.DragEvent<HTMLElement>, node: TreeNode<T>) => {
      const current = stateRef.current
      if (!node.onDragStart) return
      if (current.selectedIds.has(node.id)) {
        node.onDragStart(e, node, current)
        return
      }
      dispatch({ type: 'SELECT_NODE', id: node.id })
      node.onDragStart(e, node, {
        ...current,
        selectedIds: new Set([node.id]),
      })
    },
    [dispatch],
  )

  // Scroll the focused row into view when the focus moves, which mounts it.
  const listRef = useListRef(null)
  useEffect(() => {
    const index = rowsRef.current.findIndex(
      (row) => row.node.id === state.focusedId,
    )
    if (index !== -1) {
      listRef.current?.scrollToRow({ index, align: 'smart' })
    }
  }, [listRef, state.focusedId])

  const coarsePointer = useMediaQuery(coarsePointerQuery)
  const rowProps = useMemo(
    (): TreeListRowProps<T> => ({
      rows,
      state,
      onRowDefaultAction,
      onRowContextMenu,
      onRowDragStart: handleRowDragStart,
    }),
    [rows, state, onRowDefaultAction, onRowContextMenu, handleRowDragStart],
  )

  return (
    <TreeStateContext.Provider value={state}>
      <TreeDispatchContext.Provider value={dispatch}>
        <div
          className={
            /* TODO(stylex): global or unknown class: group */ cn(
              'group flex min-h-0 flex-1 flex-col outline-none focus:outline-none',
              className,
            )
          }
          role="tree"
          tabIndex={state.focusedId != null ? -1 : 0}
          aria-label="Tree navigation"
          aria-multiselectable="true"
          aria-orientation="vertical"
          onFocus={(e) => {
            if (e.target !== e.currentTarget || !state.focusedId) return
            const focusedElement = document.getElementById(state.focusedId)
            if (!focusedElement) return
            e.preventDefault()
            requestAnimationFrame(() => {
              focusedElement.focus({ preventScroll: true })
              focusedElement.scrollIntoView({ block: 'nearest' })
            })
          }}
          aria-describedby="tree-instructions"
          aria-activedescendant={
            state.focusedId ? `${state.focusedId}` : undefined
          }
          onKeyDown={handleKeyDown}
        >
          <div id="tree-instructions" sx={stylexStyles.rule1}>
            Use arrow keys or j/k to navigate, Enter to select, and h/l to
            collapse/expand nodes
          </div>
          {nodes.length === 0 ? (
            <div sx={stylexStyles.rule2}>{placeholder ?? 'No items'}</div>
          ) : (
            <List
              role="none"
              {...stylex.props(stylexStyles.rule3)}
              listRef={listRef}
              defaultHeight={600}
              overscanCount={8}
              rowComponent={TreeListRow<T>}
              rowCount={rows.length}
              rowHeight={coarsePointer ? coarseTreeRowHeight : treeRowHeight}
              rowProps={rowProps}
            />
          )}
        </div>
      </TreeDispatchContext.Provider>
    </TreeStateContext.Provider>
  )
}
