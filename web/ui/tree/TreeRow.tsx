import { memo, use, useCallback } from 'react'
import * as stylex from '@stylexjs/stylex'
import { stylexProps } from '@s4wave/web/style/props.js'
import { LuChevronDown, LuChevronRight } from '@s4wave/web/images/icons.js'

import { isExpandable, treeRowDomId, TreeNode } from './TreeNode.js'
import { TreeDispatchContext } from './TreeState.js'

import {
  layout as themeLayout,
  motion as themeMotion,
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../style/tokens.stylex.js'

const rowStyles = stylex.create({
  guide: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    position: 'absolute',
    borderLeftStyle: themeUtilityState['--tw-border-style'],
    borderLeftWidth: '1px',
    bottom: 0,
    left: 'var(--tree-guide-left)',
    top: 0,
  },
  action: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: '.25rem',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    position: 'relative',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  row: {
    '--tw-outline-style': 'none',
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    },
    '--tw-ring-inset': { default: null, ':focus-visible': 'inset' },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    WebkitUserSelect: 'none',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: themeLayout['--spacing'],
    paddingBlock: themeLayout['--spacing'],
    alignItems: 'center',
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    position: 'relative',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: '100%',
    minHeight: {
      default: 'var(--tree-row-min-height)',
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        '44px',
    },
    paddingLeft: 'var(--tree-row-padding-left)',
    paddingRight: 'var(--tree-row-padding-right)',
  },
  selected: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    '::before': {
      borderRadius: '3.40282e38px',
      insetBlock: themeLayout['--spacing'],
      backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      content: 'var(--tw-content)',
      position: 'absolute',
      left: 0,
      width: `calc(${themeLayout['--spacing']} * .5)`,
    },
  },
  alternate: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
  },
  idle: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    pointerEvents: 'none',
    position: 'absolute',
    height: '100%',
    left: 0,
    top: 0,
  },
  rule2: { flexShrink: 0, width: `calc(${themeLayout['--spacing']} * 4)` },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: '.25rem',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule7: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 1,
    fontSize: '.6rem',
    lineHeight: 1.5,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
    maxWidth: '42%',
    minWidth: 0,
  },
  rule8: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

/** TreeRowProps describes one rendered tree row and its owned style override. */
export interface TreeRowProps<T = void> {
  sx?: stylex.StyleXStyles
  // domIdPrefix keeps rendered IDs separate from domain IDs and other mounted trees.
  domIdPrefix: string
  node: TreeNode<T>
  // level is the depth of the node, zero at the root.
  level: number
  // index is the position of the row among the visible rows.
  index: number
  isSelected: boolean
  isFocused: boolean
  isExpanded: boolean
  onRowDefaultAction?: (nodes: TreeNode<T>[]) => void
  onRowContextMenu?: (node: TreeNode<T>, event: React.MouseEvent) => void
  onRowDragStart?: (
    event: React.DragEvent<HTMLElement>,
    node: TreeNode<T>,
  ) => void
}

const levelIndentPx = 12

// treeRowLabel describes a row for assistive technology.
function treeRowLabel<T>(
  node: TreeNode<T>,
  hasChildren: boolean,
  isExpanded: boolean,
): string {
  // Include row details before deciding whether expansion information applies.
  let label = node.name
  if (node.detail) label += `, ${node.detail}`
  if (!hasChildren) return label

  // Add expansion state and known child count to an expandable row's label.
  label += `, ${isExpanded ? 'expanded' : 'collapsed'}`
  if (node.children) label += `, ${node.children.length} items`
  return label
}

