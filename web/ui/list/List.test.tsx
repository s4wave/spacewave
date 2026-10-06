import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'
import { List, type ListProps } from './List.js'
import { ListRow } from './ListRow.js'
import { ListStateContext, useListState } from './ListState.js'
import type { ListState } from './ListState.js'
import type { RowComponentProps } from './List.js'
import type { ListItem } from './ListItem.js'

// A simple row component for testing List in isolation.
function TestRow<T>({ item, style, ariaAttributes }: RowComponentProps<T>) {
  return (
    <div role={ariaAttributes.role} style={style}>
      {item.id}
    </div>
  )
}

// TestList owns the list state the way an application component does.
function TestList<T>(props: Omit<ListProps<T>, 'state'>) {
  const state = useListState(null, 'list')
  return <List {...props} state={state} />
}

const mockItems: ListItem[] = [
  { id: 'item-1' },
  { id: 'item-2' },
  { id: 'item-3' },
]

describe('List', () => {
  afterEach(() => {
    cleanup()
  })

  describe('empty state', () => {
    it('shows default placeholder "No items" when items array is empty', () => {
      render(<TestList items={[]} rowComponent={TestRow} />)
      expect(screen.getByText('No items')).toBeTruthy()
    })

    it('shows custom placeholder when provided', () => {
      render(
        <TestList
          items={[]}
          rowComponent={TestRow}
          placeholder="Nothing to display"
        />,
      )
      expect(screen.getByText('Nothing to display')).toBeTruthy()
    })
  })

  describe('structure', () => {
    it('renders with aria-label="List"', () => {
      render(<TestList items={[]} rowComponent={TestRow} />)
      const list = screen.getByRole('group')
      expect(list.getAttribute('aria-label')).toBe('List')
    })

    it('applies custom className', () => {
      render(<TestList items={[]} rowComponent={TestRow} className="mt-1" />)
      const list = screen.getByRole('group')
      expect(list.className).toContain('mt-1')
    })

    it('renders header content when header prop is provided', () => {
      render(
        <TestList
          items={[]}
          rowComponent={TestRow}
          header={<div data-testid="test-header">Header Content</div>}
        />,
      )
      expect(screen.getByTestId('test-header')).toBeTruthy()
      expect(screen.getByText('Header Content')).toBeTruthy()
    })
  })

  describe('keyboard handling', () => {
    it.each(['ArrowDown', 'ArrowUp'])('%s prevents default', (key) => {
      render(<TestList items={mockItems} rowComponent={TestRow} />)
      const rowgroup = screen.getByRole('rowgroup')
      rowgroup.focus()

      const event = new KeyboardEvent('keydown', {
        key,
        bubbles: true,
        cancelable: true,
      })
      const preventDefaultSpy = vi.spyOn(event, 'preventDefault')
      rowgroup.dispatchEvent(event)
      expect(preventDefaultSpy).toHaveBeenCalled()
    })
  })
})

describe('ListRow', () => {
  afterEach(() => {
    cleanup()
  })

  const defaultAriaAttributes = {
    'aria-posinset': 1,
    'aria-setsize': 3,
    role: 'listitem' as const,
  }

  function renderListRow(
    overrides: Partial<RowComponentProps<void>> = {},
    state: ListState = {
      selectedIds: [],
      sortDirection: 'asc',
    },
  ) {
    const props: RowComponentProps<void> = {
      item: { id: 'item-1' },
      itemIndex: 0,
      onRowClick: vi.fn(),
      onContextMenu: vi.fn(),
      style: { height: 24 },
      ariaAttributes: defaultAriaAttributes,
      ...overrides,
    }
    const result = render(
      <ListStateContext.Provider value={state}>
        <ListRow {...props} />
      </ListStateContext.Provider>,
    )
    return { ...result, props }
  }

  describe('rendering', () => {
    it('renders item id as text', () => {
      renderListRow()
      expect(screen.getByText('item-1')).toBeTruthy()
    })

    it('applies style prop', () => {
      renderListRow({ style: { height: 48, color: 'red' } })
      const row = screen.getByRole('row')
      expect(row.style.height).toBe('48px')
      expect(row.style.color).toBe('red')
    })
  })

  describe('selection state', () => {
    it('shows aria-selected when item is in selectedIds', () => {
      renderListRow({}, { selectedIds: ['item-1'], sortDirection: 'asc' })
      const row = screen.getByRole('row')
      expect(row.getAttribute('aria-selected')).toBe('true')
    })

    it('does not show aria-selected when not selected', () => {
      renderListRow({}, { selectedIds: ['item-2'], sortDirection: 'asc' })
      const row = screen.getByRole('row')
      expect(row.getAttribute('aria-selected')).toBeNull()
    })

    it('has focus ring class when focused (focusedIndex matches itemIndex)', () => {
      renderListRow(
        { itemIndex: 0 },
        { selectedIds: [], focusedIndex: 0, sortDirection: 'asc' },
      )
      const row = screen.getByRole('row')
      expect(row.className).toContain('ring-brand/25')
    })

    it('does not have focus ring class when not focused', () => {
      renderListRow(
        { itemIndex: 0 },
        { selectedIds: [], focusedIndex: 2, sortDirection: 'asc' },
      )
      const row = screen.getByRole('row')
      expect(row.className).not.toContain('ring-brand/25')
    })
  })

  describe('click handling', () => {
    it('calls onRowClick with correct args on click', () => {
      const onRowClick = vi.fn()
      renderListRow({
        item: { id: 'item-2' },
        itemIndex: 1,
        onRowClick,
      })
      const row = screen.getByRole('row')
      fireEvent.click(row)

      expect(onRowClick).toHaveBeenCalledOnce()
      expect(onRowClick).toHaveBeenCalledWith(
        1,
        { id: 'item-2' },
        expect.any(Object),
        1,
      )
    })

    it('calls onContextMenu on right-click and prevents default', () => {
      const onContextMenu = vi.fn()
      renderListRow({
        item: { id: 'item-3' },
        itemIndex: 2,
        onContextMenu,
      })
      const row = screen.getByRole('row')
      const event = new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
      })
      const preventDefaultSpy = vi.spyOn(event, 'preventDefault')
      row.dispatchEvent(event)

      expect(onContextMenu).toHaveBeenCalledOnce()
      const [, item, evt] = onContextMenu.mock.calls[0] as [
        number,
        ListItem,
        { nativeEvent: MouseEvent },
      ]
      expect(onContextMenu).toHaveBeenCalledWith(2, item, evt)
      expect(item).toEqual({ id: 'item-3' })
      expect(evt.nativeEvent).toBeInstanceOf(MouseEvent)
      expect(preventDefaultSpy).toHaveBeenCalled()
    })
  })
})
