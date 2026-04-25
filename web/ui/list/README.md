# List Component

A generic, virtualized list component with keyboard navigation, multi-selection, and integrated sorting support.

## Features

- **Virtualized rendering** - Uses `react-window` for efficient rendering of large lists
- **Keyboard navigation** - Arrow keys, vim keys (j/k), Home/End
- **Multi-selection** - Click, Shift+click, Ctrl/Cmd+click
- **Integrated sorting** - List manages sort state internally to ensure correct selection indices
- **State persistence** - Uses `useStateReducerAtom` for automatic localStorage persistence
- **Customizable rows** - Pass your own row component
- **Accessible** - Full ARIA support

## Basic Usage

```tsx
import { List, ListItem, RowComponentProps, ListStateContext } from '@s4wave/web/ui/list'
import { useStateNamespace } from '@s4wave/web/state/persist.js'

interface MyData {
  name: string
  value: number
}

const items: ListItem<MyData>[] = [
  { id: '1', data: { name: 'Item 1', value: 10 } },
  { id: '2', data: { name: 'Item 2', value: 20 } },
]

function MyRow({
  item,
  itemIndex,
  style,
  ariaAttributes,
}: RowComponentProps<MyData>) {
  const context = useContext(ListStateContext)
  const selected = context?.selectedIds?.includes(item.id) ?? false

  return (
    <div style={style} className={selected ? 'bg-ui-selected' : ''}>
      {item.data?.name} - {item.data?.value}
    </div>
  )
}

function MyComponent() {
  const namespace = useStateNamespace(['my-feature'])

  return (
    <List
      items={items}
      rowComponent={MyRow}
      rowHeight={24}
      namespace={namespace}
      stateKey="myList"
      onRowDefaultAction={(items) => console.log('Open:', items)}
    />
  )
}
```

## With Sorting

```tsx
import { List, ListItem, ListSortFn, RenderHeaderProps } from '@s4wave/web/ui/list'

const sortFn: ListSortFn<MyData> = (items, sortKey, sortDirection) => {
  return [...items].sort((a, b) => {
    const aVal = a.data?.[sortKey as keyof MyData]
    const bVal = b.data?.[sortKey as keyof MyData]
    const cmp = aVal < bVal ? -1 : aVal > bVal ? 1 : 0
    return sortDirection === 'asc' ? cmp : -cmp
  })
}

function MyComponent() {
  const namespace = useStateNamespace(['my-feature'])

  const renderHeader = useCallback(({ state, dispatch }: RenderHeaderProps) => {
    const handleSort = (key: string) => dispatch({ type: 'SET_SORT', sortKey: key })

    return (
      <div className="flex">
        <div onClick={() => handleSort('name')}>
          Name {state.sortKey === 'name' && (state.sortDirection === 'asc' ? '↓' : '↑')}
        </div>
        <div onClick={() => handleSort('value')}>
          Value {state.sortKey === 'value' && (state.sortDirection === 'asc' ? '↓' : '↑')}
        </div>
      </div>
    )
  }, [])

  return (
    <List
      items={items}
      rowComponent={MyRow}
      sortFn={sortFn}
      defaultSortKey="name"
      defaultSortDirection="asc"
      renderHeader={renderHeader}
      namespace={namespace}
      stateKey="myList"
    />
  )
}
```

## Keyboard Shortcuts

- **Arrow Up/Down** or **j/k** - Navigate items
- **Shift+Arrow** or **J/K** - Range selection
- **Ctrl/Cmd+Arrow** - Focus without selecting
- **Enter** or **Space** - Open selected items (or toggle selection with Ctrl/Cmd)
- **Ctrl/Cmd+A** - Select all

## Props

### List Props

- `items: ListItem<T>[]` - Array of items to display
- `rowComponent: React.ComponentType<RowComponentProps<T>>` - Component to render each row
- `rowHeight?: number` - Height of each row in pixels (default: 24)
- `placeholder?: React.ReactNode` - Content to show when list is empty
- `className?: string` - Additional CSS classes
- `header?: React.ReactNode` - Static header content
- `renderHeader?: (props: RenderHeaderProps) => React.ReactNode` - Dynamic header with state/dispatch access
- `sortFn?: ListSortFn<T>` - Sorting function (required for sortable lists)
- `defaultSortKey?: string` - Initial sort key
- `defaultSortDirection?: 'asc' | 'desc'` - Initial sort direction (default: 'asc')
- `onRowDefaultAction?: (items: ListItem<T>[]) => void` - Called on Enter/double-click
- `onStateChange?: (state: ListState) => void` - Called when state changes
- `namespace?: StateNamespace` - State persistence namespace
- `stateKey?: string` - Key within namespace (default: 'list')
- `defaultState?: Partial<ListState>` - Initial state if no persisted state exists

### RowComponentProps

Row components receive these props:

- `item: ListItem<T>` - The list item
- `itemIndex: number` - Index in the sorted list
- `style: React.CSSProperties` - Style object from virtualization (must apply)
- `ariaAttributes` - Accessibility attributes (must spread)
- `onRowClick: (index, item, event, clickCount) => void` - Click handler
- `onContextMenu: (index, item) => void` - Context menu handler

### ListItem Interface

```tsx
interface ListItem<T = void> {
  id: string // Unique identifier
  data?: T // Optional associated data
}
```

### ListState Interface

```tsx
interface ListState {
  selectedIds?: string[]
  lastSelectedIndex?: number
  focusedIndex?: number
  sortKey?: string
  sortDirection?: 'asc' | 'desc'
}
```

## Design: Why Sorting is Internal

The List component manages sorting internally to solve a fundamental issue with selection indices:

1. Selection state uses indices (focusedIndex, lastSelectedIndex for range selection)
2. If sorting happens externally and selection happens internally, there's a mismatch
3. The reducer might operate on a different array order than what's displayed

By managing both sorting AND selection internally:
- The reducer always operates on the correctly sorted array
- Index calculations are always accurate
- Selection after sorting always selects the correct item

## Comparison with Tree Component

`List` is for flat, virtualized lists (like file browsers), while `Tree` is for hierarchical data with expand/collapse.

Choose `List` when:

- You have flat data or handle hierarchy externally
- You need virtualization for performance
- You want sorting with correct selection behavior

Choose `Tree` when:

- You have hierarchical data
- You need expand/collapse behavior
- You don't need virtualization (or have reasonable data size)
