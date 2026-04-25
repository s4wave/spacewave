import { useCallback, useContext } from 'react'
import { LuChevronDown, LuChevronRight } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { TreeNode } from './TreeNode.js'
import { TreeStateContext, TreeDispatchContext } from './TreeState.js'

interface TreeRowProps<T = void> {
  node: TreeNode<T>
  level: number
  index: number
  onRowDefaultAction?: (nodes: TreeNode<T>[]) => void
  onRowContextMenu?: (node: TreeNode<T>, event: React.MouseEvent) => void
}

const levelIndentPx = 12

export function TreeRow<T = void>({
  node,
  level,
  onRowDefaultAction,
  onRowContextMenu,
  index,
}: TreeRowProps<T>) {
  const state = useContext(TreeStateContext)
  const dispatch = useContext(TreeDispatchContext)

  const hasChildren = node.children && node.children.length > 0
  const isExpanded = hasChildren && (state?.expandedIds.has(node.id) ?? false)
  const isSelected = state?.selectedIds.has(node.id) ?? false
  const isFocused = state?.focusedId === node.id

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      dispatch?.({
        type: 'SELECT_NODE',
        id: node.id,
        range: e.shiftKey,
        toggle: e.ctrlKey || e.metaKey || isSelected,
      })
    },
    [dispatch, node.id, isSelected],
  )

  const handleDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (onRowDefaultAction) {
        onRowDefaultAction([node])
      }
    },
    [node, onRowDefaultAction],
  )

  const handleContextMenu = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      if (!isSelected) {
        dispatch?.({ type: 'SELECT_NODE', id: node.id })
      }
      onRowContextMenu?.(node, e)
    },
    [dispatch, isSelected, node, onRowContextMenu],
  )

  const handleToggle = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      dispatch?.({ type: 'TOGGLE_EXPAND', id: node.id })
    },
    [dispatch, node.id],
  )

  const handleFocus = useCallback(
    (e: React.FocusEvent) => {
      if (e.target === e.currentTarget && !isFocused) {
        dispatch?.({ type: 'SELECT_NODE', id: node.id, focus: true })
      }
    },
    [dispatch, isFocused, node.id],
  )

  const handleRef = useCallback(
    (el: HTMLDivElement | null) => {
      if (isFocused && el && document.activeElement !== el) {
        requestAnimationFrame(() => {
          el.focus({ preventScroll: true })
          el.scrollIntoView({ block: 'nearest' })
        })
      }
    },
    [isFocused],
  )

  const nodeOnDragStart = node.onDragStart
  const onDragStart = useCallback(
    (e: React.DragEvent<HTMLElement>) => {
      if (nodeOnDragStart && state) {
        const wasSelected = state.selectedIds.has(node.id)
        if (!wasSelected) {
          dispatch?.({ type: 'SELECT_NODE', id: node.id })
        }
        state.selectedIds.add(node.id)
        nodeOnDragStart(e, node, state)
      }
    },
    [nodeOnDragStart, node, state, dispatch],
  )

  return (
    <div
      className={cn(
        'relative flex cursor-pointer items-center text-xs select-none',
        'border-ui-outline border-b transition-colors',
        isSelected && 'bg-ui-selected',
        !isSelected && 'hover:bg-outliner-selected-highlight',
        !isSelected && index % 2 === 1 && 'bg-file-row-alternate',
      )}
      draggable={!!nodeOnDragStart}
      onDragStart={nodeOnDragStart ? onDragStart : undefined}
      style={{
        paddingLeft: `${level * levelIndentPx + 4}px`,
        paddingRight: '4px',
        height: '20px',
      }}
      onClick={handleClick}
      onDoubleClick={handleDoubleClick}
      onContextMenu={handleContextMenu}
      role="treeitem"
      aria-selected={isSelected}
      aria-expanded={hasChildren ? isExpanded : undefined}
      aria-level={level + 1}
      aria-current={isFocused ? 'true' : undefined}
      aria-setsize={node.children?.length}
      aria-posinset={level + 1}
      aria-label={`${node.name}${hasChildren ? `, ${isExpanded ? 'expanded' : 'collapsed'}, ${node.children?.length} items` : ''}`}
      aria-description={`Level ${level + 1}${isSelected ? ', selected' : ''}${isFocused ? ', focused' : ''}`}
      tabIndex={isFocused ? 0 : -1}
      onFocus={handleFocus}
      ref={handleRef}
      data-autofocus={isFocused || undefined}
      data-focused={isFocused || undefined}
      id={node.id}
    >
      {level > 0 && (
        <div className="pointer-events-none absolute top-0 left-0 h-full">
          {Array.from({ length: level }).map((_, i) => (
            <div
              key={i}
              className="border-tree-indent-line absolute top-0 bottom-0 border-l"
              style={{ left: `${(i + 1) * levelIndentPx + 1.5}px` }}
            />
          ))}
        </div>
      )}
      {hasChildren ?
        <button
          className="hover:bg-menu-hover rounded p-[2px]"
          onClick={handleToggle}
          tabIndex={-1}
          aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${node.name}`}
          aria-controls={node.children
            ?.map((child: TreeNode<T>) => child.id)
            .join(' ')}
        >
          {isExpanded ?
            <LuChevronDown className="h-4 w-4" aria-hidden="true" />
          : <LuChevronRight className="h-4 w-4" aria-hidden="true" />}
        </button>
      : <span className="w-4 flex-shrink-0" />}
      {node.icon && (
        <span className="flex h-4 w-4 flex-shrink-0 items-center justify-center">
          {node.icon}
        </span>
      )}
      <span className="ml-1 flex-1 truncate">{node.name}</span>
      {node.icons && (
        <div className="mr-2 flex items-center gap-1">
          {node.icons.map((iconData, iconIndex) => (
            <button
              key={iconIndex}
              className="text-foreground-alt hover:bg-menu-hover hover:text-foreground relative rounded p-[2px] [&>svg]:h-3 [&>svg]:w-3"
              onClick={(e) => {
                e.stopPropagation()
                iconData.onClick?.(e)
              }}
              title={iconData.tooltip}
              tabIndex={-1}
            >
              {iconData.icon}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
