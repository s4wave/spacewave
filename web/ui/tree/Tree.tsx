import { useCallback, useEffect, useMemo, useRef } from 'react'
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
import { TreeRow } from './TreeRow.js'
import { cn } from '@s4wave/web/style/utils.js'

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

// withDefaultFocus focuses the first node while no node has focus.
function withDefaultFocus(state: TreeState, firstId?: string): TreeState {
  if (state.focusedId != null || firstId == null) return state
  return {
    ...state,
    focusedId: firstId,
    lastSelectedId: state.lastSelectedId ?? firstId,
  }
}

// Tree renders a hierarchical tree with keyboard navigation and selection.
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

  // Track nodes for the reducer - update via effect to avoid ref update during render
  const nodesRef = useRef<TreeNode<T>[]>(nodes)
  useEffect(() => {
    nodesRef.current = nodes
  }, [nodes])

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

  const flattenedRows = useMemo(() => {
    const renderNode = (
      node: TreeNode<T>,
      level: number,
      index: number,
    ): React.ReactNode[] => [
      <TreeRow
        key={node.id}
        node={node}
        level={level}
        index={index}
        onRowDefaultAction={onRowDefaultAction}
        onRowContextMenu={onRowContextMenu}
      />,
      ...(node.children && state.expandedIds.has(node.id)
        ? node.children.flatMap((child, i) =>
            renderNode(child, level + 1, index + i + 1),
          )
        : []),
    ]

    return nodes.flatMap((node, index) => renderNode(node, 0, index))
  }, [nodes, state.expandedIds, onRowDefaultAction, onRowContextMenu])

  return (
    <TreeStateContext.Provider value={state}>
      <TreeDispatchContext.Provider value={dispatch}>
        <div
          className={cn(
            'group flex flex-1 flex-col overflow-auto outline-none focus:outline-none',
            className,
          )}
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
          <div id="tree-instructions" className="sr-only">
            Use arrow keys or j/k to navigate, Enter to select, and h/l to
            collapse/expand nodes
          </div>
          {nodes.length === 0 ? (
            <div className="text-foreground-alt flex flex-1 items-center justify-center p-4">
              {placeholder ?? 'No items'}
            </div>
          ) : (
            <div>{flattenedRows}</div>
          )}
        </div>
      </TreeDispatchContext.Provider>
    </TreeStateContext.Provider>
  )
}
