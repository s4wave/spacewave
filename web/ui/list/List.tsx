import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react'
import isEqual from 'lodash.isequal'
import { List as VirtualList, ListImperativeAPI } from 'react-window'
import { cn } from '@s4wave/web/style/utils.js'
import {
  useStateReducerAtom,
  StateNamespace,
} from '@s4wave/web/state/persist.js'
import { ListItem } from './ListItem.js'
import {
  ListAction,
  ListDispatch,
  ListState,
  ListStateContext,
  ListDispatchContext,
  SortDirection,
  listReducer,
  translateIndicesToNewOrder,
} from './ListState.js'

export interface RowComponentProps<T> {
  item: ListItem<T>
  itemIndex: number
  onRowClick: (
    index: number,
    item: ListItem<T>,
    event: React.MouseEvent,
    clickCount: number,
  ) => void
  onContextMenu: (
    index: number,
    item: ListItem<T>,
    event: React.MouseEvent,
  ) => void
  style: React.CSSProperties
  ariaAttributes: {
    'aria-posinset': number
    'aria-setsize': number
    role: 'listitem'
  }
}

export type ListSortFn<T> = (
  items: ListItem<T>[],
  sortKey: string,
  sortDirection: SortDirection,
) => ListItem<T>[]

export interface RenderHeaderProps {
  state: ListState
  dispatch: ListDispatch
}

export interface ListProps<T = void> {
  items: ListItem<T>[]
  placeholder?: React.ReactNode
  className?: string
  rowHeight?: number
  rowComponent: React.ComponentType<RowComponentProps<T>>
  onRowDefaultAction?: (items: ListItem<T>[]) => void
  header?: React.ReactNode
  renderHeader?: (props: RenderHeaderProps) => React.ReactNode
  sortFn?: ListSortFn<T>
  defaultSortKey?: string
  defaultSortDirection?: SortDirection
  onStateChange?: (state: ListState) => void
  namespace?: StateNamespace
  stateKey?: string
  defaultState?: Partial<ListState>
  onRowContextMenu?: (item: ListItem<T>, event: React.MouseEvent) => void
  // autoHeight disables virtualization and renders all rows at natural height.
  // The list takes its content height instead of filling its container.
  autoHeight?: boolean
}

