import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { useListState } from '@s4wave/web/ui/list'

import { Table, type TableColumn, columnSteps } from './Table.js'

interface Fruit {
  name: string
  color: string
}

const columns: readonly TableColumn<Fruit>[] = [
  { key: 'name', label: 'Name', kind: 'id', text: (f) => f.name },
  { key: 'color', label: 'Color', minWidth: 6, text: (f) => f.color },
]

// SortedTable is a Table whose stored sort names sortKey.
function SortedTable({ sortKey }: { sortKey: string }) {
  const state = useListState(null, 'fruit', { sortKey })
  return (
    <Table
      label="Fruit"
      columns={columns}
      state={state}
      rows={[
        { id: 'pear', data: { name: 'pear', color: 'green' } },
        { id: 'apple', data: { name: 'apple', color: 'red' } },
      ]}
    />
  )
}

describe('Table', () => {
  afterEach(() => {
    cleanup()
  })

  it('leaves room for the scrollbar before a column takes a step', () => {
    // 1.75rem of row padding, 7.5 of name, 0.75 of gap and 6 of color fill
    // 16rem exactly; the 6px scrollbar pushes the color to the next step.
    expect(columnSteps(columns)).toEqual([0, 22])
  })

  it('sorts by the first column when the stored sort names no column', () => {
    // A stored sort names a column the table no longer has.
    render(<SortedTable sortKey="gone" />)

    // The rows sort by name, and the name header shows the sort.
    expect(screen.getByRole('group', { name: 'Fruit' })).toBeDefined()
    const rows = screen.getAllByRole('row').map((row) => row.textContent)
    expect(rows[0]).toContain('apple')
    expect(rows[1]).toContain('pear')
    expect(screen.queryByText(/^by /)).toBeNull()
    const name = screen.getByRole('button', { name: 'Name' })
    expect(name.querySelector('svg')).not.toBeNull()
  })
})
