import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

const strVal = (s: string): SqlValue => ({
  value: { case: 'strValue', value: s },
})

class FakeTableView {
  public readonly id = 1
  public fetchCalls = 0
  getTableView(): Promise<unknown> {
    return Promise.resolve({
      tableView: {
        targetTableName: 'people',
        whereExpression: 'role = ?',
        rowLimit: 100,
        projectedColumns: ['name'],
      },
    })
  }
  fetchRows(): Promise<unknown> {
    this.fetchCalls++
    return Promise.resolve({
      columns: [{ name: 'name' }],
      rowBatches: [{ rows: [{ values: [strVal('ada')] }] }],
      rowCount: 1n,
      truncated: false,
    })
  }
}

let fakeTableView: FakeTableView

function useResourceMock<P, T>(
  parent: { value: P | null },
  factory: (parent: P, signal: AbortSignal) => Promise<T>,
) {
  const [value, setValue] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [generation, setGeneration] = useState(0)
  const parentValue = parent.value
  useEffect(() => {
    let cancelled = false
    if (parentValue == null) return
    void factory(parentValue, new AbortController().signal).then((result) => {
      if (cancelled) return
      setValue(result)
      setLoading(false)
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [parentValue, generation])
  return {
    value,
    loading,
    error: null,
    retry: () => setGeneration((g) => g + 1),
  }
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: fakeTableView,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (parent: never, factory: never) =>
    useResourceMock(parent, factory),
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'sql/db/table/people',
}))

import { SqlTableViewViewer } from './SqlTableViewViewer.js'

function renderViewer() {
  return render(<SqlTableViewViewer objectInfo={{}} worldState={{} as never} />)
}

describe('SqlTableViewViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders filter metadata and the row grid', async () => {
    fakeTableView = new FakeTableView()
    renderViewer()

    await waitFor(() => expect(screen.getByText('ada')).toBeTruthy())
    expect(screen.getByText('role = ?')).toBeTruthy()
    expect(screen.getByText('name', { selector: 'span' })).toBeTruthy()
  })

  it('refetches rows on refresh', async () => {
    fakeTableView = new FakeTableView()
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(fakeTableView.fetchCalls).toBe(1))
    await user.click(screen.getByText('Refresh'))
    await waitFor(() => expect(fakeTableView.fetchCalls).toBe(2))
  })
})
