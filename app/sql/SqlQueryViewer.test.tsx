import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

// FakeQuery drives the viewer through the SqlQuery handle surface.
class FakeQuery {
  public readonly id = 1
  public savedText?: { sql: string; dialect: string; targetDb: string }
  public savedParams?: SqlValue[]
  public runMaxRows?: number
  public runResult = { resultObjectKey: 'sql/db/result/abc', error: '' }

  constructor(
    public sqlText: string,
    public dialectHint: string,
    public targetDbObjectKey: string,
  ) {}

  async getQueryText() {
    return {
      sqlText: this.sqlText,
      dialectHint: this.dialectHint,
      targetDbObjectKey: this.targetDbObjectKey,
    }
  }

  async setQueryText(sqlText: string, dialectHint: string, targetDb: string) {
    this.savedText = { sql: sqlText, dialect: dialectHint, targetDb }
  }

  async setParameters(parameters: SqlValue[]) {
    this.savedParams = parameters
  }

  async run(maxRows: number) {
    this.runMaxRows = maxRows
    return this.runResult
  }
}

let fakeQuery: FakeQuery
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
    value: fakeQuery,
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
  getObjectKey: () => 'sql/db/query/1',
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({ navigateToObjects }),
  },
}))

import { SqlQueryViewer } from './SqlQueryViewer.js'

function renderViewer() {
  return render(
    <SqlQueryViewer objectInfo={{} as never} worldState={{} as never} />,
  )
}

describe('SqlQueryViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('loads persisted query text into the editor', async () => {
    fakeQuery = new FakeQuery('SELECT 1', 'mysql', 'sql/db')
    renderViewer()
    await waitFor(() =>
      expect(
        (screen.getByLabelText('SQL text') as HTMLTextAreaElement).value,
      ).toBe('SELECT 1'),
    )
    expect(
      (screen.getByLabelText('Dialect hint') as HTMLInputElement).value,
    ).toBe('mysql')
  })

  it('persists edited text and runs, routing to the result', async () => {
    fakeQuery = new FakeQuery('SELECT 1', 'mysql', 'sql/db')
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByLabelText('SQL text')).toBeTruthy())
    const textarea = screen.getByLabelText('SQL text')
    await user.clear(textarea)
    await user.type(textarea, 'SELECT 2')

    await user.click(screen.getByText('Run'))

    await waitFor(() => expect(fakeQuery.runMaxRows).toBe(0))
    expect(fakeQuery.savedText?.sql).toBe('SELECT 2')
    expect(navigateToObjects).toHaveBeenCalledWith(['sql/db/result/abc'])
  })

  it('blocks run on a malformed integer parameter', async () => {
    fakeQuery = new FakeQuery('SELECT ?', 'mysql', 'sql/db')
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('Add')).toBeTruthy())
    await user.click(screen.getByText('Add'))
    await user.selectOptions(screen.getByLabelText('Parameter 1 type'), 'int')
    await user.type(screen.getByLabelText('Parameter 1 value'), 'abc')

    expect((screen.getByText('Run') as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText(/invalid integer/)).toBeTruthy()
  })
})
