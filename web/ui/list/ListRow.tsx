import { useCallback, use, useEffect, useRef, useState } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { ListStateContext } from './ListState.js'
import { RowComponentProps } from './List.js'

// ListRow renders a basic list row with selection state.
export function ListRow<T>({
  item,
  itemIndex,
  onRowClick,
  onContextMenu,
  style,
  ariaAttributes,
}: RowComponentProps<T>) {
  const [clickCount, setClickCount] = useState(0)
  const clickTimerRef = useRef<number | null>(null)

  const handleListRowSelect = useCallback(
    (e: React.MouseEvent) => {
      const newCount = clickCount + 1
      setClickCount(newCount)

      if (onRowClick) {
        onRowClick(itemIndex, item, e, 1)
      }

      if (clickTimerRef.current) {
        clearTimeout(clickTimerRef.current)
      }

      clickTimerRef.current = window.setTimeout(() => {
        if (newCount > 1 && onRowClick) {
          onRowClick(itemIndex, item, e, newCount)
        }
        setClickCount(0)
        clickTimerRef.current = null
      }, 300)
    },
    [clickCount, itemIndex, item, onRowClick],
  )

  const handleContextMenu = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      if (onContextMenu) {
        onContextMenu(itemIndex, item, e)
      }
    },
    [itemIndex, item, onContextMenu],
  )

  const handleListRowKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    e.currentTarget.dispatchEvent(
      new MouseEvent('click', { bubbles: true, cancelable: true }),
    )
  }, [])

  const context = use(ListStateContext)
  const selected = context?.selectedIds?.includes(item.id) ?? false
  const focused = itemIndex === context?.focusedIndex

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
      className={cn(
        'flex items-center px-2 py-[1px] text-xs leading-tight',
        'cursor-pointer transition-colors select-none',
        selected ?
          'bg-brand/10 text-foreground'
        : 'text-foreground/90 hover:bg-foreground/5',
        focused && !selected && 'ring-brand/25 ring-1 ring-inset',
      )}
      onClick={handleListRowSelect}
      onKeyDown={handleListRowKeyDown}
      onContextMenu={handleContextMenu}
    >
      <span className="flex-1 truncate">{item.id}</span>
    </div>
  )
}
