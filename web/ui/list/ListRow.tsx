import { useCallback, use, useEffect, useRef } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { ListStateContext } from './ListState.js'
import { RowComponentProps } from './List.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: {
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
      className={
        /* TODO(stylex): conditional property composition: backgroundColor */ cn(
          'flex items-center px-2 py-[1px] text-xs leading-tight',
          'cursor-pointer transition-colors select-none',
          selected
            ? 'bg-brand/10 text-foreground'
            : 'text-foreground/90 hover:bg-foreground/5',
          focused && !selected && 'ring-brand/25 ring-1 ring-inset',
        )
      }
      onClick={handleListRowSelect}
      onKeyDown={handleListRowKeyDown}
      onContextMenu={handleContextMenu}
    >
      <span sx={stylexStyles.rule1}>{item.id}</span>
    </div>
  )
}
