import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// FakeDb drives the viewer through the SqlDatabase handle surface it consumes.
class FakeDb {
  public readonly id = 1
  public tablesBySchema: Record<string, string[]>
  public listTablesCalls: string[] = []

  constructor(
    public schemas: string[],
    tablesBySchema: Record<string, string[]> = {},
  ) {
    this.tablesBySchema = tablesBySchema
  }

  async listSchemas(): Promise<string[]> {
    return this.schemas
  }

  async listTables(schema: string): Promise<string[]> {
    this.listTablesCalls.push(schema)
    return this.tablesBySchema[schema] ?? []
  }
}

let fakeDb: FakeDb
const createObject = vi.fn(async () => ({ release: vi.fn() }))
const navigateToObjects = vi.fn()

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
    setLoading(true)
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

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: fakeDb,
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
  getObjectKey: () => 'sql/db',
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({
      spaceWorld: { createObject },
      navigateToObjects,
    }),
  },
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  setObjectType: vi.fn(),
}))

import { SqlDbViewer } from './SqlDbViewer.js'

function renderViewer() {
  return render(
    <SqlDbViewer objectInfo={{} as never} worldState={{} as never} />,
  )
}

describe('SqlDbViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('lists schemas and lazily loads tables on expand', async () => {
    fakeDb = new FakeDb(['main'], { main: ['people', 'projects'] })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('main')).toBeTruthy())
    expect(screen.getByText('1 schema')).toBeTruthy()
    // Tables are not fetched while the schema is collapsed.
    expect(fakeDb.listTablesCalls).toEqual([])

    await user.click(screen.getByText('main'))
    await waitFor(() => expect(screen.getByText('people')).toBeTruthy())
    expect(screen.getByText('projects')).toBeTruthy()
    expect(fakeDb.listTablesCalls).toEqual(['main'])
  })

  it('creates a query child and navigates on open query editor', async () => {
    fakeDb = new FakeDb(['main'])
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('main')).toBeTruthy())
    await user.click(screen.getByText('Query Editor'))

    await waitFor(() => expect(createObject).toHaveBeenCalled())
    expect(navigateToObjects).toHaveBeenCalledTimes(1)
    const [keys] = navigateToObjects.mock.calls[0]
    expect(keys[0]).toContain('sql/db/query/')
  })
})
