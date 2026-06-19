import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

const strVal = (s: string): SqlValue => ({
  value: { case: 'strValue', value: s },
})

class FakeResult {
  public readonly id = 1
  constructor(private readonly grid: unknown) {}
  async getResultGrid() {
    return this.grid
  }
}

let fakeResult: FakeResult
const navigateToObjects = vi.fn()

function useResourceMock<P, T>(
  parent: { value: P | null },
  factory: (parent: P, signal: AbortSignal) => Promise<T>,
) {
  const [value, setValue] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const parentValue = parent.value
  useEffect(() => {
    let cancelled = false
    if (parentValue == null) return
    factory(parentValue, new AbortController().signal).then((result) => {
      if (cancelled) return
      setValue(result)
      setLoading(false)
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [parentValue])
  return { value, loading, error: null, retry: vi.fn() }
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: fakeResult,
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
  getObjectKey: () => 'sql/db/result/1',
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({ navigateToObjects }),
  },
}))

import { SqlQueryResultViewer } from './SqlQueryResultViewer.js'

function renderViewer() {
  return render(
    <SqlQueryResultViewer objectInfo={{} as never} worldState={{} as never} />,
  )
}

describe('SqlQueryResultViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the typed grid and navigates to the source query', async () => {
    fakeResult = new FakeResult({
      columns: [{ name: 'name' }],
      rowBatches: [{ rows: [{ values: [strVal('ada')] }] }],
      rowCount: 1n,
      truncated: false,
      sourceQueryObjectKey: 'sql/db/query/1',
    })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('ada')).toBeTruthy())
    expect(screen.getByText('1 rows')).toBeTruthy()

    await user.click(screen.getByText('Query'))
    expect(navigateToObjects).toHaveBeenCalledWith(['sql/db/query/1'])
  })

  it('surfaces an execution error instead of the grid', async () => {
    fakeResult = new FakeResult({
      columns: [],
      rowBatches: [],
      error: { message: 'syntax error near SELCT' },
    })
    renderViewer()
    await waitFor(() =>
      expect(screen.getByText('syntax error near SELCT')).toBeTruthy(),
    )
  })
})
