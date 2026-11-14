import {
  type HTMLAttributes,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  type Ref,
  use,
  useCallback,
  useEffect,
  useRef,
} from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { ListStateContext } from './ListState.js'
import { RowComponentProps } from './List.js'

// isEditableElement reports whether el takes typed text.
function isEditableElement(el: Element | null): el is HTMLElement {
  return (
    el instanceof HTMLElement &&
    (el.matches('input, textarea, select, [contenteditable="true"]') ||
      el.isContentEditable)
  )
}

// useFocusRow moves DOM focus to the row when it becomes the focused row,
// unless the person is typing in an input, in the row or elsewhere.
function useFocusRow(focused: boolean) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    // Focus the row unless it holds an input the person may be typing in.
    const row = ref.current
    if (!focused || !row) return
    if (row.querySelector('input, textarea, select, [contenteditable="true"]'))
      return
    const active = document.activeElement
    if (isEditableElement(active) && !row.contains(active)) return
    row.focus()
  }, [focused])
  return ref
}

// ListRowProps are a List row's props plus the cells it draws. Other div
// props, such as drag handlers, pass to the row.
export interface ListRowProps<T>
  extends
    RowComponentProps<T>,
    Omit<
      HTMLAttributes<HTMLDivElement>,
      'children' | 'className' | 'style' | 'onContextMenu'
    > {
  // children are the row's cells; the item id when absent.
  children?: ReactNode
  // dropTarget marks the row as where a dragged item would drop.
  dropTarget?: boolean
  // className is appended to the row's class list, for layout such as gaps.
  className?: string
}

// ListRow is the row every List draws: the selected and focused look, the bar
// at the selected row's edge, focus that follows the List's focused index,
// and clicks, Enter and Space reported to the List, which selects by their
// modifier keys. A row lays its cells out in a line.
export function ListRow<T>({
  item,
  itemIndex,
  onRowClick,
  onContextMenu,
  style,
  ariaAttributes,
  children,
  dropTarget,
  className,
  ...rest
}: ListRowProps<T>) {
  // Read the row's selection and focus from the List.
  const context = use(ListStateContext)
  const selected = context?.selectedIds?.includes(item.id) ?? false
  const focused = itemIndex === context?.focusedIndex
  const ref: Ref<HTMLDivElement> = useFocusRow(focused)

  // A click selects, a double click opens.
  const select = useCallback(
    (e: MouseEvent) => onRowClick?.(itemIndex, item, e, 1),
    [itemIndex, item, onRowClick],
  )
  const open = useCallback(
    (e: MouseEvent) => onRowClick?.(itemIndex, item, e, 2),
    [itemIndex, item, onRowClick],
  )

  // Enter and Space act as a single click on the row.
  const keyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key !== 'Enter' && e.key !== ' ') return
      e.preventDefault()
      onRowClick?.(itemIndex, item, e as unknown as MouseEvent, 1)
    },
    [itemIndex, item, onRowClick],
  )

  // Open the row's menu at the pointer.
  const contextMenu = useCallback(
    (e: MouseEvent) => {
      e.preventDefault()
      e.stopPropagation()
      onContextMenu?.(itemIndex, item, e)
    },
    [itemIndex, item, onContextMenu],
  )

  return (
    <div
      {...rest}
      ref={ref}
      role="row"
      tabIndex={focused ? 0 : -1}
      aria-selected={selected || undefined}
      aria-posinset={ariaAttributes['aria-posinset']}
      aria-setsize={ariaAttributes['aria-setsize']}
      style={style}
      className={cn(
        'group relative flex items-center px-3 py-1.5 text-xs [@media(pointer:coarse)]:py-0',
        'cursor-pointer transition-colors select-none',
        selected
          ? 'bg-brand/10 text-foreground'
          : 'text-foreground/90 hover:bg-foreground/5',
        focused && !selected && 'ring-brand/25 ring-1 ring-inset',
        dropTarget && 'bg-brand/10 ring-brand/40 ring-1 ring-inset',
        className,
      )}
      onClick={select}
      onKeyDown={keyDown}
      onDoubleClick={open}
      onContextMenu={contextMenu}
    >
      {selected && (
        <span className="bg-brand/80 absolute top-1 bottom-1 left-0 w-0.5 rounded-r" />
      )}
      {children ?? <span className="flex-1 truncate">{item.id}</span>}
    </div>
  )
}
