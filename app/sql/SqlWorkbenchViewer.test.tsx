import { useEffect, useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { WorkbenchTabKind } from '@s4wave/sdk/sql/workbench/workbench.pb.js'

class FakeWorkbench {
  public readonly id = 1
  public removed: string[] = []
  public layouts: unknown[] = []
  constructor(private readonly state: unknown) {}
  async getWorkbench() {
    return { workbench: this.state }
  }
  async removePin(key: string) {
    this.removed.push(key)
  }
  async setLayout(tabs: unknown, layout: unknown) {
    this.layouts.push({ tabs, layout })
  }
}

let fakeWorkbench: FakeWorkbench

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
    factory(parentValue, new AbortController().signal).then((result) => {
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
    value: fakeWorkbench,
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
  getObjectKey: () => 'sql/workbench',
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: ({
    objectInfo,
  }: {
    objectInfo: { info?: { value?: { objectKey?: string } } }
  }) => <div data-testid="embedded">{objectInfo?.info?.value?.objectKey}</div>,
}))

import { SqlWorkbenchViewer } from './SqlWorkbenchViewer.js'

function renderViewer() {
  return render(
    <SqlWorkbenchViewer objectInfo={{} as never} worldState={{} as never} />,
  )
}

describe('SqlWorkbenchViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the target db sidebar, pins, and the active tab content', async () => {
    fakeWorkbench = new FakeWorkbench({
      targetDbObjectKey: 'sql/db',
      pinnedQueryObjectKeys: ['sql/db/query/1'],
      openTabs: [
        {
          tabId: 'query:sql/db/query/1',
          objectKey: 'sql/db/query/1',
          kind: WorkbenchTabKind.QUERY,
          title: '1',
        },
      ],
      layout: { activeTabId: 'query:sql/db/query/1' },
    })
    renderViewer()

    await waitFor(() =>
      expect(screen.getAllByTestId('embedded').length).toBeGreaterThan(0),
    )
    // Both the sidebar db tree and the active tab embed an ObjectViewer.
    const embedded = screen
      .getAllByTestId('embedded')
      .map((el) => el.textContent)
    expect(embedded).toContain('sql/db')
    expect(embedded).toContain('sql/db/query/1')
  })

  it('unpins a query through the handle', async () => {
    fakeWorkbench = new FakeWorkbench({
      targetDbObjectKey: 'sql/db',
      pinnedQueryObjectKeys: ['sql/db/query/1'],
      openTabs: [],
      layout: {},
    })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('1')).toBeTruthy())
    await user.click(screen.getByLabelText('Unpin query'))
    await waitFor(() =>
      expect(fakeWorkbench.removed).toEqual(['sql/db/query/1']),
    )
  })

  it('opens a pinned query as a persisted tab', async () => {
    fakeWorkbench = new FakeWorkbench({
      targetDbObjectKey: 'sql/db',
      pinnedQueryObjectKeys: ['sql/db/query/1'],
      openTabs: [],
      layout: {},
    })
    const user = userEvent.setup()
    renderViewer()

    await waitFor(() => expect(screen.getByText('1')).toBeTruthy())
    await user.click(screen.getByText('1'))
    await waitFor(() => expect(fakeWorkbench.layouts.length).toBeGreaterThan(0))
  })
})
