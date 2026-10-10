import { useCallback, use } from 'react'
import { LuChevronDown, LuChevronRight } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { isExpandable, TreeNode } from './TreeNode.js'
import { TreeStateContext, TreeDispatchContext } from './TreeState.js'

interface TreeRowProps<T = void> {
  node: TreeNode<T>
  level: number
  index: number
  onRowDefaultAction?: (nodes: TreeNode<T>[]) => void
  onRowContextMenu?: (node: TreeNode<T>, event: React.MouseEvent) => void
}

const levelIndentPx = 12

// treeRowLabel describes a row for assistive technology.
function treeRowLabel<T>(
  node: TreeNode<T>,
  hasChildren: boolean,
  isExpanded: boolean,
): string {
  let label = node.name
  if (node.detail) label += `, ${node.detail}`
  if (!hasChildren) return label

  label += `, ${isExpanded ? 'expanded' : 'collapsed'}`
  if (node.children) label += `, ${node.children.length} items`
  return label
}

// TreeIndentGuides draws one vertical guide per ancestor level.
function TreeIndentGuides({ level }: { level: number }) {
  if (level === 0) return null

  return (
    <div className="pointer-events-none absolute top-0 left-0 h-full">
      {Array.from({ length: level }).map((_, i) => (
        <div
          key={i}
          className="border-foreground/6 tree-guide-position absolute top-0 bottom-0 border-l"
          style={{
            '--tree-guide-left': `${(i + 1) * levelIndentPx + 1.5}px`,
          }}
        />
      ))}
    </div>
  )
}

