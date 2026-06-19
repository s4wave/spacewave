import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { SqlValue } from '@go/github.com/s4wave/spacewave/db/sql/sql.pb.js'

const intVal = (n: bigint): SqlValue => ({
  value: { case: 'intValue', value: n },
})

class FakeSchema {
  public readonly id = 1
  async getSchema() {
    return {
      schema: { schemaName: 'main', targetDbObjectKey: 'sql/db' },
    }
  }
  async listTables() {
    return { tables: [{ name: 'people' }, { name: 'projects' }] }
  }
}

class FakeDb {
  public readonly id = 2
  public counts: Record<string, bigint> = { people: 2n, projects: 5n }
  async query(sql: string) {
    const match = /FROM `(?:[^`]+`\.`)?([^`]+)`/.exec(sql)
    const table = match?.[1] ?? ''
    return {
      columns: [],
      rows: [{ values: [intVal(this.counts[table] ?? 0n)] }],
    }
  }
}

const navigateToObjects = vi.fn()
const setBlock = vi.fn()
const createObjectCursor = { release: vi.fn() }

function useResourceMock<P, T>(
  parent: { value: P | null },
  factory: (parent: P, signal: AbortSignal) => Promise<T>,
  deps: unknown[],
) {
  const [value, setValue] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const parentValue = parent.value
  const depKey = JSON.stringify(deps)
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
  }, [parentValue, depKey])
  return { value, loading, error: null, retry: vi.fn() }
}

const fakeSchema = new FakeSchema()
const fakeDb = new FakeDb()

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: (
    _world: never,
    _key: string,
    HandleClass: { name: string },
  ) => ({
    value: HandleClass?.name === 'FakeDb' ? fakeDb : fakeSchema,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (parent: never, factory: never, deps: never) =>
    useResourceMock(parent, factory, deps),
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'sql/db/schema/main',
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({
      spaceWorld: {
        buildStorageCursor: async () => ({
          [Symbol.dispose]() {},
        }),
        createObject: vi.fn(),
      },
      navigateToObjects,
    }),
  },
}))

vi.mock('@s4wave/sdk/world/utils.js', () => ({
  createWorldObject: async (
    _world: never,
    _cursor: never,
    _key: string,
    cb: (cursor: { setBlock: typeof setBlock }) => void,
  ) => {
    cb({ setBlock })
    return { objectState: createObjectCursor }
  },
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  setObjectType: vi.fn(),
}))

// SqlDatabase resolves to FakeDb via the handle mock dispatch by class name; the
// viewer imports the real class, so rename it for the test dispatch.
vi.mock('@s4wave/sdk/sql/sql.js', () => ({
  SqlDatabase: class FakeDb {},
}))

import { SqlSchemaViewer } from './SqlSchemaViewer.js'

function renderViewer() {
  return render(
    <SqlSchemaViewer objectInfo={{} as never} worldState={{} as never} />,
  )
}

describe('SqlSchemaViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('lists tables with row counts queried from the target database', async () => {
    renderViewer()
    await waitFor(() => expect(screen.getByText('people')).toBeTruthy())
    expect(screen.getByText('projects')).toBeTruthy()
    await waitFor(() => expect(screen.getByText('2 rows')).toBeTruthy())
    expect(screen.getByText('5 rows')).toBeTruthy()
  })

  it('opens a table as a table-view child and navigates', async () => {
    const user = userEvent.setup()
    renderViewer()
    await waitFor(() => expect(screen.getByText('people')).toBeTruthy())

    await user.click(screen.getByText('people'))
    await waitFor(() => expect(navigateToObjects).toHaveBeenCalled())
    expect(setBlock).toHaveBeenCalled()
    const [keys] = navigateToObjects.mock.calls[0]
    expect(keys[0]).toContain('/table/people/')
  })
})
