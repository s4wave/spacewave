import { useCallback, useContext, useEffect, useRef, useState } from 'react'
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

  const handleClick = useCallback(
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

  const context = useContext(ListStateContext)
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
        'text-ui flex items-center px-2 py-[1px] leading-tight',
        'hover:bg-outliner-selected-highlight cursor-pointer transition-colors select-none',
        selected && 'bg-ui-selected hover:bg-ui-selected',
        itemIndex % 2 === 1 && !selected && 'bg-file-row-alternate',
        focused && 'ring-ui-outline-active ring-1 ring-inset',
      )}
      onClick={handleClick}
      onContextMenu={handleContextMenu}
    >
      <span className="flex-1 truncate">{item.id}</span>
    </div>
  )
}
