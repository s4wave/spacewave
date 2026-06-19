import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

import { SqlResultGrid } from './SqlResultGrid.js'

const strVal = (s: string): SqlValue => ({
  value: { case: 'strValue', value: s },
})

describe('SqlResultGrid', () => {
  afterEach(cleanup)

  it('renders columns, rows, and NULL cells distinctly', () => {
    render(
      <SqlResultGrid
        data={{
          columns: [{ name: 'name' }, { name: 'role' }],
          rows: [{ values: [strVal('ada'), {}] }],
          truncated: false,
        }}
      />,
    )
    expect(screen.getByText('name')).toBeTruthy()
    expect(screen.getByText('ada')).toBeTruthy()
    const nulls = screen.getAllByText('NULL')
    expect(nulls.length).toBeGreaterThan(0)
  })

  it('shows an empty state when there are no rows or columns', () => {
    render(
      <SqlResultGrid
        data={{ columns: [], rows: [], truncated: false }}
        emptyTitle="Nothing here"
      />,
    )
    expect(screen.getByText('Nothing here')).toBeTruthy()
  })

  it('reports a truncated result', () => {
    render(
      <SqlResultGrid
        data={{
          columns: [{ name: 'a' }],
          rows: [{ values: [strVal('1')] }],
          truncated: true,
        }}
      />,
    )
    expect(screen.getByText(/truncated/)).toBeTruthy()
  })
})
