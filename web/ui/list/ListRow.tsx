import { useCallback, use, useEffect, useRef } from 'react'

import { ListStateContext } from './ListState.js'
import { RowComponentProps } from './List.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const styles = stylex.create({
  row: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: '1px',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-tight)',
    '--tw-leading': 'var(--leading-tight)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  selected: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  unselected: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 90%, transparent)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  focused: {
    '--tw-ring-shadow':
      'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 25%, transparent)`,
    '--tw-ring-inset': 'inset',
  },
  label: {
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
})

// ListRow renders a basic list row with selection state.
export function ListRow<T>({
  item,
  itemIndex,
  onRowClick,
  onContextMenu,
  style,
  ariaAttributes,
}: RowComponentProps<T>) {
  // Keep click grouping and its timer within this mounted row.
  const clickCountRef = useRef(0)
  const clickTimerRef = useRef<number | null>(null)

  // Report the first click immediately and group subsequent clicks.
  const handleListRowSelect = useCallback(
    (e: React.MouseEvent) => {
      // Report the current row and increment its pending click count.
      const newCount = clickCountRef.current + 1
      clickCountRef.current = newCount
      if (onRowClick) {
        onRowClick(itemIndex, item, e, 1)
      }

      // Replace the previous click window before waiting for additional clicks.
      if (clickTimerRef.current) {
        clearTimeout(clickTimerRef.current)
      }

      // Report a completed multiple click and clear its grouping state.
      clickTimerRef.current = window.setTimeout(() => {
        if (newCount > 1 && onRowClick) {
          onRowClick(itemIndex, item, e, newCount)
        }
        clickCountRef.current = 0
        clickTimerRef.current = null
      }, 300)
    },
    [itemIndex, item, onRowClick],
  )

  // Suppress the browser menu before notifying the row's context-menu handler.
  const handleContextMenu = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      if (onContextMenu) {
        onContextMenu(itemIndex, item, e)
      }
    },
    [itemIndex, item, onContextMenu],
  )

  // Use the same row click path for Enter and Space.
  const handleListRowKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    e.currentTarget.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true }),
    )
  }, [])

  // Resolve this row's selection and focus from the shared list state.
  const context = use(ListStateContext)
  const selected = context?.selectedIds?.includes(item.id) ?? false
  const focused = itemIndex === context?.focusedIndex

  // Move browser focus to the row selected by keyboard navigation.
  const divRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (focused && divRef.current) {
      divRef.current.focus()
    }
  }, [focused])

  return (
    <div
      ref={divRef}
      role="row"
      tabIndex={focused ? 0 : -1}
      aria-selected={selected || undefined}
      aria-posinset={ariaAttributes['aria-posinset']}
      aria-setsize={ariaAttributes['aria-setsize']}
      style={style}
      sx={[
        styles.row,
        selected ? styles.selected : styles.unselected,
        !!(focused && !selected) && styles.focused,
      ]}
      onClick={handleListRowSelect}
      onKeyDown={handleListRowKeyDown}
      onContextMenu={handleContextMenu}
    >
      <span sx={styles.label}>{item.id}</span>
    </div>
  )
}