// TreeIndentGuides draws one vertical guide per ancestor level.
function TreeIndentGuides({ level }: { level: number }) {
  if (level === 0) return null

  return (
    <div sx={stylexStyles.rule1}>
      {Array.from({ length: level }).map((_, i) => (
        <div
          key={i}
          sx={rowStyles.guide}
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
  domIdPrefix,
}: {
  node: TreeNode<T>
  hasChildren: boolean
  isExpanded: boolean
  onToggle: (e: React.MouseEvent) => void
  domIdPrefix: string
}) {
  if (!hasChildren) return <span sx={stylexStyles.rule2} />

  const Chevron = isExpanded ? LuChevronDown : LuChevronRight

  return (
    <button
      type="button"
      sx={[stylexStyles.rule3, rowTargetStyles.touch]}
      onClick={onToggle}
      tabIndex={-1}
      aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${node.name}`}
      aria-controls={node.children
        ?.map((child: TreeNode<T>) => treeRowDomId(domIdPrefix, child.id))
        .join(' ')}
    >
      <Chevron sx={stylexStyles.rule4} aria-hidden="true" />
    </button>
  )
}

// TreeRowContent renders the icon, name, detail, and action icons of a row.
function TreeRowContent<T>({ node }: { node: TreeNode<T> }) {
  return (
    <>
      {node.icon && <span sx={stylexStyles.rule5}>{node.icon}</span>}
      <span sx={stylexStyles.rule6}>{node.name}</span>
      {node.detail && (
        <span sx={stylexStyles.rule7} title={node.detail}>
          {node.detail}
        </span>
      )}
      {node.icons && (
        <div sx={stylexStyles.rule8}>
          {node.icons.map((iconData, iconIndex) => (
            <button
              type="button"
              key={iconIndex}
              sx={[rowStyles.action, rowTargetStyles.touch]}
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
function useTreeRowHandlers<T>({
  node,
  isSelected,
  isFocused,
  onRowDefaultAction,
  onRowContextMenu,
  onRowDragStart,
}: TreeRowProps<T>) {
  // Use the tree's shared dispatcher for all row actions.
  const dispatch = use(TreeDispatchContext)

  // Translate row selection modifiers into the existing tree action.
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

  // Open this row without also activating its parent.
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

  // Select an unselected row before opening its context menu.
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

  // Toggle expansion without changing the surrounding row selection.
  const handleToggle = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      dispatch?.({ type: 'TOGGLE_EXPAND', id: node.id })
    },
    [dispatch, node.id],
  )

  // Update tree focus only when this row itself receives focus.
  const handleTreeItemFocus = useCallback(
    (e: React.FocusEvent) => {
      if (e.target === e.currentTarget && !isFocused) {
        dispatch?.({ type: 'SELECT_NODE', id: node.id, focus: true })
      }
    },
    [dispatch, isFocused, node.id],
  )

  // Use the ordinary row click path for Enter and Space.
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
          // Keep late-mounted rows from stealing focus outside their tree.
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

  // Deliver the current row and drag event through the tree's handler.
  const onDragStart = useCallback(
    (e: React.DragEvent<HTMLElement>) => onRowDragStart?.(e, node),
    [onRowDragStart, node],
  )

  return {
    handleTreeItemSelect,
    handleDoubleClick,
    handleContextMenu,
    handleToggle,
    handleTreeItemFocus,
    handleTreeItemKeyDown,
    handleRef,
    onDragStart: node.onDragStart ? onDragStart : undefined,
  }
}

// TreeRowImpl renders one row from its own selection, focus, and expansion, so
// a change of the tree state renders only the rows it changes.
function TreeRowImpl<T = void>(props: TreeRowProps<T>) {
  // Resolve expansion and bound actions from the current row state.
  const { node, level, index, isSelected, isFocused } = props
  const hasChildren = isExpandable(node)
  const isExpanded = hasChildren && props.isExpanded
  const handlers = useTreeRowHandlers(props)
  const domIdPrefix = props.domIdPrefix

  return (
    <div
      {...stylexProps(
        [
          rowStyles.row,
          isSelected
            ? rowStyles.selected
            : index % 2 === 1
              ? rowStyles.alternate
              : rowStyles.idle,
          props.sx,
        ],
        {
          style: {
            '--tree-row-padding-left': `${level * levelIndentPx + 4}px`,
            '--tree-row-padding-right': '8px',
            '--tree-row-min-height': '28px',
          },
        },
      )}
      draggable={!!node.onDragStart}
      onDragStart={handlers.onDragStart}

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
      id={treeRowDomId(domIdPrefix, node.id)}
    >
      <TreeIndentGuides level={level} />
      <TreeRowToggle
        node={node}
        hasChildren={hasChildren}
        isExpanded={isExpanded}
        onToggle={handlers.handleToggle}
        domIdPrefix={domIdPrefix}
      />
      <TreeRowContent node={node} />
    </div>
  )
}

// TreeRow renders one tree row, skipping renders while its props are unchanged.
export const TreeRow = memo(TreeRowImpl) as typeof TreeRowImpl

const rowTargetStyles = stylex.create({
  touch: {
    alignItems: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        'center',
    },
    display: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        'inline-flex',
    },
    justifyContent: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        'center',
    },
    minHeight: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)': 44,
    },
    minWidth: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)': 44,
    },
  },
})