// TreeRowToggle renders the expand button, or a spacer for a leaf.
function TreeRowToggle<T>({
  node,
  hasChildren,
  isExpanded,
  onToggle,
}: {
  node: TreeNode<T>
  hasChildren: boolean
  isExpanded: boolean
  onToggle: (e: React.MouseEvent) => void
}) {
  if (!hasChildren) return <span className="w-4 flex-shrink-0" />

  const Chevron = isExpanded ? LuChevronDown : LuChevronRight

  return (
    <button
      type="button"
      className="hover:bg-foreground/5 rounded p-0.5 transition-colors"
      onClick={onToggle}
      tabIndex={-1}
      aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${node.name}`}
      aria-controls={node.children
        ?.map((child: TreeNode<T>) => child.id)
        .join(' ')}
    >
      <Chevron className="size-4" aria-hidden="true" />
    </button>
  )
}

// TreeRowContent renders the icon, name, detail, and action icons of a row.
function TreeRowContent<T>({ node }: { node: TreeNode<T> }) {
  return (
    <>
      {node.icon && (
        <span className="flex size-4 flex-shrink-0 items-center justify-center">
          {node.icon}
        </span>
      )}
      <span className="text-foreground min-w-0 flex-1 truncate">
        {node.name}
      </span>
      {node.detail && (
        <span
          className="text-foreground-alt/50 micro-text ml-2 max-w-[42%] min-w-0 shrink truncate"
          title={node.detail}
        >
          {node.detail}
        </span>
      )}
      {node.icons && (
        <div className="ml-2 flex shrink-0 items-center gap-1">
          {node.icons.map((iconData, iconIndex) => (
            <button
              type="button"
              key={iconIndex}
              className="text-foreground-alt hover:bg-foreground/5 hover:text-foreground relative rounded p-0.5 transition-colors [&>svg]:h-3 [&>svg]:w-3"
              onClick={(e) => {
                e.stopPropagation()
                iconData.onClick?.(e)
              }}
              title={iconData.tooltip}
              aria-label={iconData.tooltip}
              tabIndex={-1}
            >
              {iconData.icon}
            </button>
          ))}
        </div>
      )}
    </>
  )
}

// useTreeRowHandlers binds the selection, focus, and drag handlers of a row.
function useTreeRowHandlers<T>(
  node: TreeNode<T>,
  isSelected: boolean,
  isFocused: boolean,
  onRowDefaultAction: TreeRowProps<T>['onRowDefaultAction'],
  onRowContextMenu: TreeRowProps<T>['onRowContextMenu'],
) {
  const state = use(TreeStateContext)
  const dispatch = use(TreeDispatchContext)

  const handleTreeItemSelect = useCallback(
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

  const handleTreeItemFocus = useCallback(
    (e: React.FocusEvent) => {
      if (e.target === e.currentTarget && !isFocused) {
        dispatch?.({ type: 'SELECT_NODE', id: node.id, focus: true })
      }
    },
    [dispatch, isFocused, node.id],
  )

  const handleTreeItemKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    e.currentTarget.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true }),
    )
  }, [])

  // The focused row claims DOM focus only from the page body or from within
  // its tree, so rows that mount late never pull focus out of a dialog input.
  const handleRef = useCallback(
    (el: HTMLDivElement | null) => {
      if (isFocused && el && document.activeElement !== el) {
        requestAnimationFrame(() => {
          const active = document.activeElement
          const tree = el.closest('[role="tree"]')
          if (active && active !== document.body && !tree?.contains(active)) {
            return
          }
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

  return {
    handleTreeItemSelect,
    handleDoubleClick,
    handleContextMenu,
    handleToggle,
    handleTreeItemFocus,
    handleTreeItemKeyDown,
    handleRef,
    onDragStart: nodeOnDragStart ? onDragStart : undefined,
  }
}

export function TreeRow<T = void>({
  node,
  level,
  onRowDefaultAction,
  onRowContextMenu,
  index,
}: TreeRowProps<T>) {
  const state = use(TreeStateContext)

  const hasChildren = isExpandable(node)
  const isExpanded = hasChildren && (state?.expandedIds.has(node.id) ?? false)
  const isSelected = state?.selectedIds.has(node.id) ?? false
  const isFocused = state?.focusedId === node.id
  const handlers = useTreeRowHandlers(
    node,
    isSelected,
    isFocused,
    onRowDefaultAction,
    onRowContextMenu,
  )

  return (
    <div
      className={cn(
        'tree-row-indent relative flex cursor-pointer items-center gap-1 py-1 text-xs select-none',
        'border-foreground/6 border-b outline-none transition-colors',
        'focus-visible:ring-brand/30 focus-visible:ring-1 focus-visible:ring-inset',
        isSelected &&
          'bg-brand/10 before:bg-brand/60 before:absolute before:inset-y-1 before:left-0 before:w-0.5 before:rounded-full',
        !isSelected && 'hover:bg-background-card/50',
        !isSelected && index % 2 === 1 && 'bg-background-card/20',
      )}
      draggable={!!node.onDragStart}
      onDragStart={handlers.onDragStart}
      style={{
        '--tree-row-padding-left': `${level * levelIndentPx + 4}px`,
        '--tree-row-padding-right': '8px',
        '--tree-row-min-height': '28px',
      }}
      onClick={handlers.handleTreeItemSelect}
      onKeyDown={handlers.handleTreeItemKeyDown}
      onDoubleClick={handlers.handleDoubleClick}
      onContextMenu={handlers.handleContextMenu}
      role="treeitem"
      aria-selected={isSelected}
      aria-expanded={hasChildren ? isExpanded : undefined}
      aria-level={level + 1}
      aria-current={isFocused ? 'true' : undefined}
      aria-setsize={node.children?.length}
      aria-posinset={level + 1}
      aria-label={treeRowLabel(node, hasChildren, isExpanded)}
      aria-description={`Level ${level + 1}${isSelected ? ', selected' : ''}${isFocused ? ', focused' : ''}`}
      tabIndex={isFocused ? 0 : -1}
      onFocus={handlers.handleTreeItemFocus}
      ref={handlers.handleRef}
      data-autofocus={isFocused || undefined}
      data-focused={isFocused || undefined}
      id={node.id}
    >
      <TreeIndentGuides level={level} />
      <TreeRowToggle
        node={node}
        hasChildren={hasChildren}
        isExpanded={isExpanded}
        onToggle={handlers.handleToggle}
      />
      <TreeRowContent node={node} />
    </div>
  )
}