// List renders a virtualized list with keyboard navigation and selection.
export function List<T>({
  items,
  placeholder,
  className,
  rowHeight = 24,
  rowComponent: RowComponentProp,
  onRowDefaultAction,
  header,
  renderHeader,
  sortFn,
  defaultSortKey,
  defaultSortDirection = 'asc',
  onStateChange,
  namespace,
  stateKey = 'list',
  defaultState,
  onRowContextMenu,
  autoHeight,
}: ListProps<T>) {
  const listRef = useRef<ListImperativeAPI | null>(null)

  // Build initial state with defaults - computed once on mount
  const [initialState] = useState<ListState>(() => ({
    selectedIds: [],
    sortKey: defaultSortKey,
    sortDirection: defaultSortDirection,
    ...defaultState,
  }))

  // Track sorted items for the reducer - use ref to avoid stale closure
  const sortedItemsRef = useRef<ListItem<T>[]>(items)

  // Reducer wrapper that uses current sorted items
  const reducer = useCallback(
    (state: ListState, action: ListAction) =>
      listReducer(sortedItemsRef.current, state, action),
    [],
  )

  // Use persisted state via useStateReducerAtom
  const [state, dispatch] = useStateReducerAtom<ListState, ListAction>(
    namespace ?? null,
    stateKey,
    reducer,
    initialState,
  )

  // Compute sorted items based on current state
  const sortedItems = useMemo(() => {
    if (!sortFn) return items
    const key = state.sortKey ?? defaultSortKey
    const dir = state.sortDirection ?? defaultSortDirection
    if (!key) return items
    return sortFn(items, key, dir)
  }, [
    items,
    sortFn,
    state.sortKey,
    state.sortDirection,
    defaultSortKey,
    defaultSortDirection,
  ])

  // Track previous sorted items for index translation
  const prevSortedItemsRef = useRef<ListItem<T>[]>(sortedItems)

  // Update sortedItemsRef for reducer
  useEffect(() => {
    sortedItemsRef.current = sortedItems
  }, [sortedItems])

  // Translate indices when sort order changes
  useEffect(() => {
    const prev = prevSortedItemsRef.current
    if (prev !== sortedItems) {
      const translation = translateIndicesToNewOrder(state, prev, sortedItems)
      if (translation) {
        dispatch({
          type: 'UPDATE_INDICES',
          focusedIndex: translation.focusedIndex,
          lastSelectedIndex: translation.lastSelectedIndex,
        })
      }
      prevSortedItemsRef.current = sortedItems
    }
  }, [sortedItems, state, dispatch])

  // Notify parent of state changes using useEffectEvent to access latest callback
  const notifyStateChange = useEffectEvent((newState: ListState) => {
    onStateChange?.(newState)
  })
  useEffect(() => {
    notifyStateChange(state)
  }, [state])

  // Store state ref for callbacks
  const stateRef = useRef<ListState>(state)
  const pendingDeselectRef = useRef<number | null>(null)
  useEffect(() => {
    stateRef.current = state
  }, [state])

  const handleOpenItems = useCallback(() => {
    if (!onRowDefaultAction) return
    const selectedItemsIds = stateRef.current?.selectedIds
    const selectedItems = sortedItemsRef.current.filter((v) =>
      selectedItemsIds?.includes(v.id),
    )
    onRowDefaultAction(selectedItems)
  }, [onRowDefaultAction])

  const handleRowClick = useCallback(
    (
      _index: number,
      { id }: ListItem<T>,
      event: React.MouseEvent,
      clickCount: number,
    ) => {
      const isRangeSelection = event.shiftKey
      const isToggleSelection = event.ctrlKey || event.metaKey

      if (!isRangeSelection && !isToggleSelection) {
        const isDoubleClick = clickCount === 2
        if (isDoubleClick) {
          if (pendingDeselectRef.current != null) {
            clearTimeout(pendingDeselectRef.current)
            pendingDeselectRef.current = null
          }
          const selectedItemsIds = stateRef.current?.selectedIds
          const selected = sortedItemsRef.current.filter(
            (v) => v.id === id || selectedItemsIds?.some((sid) => sid === v.id),
          )
          if (selected.length && onRowDefaultAction) {
            onRowDefaultAction(selected)
          }
          return
        }

        if (clickCount && clickCount !== 1) return

        // If clicking on an already-selected item, defer deselection so a
        // double-click can still open all selected items. If no double-click
        // follows, collapse to single selection. Matches Finder/Explorer.
        const selectedItemsIds = stateRef.current?.selectedIds
        if (selectedItemsIds?.includes(id)) {
          if (selectedItemsIds.length > 1) {
            pendingDeselectRef.current = window.setTimeout(() => {
              pendingDeselectRef.current = null
              dispatch({ type: 'SELECT_ITEM', id })
            }, 250)
          }
          return
        }
      }

      dispatch({
        type: 'SELECT_ITEM',
        id,
        range: isRangeSelection,
        toggle: isToggleSelection,
      })
    },
    [dispatch, onRowDefaultAction],
  )

  const handleContextMenu = useCallback(
    (_index: number, item: ListItem<T>, event: React.MouseEvent) => {
      if (item.id && !stateRef.current?.selectedIds?.includes(item.id)) {
        dispatch({
          type: 'SELECT_ITEM',
          id: item.id,
        })
      }
      onRowContextMenu?.(item, event)
    },
    [dispatch, onRowContextMenu],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const { key, ctrlKey, metaKey } = e
      let { shiftKey } = e
      let focus = ctrlKey || metaKey
      let toggle = false
      let offset: number | undefined

      switch (key) {
        case 'a':
          if (ctrlKey || metaKey) {
            e.preventDefault()
            dispatch({
              type: 'SELECT_ITEM',
              all: true,
            })
            return
          }
          return

        case 'o':
          if (ctrlKey || metaKey) {
            e.preventDefault()
            handleOpenItems()
            return
          }
          return

        case 'K':
          shiftKey = true
        // eslint-disable-next-line no-fallthrough
        case 'ArrowUp':
        case 'k':
          offset = -1
          break

        case 'J':
          shiftKey = true
        // eslint-disable-next-line no-fallthrough
        case 'ArrowDown':
        case 'j':
          offset = 1
          break

        case ' ':
        case 'Enter':
          if (ctrlKey || metaKey) {
            offset = 0
            toggle = true
            focus = false
            break
          }

          e.preventDefault()
          handleOpenItems()
          return

        default:
          return
      }

      e.preventDefault()
      dispatch({
        type: 'SELECT_ITEM',
        offset,
        toggle,
        focus,
        range: shiftKey,
      })
    },
    [dispatch, handleOpenItems],
  )

  useEffect(() => {
    if (
      listRef?.current &&
      typeof state.lastSelectedIndex === 'number' &&
      state.lastSelectedIndex >= 0 &&
      state.lastSelectedIndex < sortedItems.length
    ) {
      listRef.current.scrollToRow({
        index: state.lastSelectedIndex,
        align: 'smart',
      })
    }
  }, [state.lastSelectedIndex, sortedItems.length])

  const [visibleItems, setVisibleItems] = useState<{
    visibleStartIndex: number
    visibleStopIndex: number
  } | null>(null)

  const onRowsRendered = useCallback(
    (
      visibleRows: { startIndex: number; stopIndex: number },
      _allRows: { startIndex: number; stopIndex: number },
    ) => {
      setVisibleItems((prev) => {
        const next = {
          visibleStartIndex: visibleRows.startIndex,
          visibleStopIndex: visibleRows.stopIndex,
        }
        return isEqual(next, prev) ? prev : next
      })
    },
    [],
  )

  const focusedIndexVisible =
    state.focusedIndex !== undefined &&
    visibleItems &&
    state.focusedIndex >= visibleItems.visibleStartIndex &&
    state.focusedIndex <= visibleItems.visibleStopIndex

  const listContainerRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!focusedIndexVisible && listContainerRef.current) {
      listContainerRef.current.focus()
    }
  }, [focusedIndexVisible])

  const RowComponentInternal = useCallback(
    ({
      index,
      style,
      ariaAttributes,
    }: {
      index: number
      style: React.CSSProperties
      ariaAttributes: {
        'aria-posinset': number
        'aria-setsize': number
        role: 'listitem'
      }
    }) => {
      const item = sortedItems[index]
      return (
        <RowComponentProp
          item={item}
          itemIndex={index}
          onRowClick={handleRowClick}
          onContextMenu={handleContextMenu}
          style={style}
          ariaAttributes={ariaAttributes}
        />
      )
    },
    [sortedItems, handleRowClick, handleContextMenu, RowComponentProp],
  )

  // Render header with state and dispatch if renderHeader is provided
  const headerContent = useMemo(() => {
    if (renderHeader) {
      return renderHeader({ state, dispatch })
    }
    return header
  }, [renderHeader, state, dispatch, header])

  return (
    <ListStateContext.Provider value={state}>
      <ListDispatchContext.Provider value={dispatch}>
        <div
          className={cn(
            autoHeight ? 'flex flex-col' : (
              'flex min-h-0 flex-1 flex-col overflow-hidden'
            ),
            className,
          )}
          role="list"
          aria-label="List"
        >
          {headerContent}

          <div
            ref={listContainerRef}
            tabIndex={focusedIndexVisible ? -1 : 0}
            role="rowgroup"
            className={cn(
              'p-[2px] outline-none',
              autoHeight ? 'flex flex-col' : (
                'flex min-h-0 flex-1 flex-col overflow-hidden'
              ),
            )}
            onKeyDown={handleKeyDown}
          >
            {sortedItems.length === 0 ?
              <div className="text-foreground-alt flex flex-1 items-center justify-center p-4">
                {placeholder ?? 'No items'}
              </div>
            : autoHeight ?
              <div role="list" className="relative">
                {sortedItems.map((item, index) => (
                  <RowComponentInternal
                    key={item.id}
                    index={index}
                    style={{ height: rowHeight }}
                    ariaAttributes={{
                      'aria-posinset': index + 1,
                      'aria-setsize': sortedItems.length,
                      role: 'listitem',
                    }}
                  />
                ))}
              </div>
            : <VirtualList
                listRef={listRef}
                rowHeight={rowHeight}
                rowCount={sortedItems.length}
                onRowsRendered={onRowsRendered}
                rowComponent={RowComponentInternal}
                rowProps={{}}
                className="flex-1"
              />
            }
          </div>
        </div>
      </ListDispatchContext.Provider>
    </ListStateContext.Provider>
  )
}
