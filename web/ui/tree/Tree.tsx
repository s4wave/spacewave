import { useCallback, useEffect, useRef, useState } from 'react'
import { TreeNode } from './TreeNode.js'
import {
  findNodeById,
  findParentNode,
  getVisibleNodes,
  treeReducer,
  TreeState,
  TreeAction,
  TreeStateContext,
  TreeDispatchContext,
} from './TreeState.js'
import { TreeRow } from './TreeRow.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  useStateReducerAtom,
  StateNamespace,
} from '@s4wave/web/state/persist.js'

export interface TreeProps<T = void> {
  nodes: TreeNode<T>[]
  placeholder?: React.ReactNode
  className?: string
  onRowDefaultAction?: (nodes: TreeNode<T>[]) => void
  onRowContextMenu?: (node: TreeNode<T>, event: React.MouseEvent) => void
  onSelectionChange?: (selectedIds: Set<string>) => void
  defaultExpandedIds?: Set<string>
  defaultSelectedIds?: Set<string>
  namespace?: StateNamespace
  stateKey?: string
  defaultState?: Partial<TreeState>
}

// Tree renders a hierarchical tree with keyboard navigation and selection.
export function Tree<T>({
  nodes,
  placeholder,
  className,
  onRowDefaultAction,
  onRowContextMenu,
  onSelectionChange,
  defaultExpandedIds,
  defaultSelectedIds,
  namespace,
  stateKey = 'tree',
  defaultState,
}: TreeProps<T>) {
  const firstNodeID = nodes[0]?.id

  const [initialState] = useState<TreeState>(() => ({
    expandedIds: defaultExpandedIds ?? new Set<string>(),
    selectedIds: defaultSelectedIds ?? new Set<string>(),
    focusedId: firstNodeID,
    lastSelectedId: firstNodeID,
    ...defaultState,
  }))

  // Track nodes for the reducer - update via effect to avoid ref update during render
  const nodesRef = useRef<TreeNode<T>[]>(nodes)
  useEffect(() => {
    nodesRef.current = nodes
  }, [nodes])

  // Reducer wrapper that uses current nodes
  const reducer = useCallback(
    (state: TreeState, action: TreeAction) =>
      treeReducer<T>(nodesRef.current, state, action),
    [],
  )

  // Use persisted state via useStateReducerAtom
  const [state, dispatch] = useStateReducerAtom<TreeState, TreeAction>(
    namespace ?? null,
    stateKey,
    reducer,
    initialState,
  )

  // Notify when selection changes
  const prevSelectedIdsRef = useRef<Set<string>>(state.selectedIds)
  useEffect(() => {
    if (onSelectionChange && state.selectedIds !== prevSelectedIdsRef.current) {
      prevSelectedIdsRef.current = state.selectedIds
      onSelectionChange(state.selectedIds)
    }
  }, [state.selectedIds, onSelectionChange])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const { key } = e
      const metaKey = e.metaKey || e.ctrlKey
      const shiftKey = e.shiftKey

      const focusedNode =
        state.focusedId ? findNodeById(nodes, state.focusedId) : undefined

      switch (key) {
        case 'Enter': {
          if (focusedNode) {
            dispatch({ type: 'SELECT_NODE', id: focusedNode.id })
            if (onRowDefaultAction) {
              const selectedNodes = Array.from(state.selectedIds)
                .map((id) => findNodeById(nodes, id))
                .filter((node): node is TreeNode<T> => node !== undefined)
              if (selectedNodes.length > 0) {
                onRowDefaultAction(selectedNodes)
              }
            } else if (
              focusedNode.children &&
              focusedNode.children.length > 0
            ) {
              dispatch({ type: 'TOGGLE_EXPAND', id: focusedNode.id })
            }
          }
          break
        }

        case ' ': {
          if (focusedNode) {
            if (focusedNode.children && focusedNode.children.length > 0) {
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
          if (!focusedNode?.children?.length) break

          const firstChild = focusedNode.children[0]
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

  const renderFlattenedRows = useCallback(() => {
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
      ...(node.children && state.expandedIds.has(node.id) ?
        node.children.flatMap((child, i) =>
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
            if (e.target === e.currentTarget) {
              if (state.focusedId) {
                const focusedElement = document.getElementById(state.focusedId)
                if (focusedElement) {
                  e.preventDefault()
                  requestAnimationFrame(() => {
                    focusedElement.focus({ preventScroll: true })
                    focusedElement.scrollIntoView({ block: 'nearest' })
                  })
                }
              } else if (nodes.length > 0) {
                dispatch({ type: 'SELECT_NODE', id: nodes[0].id, focus: true })
              }
            }
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
          {nodes.length === 0 ?
            <div className="text-foreground-alt flex flex-1 items-center justify-center p-4">
              {placeholder ?? 'No items'}
            </div>
          : <div>{renderFlattenedRows()}</div>}
        </div>
      </TreeDispatchContext.Provider>
    </TreeStateContext.Provider>
  )
}
