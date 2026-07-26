import { useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import {
  SHELL_TAB_PATH_COMMITTED_EVENT,
  ShellTabStateProvider,
  ShellTabsProvider,
  useShellTabs,
} from './ShellTabContext.js'
import {
  BROWSER_SHELL_TABS_STORAGE_KEY,
  BrowserShellTabsStore,
  resetBrowserShellTabsStoreForTests,
} from './BrowserShellTabsStore.js'
import {
  installShellTabTestBrowser,
  readShellTabsSnapshot,
  seedShellTabs,
} from './ShellTabTestHarness.js'
import { classifyShellDocumentEntry } from './ShellDocumentEntry.js'
import {
  readShellDocumentState,
  shellTabStateStorageKey,
  writeShellDocumentState,
} from './ShellDocumentState.js'
import {
  StateNamespaceProvider,
  useStateAtom,
  type StateAtomAccessor,
} from '@s4wave/web/state/index.js'
import { toast } from '@s4wave/web/ui/toaster.js'

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    error: vi.fn(),
  },
}))

const continuationEntry = {
  kind: 'continuation' as const,
  path: '/',
  params: {},
  incarnation: 'test-document',
}

function NoopPathUpdateProbe() {
  const { tabs, updateTabPath } = useShellTabs()

  useEffect(() => {
    if (tabs.length === 0) return
    updateTabPath(tabs[0].id, tabs[0].path)
  }, [tabs, updateTabPath])

  return <div data-testid="tab-count">{tabs.length}</div>
}

function NoopSetTabsProbe({ onCommit }: { onCommit: () => void }) {
  const { setTabs } = useShellTabs()

  useEffect(onCommit)

  useEffect(() => {
    setTabs((tabs) => tabs.map((tab) => ({ ...tab })))
  }, [setTabs])

  return <div data-testid="probe-mounted">mounted</div>
}

function ActiveTabProbe({ testId = 'active-tab-id' }: { testId?: string }) {
  const { activeTabId } = useShellTabs()
  return <div data-testid={testId}>{activeTabId}</div>
}

function TabPathProbe() {
  const { tabs } = useShellTabs()
  return (
    <div data-testid="tab-paths">{tabs.map((tab) => tab.path).join('|')}</div>
  )
}

function TabsProjectionProbe({ testId }: { testId: string }) {
  const { activeTabId, tabs } = useShellTabs()
  return (
    <div data-testid={testId}>
      {JSON.stringify({
        activeTabId,
        tabs: tabs.map(({ id, path }) => ({ id, path })),
      })}
    </div>
  )
}

function TabStateProbe() {
  const [count, setCount] = useStateAtom<number>(null, 'count', 0)
  return (
    <>
      <div data-testid="tab-state-count">{count}</div>
      <button type="button" onClick={() => setCount((value) => value + 1)}>
        Increment state
      </button>
    </>
  )
}

function ReplaceActiveTabPathProbe({ path }: { path: string }) {
  const { activeTabId, tabs, updateTabPath } = useShellTabs()

  useEffect(() => {
    updateTabPath(activeTabId, path)
  }, [activeTabId, path, updateTabPath])

  return (
    <>
      <div data-testid="tab-count">{tabs.length}</div>
      <div data-testid="tab-paths">{tabs.map((tab) => tab.path).join('|')}</div>
    </>
  )
}

function OpenDocsProbe() {
  const { activeTabId, openPathInNewTab, tabs } = useShellTabs()
  return (
    <>
      <button
        type="button"
        onClick={() =>
          openPathInNewTab('/docs', {
            afterTabId: activeTabId,
            focusExisting: true,
          })
        }
      >
        Open Docs
      </button>
      <div data-testid="active-tab-id">{activeTabId}</div>
      <div data-testid="tabs-json">{JSON.stringify(tabs)}</div>
    </>
  )
}

function ResetProbe() {
  const { resetShellTabs } = useShellTabs()
  return (
    <button type="button" onClick={resetShellTabs}>
      Reset Shell Tabs
    </button>
  )
}

function CloseProbe({ tabId }: { tabId: string }) {
  const { closeShellTab } = useShellTabs()
  return (
    <button type="button" onClick={() => closeShellTab(tabId)}>
      Close Shell Tab
    </button>
  )
}

function MutationFailureProbe() {
  const { mutationError, openPathInNewTab } = useShellTabs()
  return (
    <>
      <button type="button" onClick={() => openPathInNewTab('/docs')}>
        Trigger failed add
      </button>
      {mutationError ? <div role="alert">{mutationError.code}</div> : null}
    </>
  )
}

function OpenCliInActiveTabsetProbe() {
  const { activeTabId, openPathInActiveTabset, tabs } = useShellTabs()
  return (
    <>
      <button
        type="button"
        onClick={() =>
          openPathInActiveTabset('/u/7/settings/cli/terminal', {
            afterTabId: activeTabId,
            focusExisting: true,
            select: true,
          })
        }
      >
        Open CLI terminal
      </button>
      <div data-testid="active-tab-id">{activeTabId}</div>
      <div data-testid="tabs-json">{JSON.stringify(tabs)}</div>
    </>
  )
}
describe('ShellTabContext', () => {
  beforeEach(() => {
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: async (
          _name: string,
          _options: unknown,
          callback: (lock: object) => Promise<unknown>,
        ) => callback({}),
      },
    })
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    resetBrowserShellTabsStoreForTests()
  })

  it('treats same-path tab updates as a no-op', () => {
    seedShellTabs([{ id: 'tab-1', name: 'Home', path: '/' }])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <NoopPathUpdateProbe />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('tab-count').textContent).toBe('1')
  })

  it('keeps semantically equal local arrays from mutating shared records', async () => {
    const onCommit = vi.fn()
    seedShellTabs([{ id: 'tab-1', name: 'Home', path: '/' }])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <NoopSetTabsProbe onCommit={onCommit} />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('probe-mounted').textContent).toBe('mounted')
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(onCommit).toHaveBeenCalled()
  })

  it('accepts newer cross-window records without adopting local selection', async () => {
    seedShellTabs([{ id: 'tab-1', name: 'Home', path: '/' }])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ActiveTabProbe />
        <TabPathProbe />
      </ShellTabsProvider>,
    )

    window.dispatchEvent(
      new StorageEvent('storage', {
        key: BROWSER_SHELL_TABS_STORAGE_KEY,
        newValue: JSON.stringify({
          schemaVersion: 1,
          epoch: 0,
          revision: 2,
          records: [
            { id: 'tab-1', name: 'Home', path: '/', creationSequence: 1 },
            { id: 'tab-2', name: 'Docs', path: '/docs', creationSequence: 2 },
            { id: 'tab-3', name: 'Chat', path: '/chat', creationSequence: 3 },
          ],
        }),
      }),
    )

    await waitFor(() =>
      expect(screen.getByTestId('tab-paths').textContent).toBe('/|/docs|/chat'),
    )
  })

  it('hands a committed fresh record to its document without moving another document', async () => {
    seedShellTabs([{ id: 'existing', name: 'Files', path: '/files' }])
    let releaseMutation!: () => void
    const mutationRelease = new Promise<void>((resolve) => {
      releaseMutation = resolve
    })
    let mutationCommitted!: () => void
    const committed = new Promise<void>((resolve) => {
      mutationCommitted = resolve
    })
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: async (
          _name: string,
          _options: unknown,
          callback: (lock: object) => unknown,
        ) => {
          const result = await callback({})
          mutationCommitted()
          await mutationRelease
          return result
        },
      },
    })
    const store = new BrowserShellTabsStore()
    const entryA = {
      ...continuationEntry,
      incarnation: 'document-a',
    }
    const entryB = {
      kind: 'fresh' as const,
      path: '/files',
      params: {},
      incarnation: 'document-b',
    }
    const view = render(
      <>
        <ShellTabsProvider key="a" store={store} entry={entryA}>
          <ActiveTabProbe testId="active-tab-a" />
        </ShellTabsProvider>
        <ShellTabsProvider key="b" store={store} entry={entryB}>
          <ActiveTabProbe testId="active-tab-b" />
        </ShellTabsProvider>
      </>,
    )

    await committed
    const created = store
      .getSnapshot()
      .records.find((record) => record.id !== 'existing')
    expect(created).toBeDefined()

    view.rerender(
      <>
        <ShellTabsProvider key="a" store={store} entry={entryA}>
          <ActiveTabProbe testId="active-tab-a" />
        </ShellTabsProvider>
        <ShellTabsProvider
          key="b-continuation"
          store={store}
          entry={{ ...entryB, kind: 'continuation' }}
        >
          <ActiveTabProbe testId="active-tab-b" />
        </ShellTabsProvider>
      </>,
    )
    releaseMutation()

    await waitFor(() => {
      expect(screen.getByTestId('active-tab-a').textContent).toBe('existing')
      expect(screen.getByTestId('active-tab-b').textContent).toBe(created?.id)
    })
  })

  it('replaces the active tab path without creating a new tab', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Space', path: '/u/0/so/space' },
      { id: 'tab-2', name: 'Docs', path: '/docs' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ReplaceActiveTabPathProbe path="/u/0/so/space/-/files" />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('tab-count').textContent).toBe('2')
    expect(screen.getByTestId('tab-paths').textContent).toBe(
      '/u/0/so/space/-/files|/docs',
    )
  })

  it('emits an event after the active tab path commits', async () => {
    const listener = vi.fn()
    window.addEventListener(SHELL_TAB_PATH_COMMITTED_EVENT, listener)
    try {
      seedShellTabs([
        { id: 'tab-1', name: 'Space', path: '/u/0/so/space' },
        { id: 'tab-2', name: 'Docs', path: '/docs' },
      ])

      render(
        <ShellTabsProvider entry={continuationEntry}>
          <ReplaceActiveTabPathProbe path="/u/0/so/space/-/files" />
        </ShellTabsProvider>,
      )

      await waitFor(() => expect(listener).toHaveBeenCalled())
      const event = listener.mock.calls[0]?.[0]
      expect(event).toBeInstanceOf(CustomEvent)
      if (!(event instanceof CustomEvent)) {
        throw new Error('tab path commit event was not a CustomEvent')
      }
      expect(event.detail).toEqual({
        tabId: 'tab-1',
        path: '/u/0/so/space/-/files',
      })
    } finally {
      window.removeEventListener(SHELL_TAB_PATH_COMMITTED_EVENT, listener)
    }
  })

  it('normalizes loaded active tab selection to a real tab', () => {
    seedShellTabs([{ id: 'tab-1', name: 'Home', path: '/' }])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1')
  })

  it('creates a selected docs tab with the resolved Docs label', async () => {
    seedShellTabs([{ id: 'home', name: 'Home', path: '/' }])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <OpenDocsProbe />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Docs' }))

    await waitFor(() => {
      const tabs = JSON.parse(
        screen.getByTestId('tabs-json').textContent ?? '[]',
      )
      const docsTab = tabs.find((tab: { path: string }) => tab.path === '/docs')
      expect(docsTab).toMatchObject({ name: 'Docs', path: '/docs' })
      expect(screen.getByTestId('active-tab-id').textContent).toBe(docsTab.id)
    })
  })

  it('focuses an existing exact docs tab and preserves its custom name', async () => {
    seedShellTabs([
      { id: 'home', name: 'Home', path: '/' },
      {
        id: 'docs-tab',
        name: 'Docs',
        path: '/docs',
        customName: 'Reference',
      },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <OpenDocsProbe />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Docs' }))

    await waitFor(() => {
      const tabs = JSON.parse(
        screen.getByTestId('tabs-json').textContent ?? '[]',
      )
      expect(tabs).toHaveLength(2)
      expect(tabs[1]).toMatchObject({
        id: 'docs-tab',
        name: 'Docs',
        path: '/docs',
        customName: 'Reference',
      })
      expect(screen.getByTestId('active-tab-id').textContent).toBe('docs-tab')
    })
  })

  it('falls back to opening a selected Terminal tab when no active-tabset opener is registered', async () => {
    seedShellTabs([
      { id: 'settings-tab', name: 'Settings', path: '/u/7/settings/cli' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <OpenCliInActiveTabsetProbe />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open CLI terminal' }))

    await waitFor(() => {
      const tabs = JSON.parse(
        screen.getByTestId('tabs-json').textContent ?? '[]',
      )
      const terminalTab = tabs.find(
        (tab: { path: string }) => tab.path === '/u/7/settings/cli/terminal',
      )
      expect(tabs).toHaveLength(2)
      expect(terminalTab).toMatchObject({
        name: 'Terminal',
        path: '/u/7/settings/cli/terminal',
      })
      expect(screen.getByTestId('active-tab-id').textContent).toBe(
        terminalTab.id,
      )
    })
  })

  it('resets shared records to one fresh route and cleans Shell-local state', async () => {
    seedShellTabs([
      { id: 'old-tab', name: 'Old', path: '/old' },
      { id: 'other-tab', name: 'Other', path: '/other' },
    ])
    sessionStorage.setItem(
      shellTabStateStorageKey(continuationEntry.incarnation, 'old-tab'),
      JSON.stringify({ json: { count: 3 } }),
    )
    localStorage.setItem('tab-state-old-tab', JSON.stringify({ legacy: true }))

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ResetProbe />
      </ShellTabsProvider>,
    )
    const before = JSON.parse(
      localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY) ?? '{}',
    ) as { epoch: number }

    fireEvent.click(screen.getByRole('button', { name: 'Reset Shell Tabs' }))

    await waitFor(() => {
      const snapshot = JSON.parse(
        localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY) ?? '{}',
      ) as {
        epoch: number
        records: Array<{ id: string; path: string }>
      }
      expect(snapshot.epoch).toBe(before.epoch + 1)
      expect(snapshot.records).toHaveLength(1)
      expect(snapshot.records[0]?.path).toBe('/')
      expect(snapshot.records[0]?.id).not.toBe('old-tab')
    })
    expect(
      sessionStorage.getItem(
        shellTabStateStorageKey(continuationEntry.incarnation, 'old-tab'),
      ),
    ).toBeNull()
    expect(localStorage.getItem('tab-state-old-tab')).toBeNull()
  })

  it('renders a visible error when a shared mutation cannot commit', async () => {
    const raw = JSON.stringify({
      schemaVersion: 1,
      epoch: 0,
      revision: 0,
      records: [{ id: 'tab-1', name: 'Home', path: '/', creationSequence: 1 }],
    })
    const storage: Storage = {
      getItem: () => raw,
      setItem: () => {
        throw Object.assign(new Error('full'), { name: 'QuotaExceededError' })
      },
      removeItem: () => {},
      clear: () => {},
      key: () => null,
      length: 1,
    }
    const store = new BrowserShellTabsStore({ storage })

    render(
      <ShellTabsProvider entry={continuationEntry} store={store}>
        <MutationFailureProbe />
      </ShellTabsProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Trigger failed add' }))

    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toBe('quota'),
    )
  })

  it('removes obsolete and incarnation-scoped state after explicit close', async () => {
    seedShellTabs([
      { id: 'old-tab', name: 'Old', path: '/old' },
      { id: 'other-tab', name: 'Other', path: '/other' },
    ])
    sessionStorage.setItem(
      shellTabStateStorageKey(continuationEntry.incarnation, 'old-tab'),
      JSON.stringify({ json: { count: 3 } }),
    )
    localStorage.setItem('tab-state-old-tab', JSON.stringify({ legacy: true }))

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <CloseProbe tabId="old-tab" />
      </ShellTabsProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Close Shell Tab' }))

    await waitFor(() => {
      expect(
        readShellTabsSnapshot().records.map((record) => record.id),
      ).toEqual(['other-tab'])
    })
    expect(
      sessionStorage.getItem(
        shellTabStateStorageKey(continuationEntry.incarnation, 'old-tab'),
      ),
    ).toBeNull()
    expect(localStorage.getItem('tab-state-old-tab')).toBeNull()
  })

  it('preserves another document projection when closing its inactive record', async () => {
    seedShellTabs([
      { id: 'tab-a', name: 'A', path: '/a-only' },
      { id: 'shared', name: 'Shared Docs', path: '/docs' },
      { id: 'tab-b', name: 'B', path: '/b-only' },
    ])
    const store = new BrowserShellTabsStore()

    render(
      <>
        <ShellTabsProvider
          store={store}
          entry={{
            kind: 'handoff',
            path: '/a-only',
            params: {},
            tabId: 'tab-a',
            incarnation: 'document-a',
          }}
        >
          <TabsProjectionProbe testId="projection-a" />
        </ShellTabsProvider>
        <ShellTabsProvider
          store={store}
          entry={{
            kind: 'handoff',
            path: '/docs',
            params: {},
            tabId: 'shared',
            incarnation: 'document-b',
          }}
        >
          <CloseProbe tabId="shared" />
        </ShellTabsProvider>
      </>,
    )

    expect(
      JSON.parse(screen.getByTestId('projection-a').textContent ?? '{}'),
    ).toEqual({
      activeTabId: 'tab-a',
      tabs: [
        { id: 'tab-a', path: '/a-only' },
        { id: 'shared', path: '/docs' },
        { id: 'tab-b', path: '/b-only' },
      ],
    })
    fireEvent.click(screen.getByRole('button', { name: 'Close Shell Tab' }))

    await waitFor(() => {
      expect(
        JSON.parse(screen.getByTestId('projection-a').textContent ?? '{}'),
      ).toEqual({
        activeTabId: 'tab-a',
        tabs: [
          { id: 'tab-a', path: '/a-only' },
          { id: 'tab-b', path: '/b-only' },
        ],
      })
    })
  })

  it('normalizes a final explicit close to a fresh default record', async () => {
    seedShellTabs([{ id: 'only-tab', name: 'Only', path: '/only' }])
    sessionStorage.setItem(
      shellTabStateStorageKey(continuationEntry.incarnation, 'only-tab'),
      JSON.stringify({ json: { count: 3 } }),
    )
    localStorage.setItem('tab-state-only-tab', JSON.stringify({ legacy: true }))

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <CloseProbe tabId="only-tab" />
        <ActiveTabProbe />
        <TabPathProbe />
      </ShellTabsProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Close Shell Tab' }))

    await waitFor(() => {
      const records = readShellTabsSnapshot().records
      expect(records).toHaveLength(1)
      expect(records[0]).toMatchObject({ path: '/', name: 'Home' })
      expect(records[0]?.id).not.toBe('only-tab')
      expect(screen.getByTestId('active-tab-id').textContent).toBe(
        records[0]?.id,
      )
      expect(screen.getByTestId('tab-paths').textContent).toBe('/')
    })
    expect(
      sessionStorage.getItem(
        shellTabStateStorageKey(continuationEntry.incarnation, 'only-tab'),
      ),
    ).toBeNull()
    expect(localStorage.getItem('tab-state-only-tab')).toBeNull()
  })

  it('reports initialization failure through the visible error and toast owners', async () => {
    const previousLocks = Object.getOwnPropertyDescriptor(navigator, 'locks')
    vi.mocked(toast.error).mockClear()
    delete (navigator as { locks?: unknown }).locks
    try {
      const store = new BrowserShellTabsStore({ locks: undefined })
      render(
        <ShellTabsProvider entry={continuationEntry} store={store}>
          <MutationFailureProbe />
        </ShellTabsProvider>,
      )

      await waitFor(() =>
        expect(screen.getByRole('alert').textContent).toBe(
          'web-lock-unavailable',
        ),
      )
      expect(toast.error).toHaveBeenCalledWith(
        'Shell tab update failed',
        expect.objectContaining({
          description: 'Web Locks are required for Shell Tab mutations.',
        }),
      )
    } finally {
      if (previousLocks) {
        Object.defineProperty(navigator, 'locks', previousLocks)
      } else {
        delete (navigator as { locks?: unknown }).locks
      }
    }
  })
  it('continues the stored active ID for reload and same-entry state', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Home', path: '/' },
      { id: 'tab-2', name: 'Docs', path: '/docs' },
    ])
    writeShellDocumentState({
      incarnation: 'reload-incarnation',
      activeTabId: 'tab-2',
    })

    const reloadEntry = classifyShellDocumentEntry({
      path: '/docs',
      navigationType: 'reload',
    })
    expect(reloadEntry).toMatchObject({
      kind: 'continuation',
      incarnation: 'reload-incarnation',
    })

    render(
      <ShellTabsProvider entry={reloadEntry}>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2')
    expect(readShellDocumentState()).toMatchObject({
      incarnation: 'reload-incarnation',
      activeTabId: 'tab-2',
    })
  })

  it('creates a fresh entry despite cloned session state and removes old combined state', async () => {
    sessionStorage.setItem(
      'shell-tabs-state',
      JSON.stringify({
        tabs: [{ id: 'old-id', name: 'Old', path: '/old' }],
        activeTabId: 'old-id',
      }),
    )
    const freshEntry = classifyShellDocumentEntry({
      path: '/fresh',
      navigationType: 'navigate',
    })
    expect(freshEntry.kind).toBe('fresh')
    expect(freshEntry.incarnation).not.toBe('reload-incarnation')

    render(
      <ShellTabsProvider entry={freshEntry}>
        <ActiveTabProbe />
        <TabPathProbe />
      </ShellTabsProvider>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('tab-paths').textContent).toBe('/fresh')
    })
    expect(screen.getByTestId('active-tab-id').textContent).not.toBe('old-id')
    expect(sessionStorage.getItem('shell-tabs-state')).toBeNull()
    expect(localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY)).not.toContain(
      'old-id',
    )
  })

  it('selects an explicitly handed-off shared record in its new incarnation', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Home', path: '/' },
      { id: 'tab-2', name: 'Docs', path: '/docs' },
    ])
    const handoffEntry = classifyShellDocumentEntry({
      hash: '#/docs?shellTabId=tab-2',
      navigationType: 'navigate',
    })

    render(
      <ShellTabsProvider entry={handoffEntry}>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2')
  })

  it('updates an explicit handoff ID when the active record changes', async () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Home', path: '/' },
      { id: 'tab-2', name: 'Docs', path: '/docs' },
    ])
    window.location.hash = '#/?shellTabId=tab-1&spaceId=space-1'
    const entry = classifyShellDocumentEntry({
      hash: '#/?shellTabId=tab-1&spaceId=space-1',
      navigationType: 'navigate',
      incarnation: 'selected-handoff',
    })
    const first = render(
      <ShellTabsProvider entry={entry}>
        <OpenDocsProbe />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Docs' }))
    await waitFor(() => {
      expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2')
      const params = new URLSearchParams(window.location.hash.split('?')[1])
      expect(params.get('shellTabId')).toBe('tab-2')
      expect(params.get('spaceId')).toBe('space-1')
    })

    first.unmount()
    const reloadEntry = classifyShellDocumentEntry({
      hash: window.location.hash,
      navigationType: 'reload',
      incarnation: 'selected-handoff',
    })
    render(
      <ShellTabsProvider entry={reloadEntry}>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2')
  })

  it('retains shared records after the last Shell Tabs provider unmounts', async () => {
    const restoreTestBrowser = installShellTabTestBrowser()
    try {
      seedShellTabs([
        { id: 'tab-1', name: 'Home', path: '/' },
        { id: 'tab-2', name: 'Docs', path: '/docs' },
      ])
      const view = render(
        <ShellTabsProvider entry={continuationEntry}>
          <ActiveTabProbe />
        </ShellTabsProvider>,
      )

      await waitFor(() =>
        expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1'),
      )
      view.unmount()
      expect(
        readShellTabsSnapshot().records.map((record) => record.id),
      ).toEqual(['tab-1', 'tab-2'])
    } finally {
      restoreTestBrowser()
    }
  })

  it('repairs a removed explicit ID and remains stable across reload', async () => {
    seedShellTabs([])
    const entry = classifyShellDocumentEntry({
      hash: '#/docs?shellTabId=removed-tab&spaceId=space-1',
      navigationType: 'navigate',
      incarnation: 'removed-handoff',
    })
    const first = render(
      <ShellTabsProvider entry={entry}>
        <ActiveTabProbe />
        <TabPathProbe />
      </ShellTabsProvider>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('tab-paths').textContent).toBe('/docs')
    })
    const replacementId = readShellTabsSnapshot().records[0]?.id
    expect(replacementId).toBeTruthy()
    const params = new URLSearchParams(window.location.hash.split('?')[1])
    expect(params.get('shellTabId')).toBe(replacementId)
    expect(params.get('spaceId')).toBe('space-1')

    first.unmount()
    const reloadEntry = classifyShellDocumentEntry({
      hash: window.location.hash,
      navigationType: 'reload',
      incarnation: 'reload-handoff',
    })
    expect(reloadEntry).toMatchObject({
      kind: 'handoff',
      tabId: replacementId,
    })
    render(
      <ShellTabsProvider entry={reloadEntry}>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )
    expect(screen.getByTestId('active-tab-id').textContent).toBe(replacementId)
  })

  it('repairs malformed explicit IDs while retaining other named params', async () => {
    cleanup()
    seedShellTabs([])
    const entry = classifyShellDocumentEntry({
      hash: '#/docs?shellTabId=bad%20id&spaceId=space-2',
      navigationType: 'navigate',
      incarnation: 'malformed-handoff',
    })
    render(
      <ShellTabsProvider entry={entry}>
        <TabPathProbe />
      </ShellTabsProvider>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('tab-paths').textContent).toBe('/docs')
    })
    const replacementId = readShellTabsSnapshot().records[0]?.id
    expect(replacementId).toBeTruthy()
    const params = new URLSearchParams(window.location.hash.split('?')[1])
    expect(params.get('shellTabId')).toBe(replacementId)
    expect(params.get('spaceId')).toBe('space-2')
  })

  it('keeps Shell tab state local despite an inherited backend accessor', () => {
    seedShellTabs([{ id: 'tab-1', name: 'Home', path: '/' }])
    const inheritedAccessor: StateAtomAccessor = {
      value: vi.fn(async () => {
        throw new Error('inherited backend accessor must not be used')
      }),
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    const entry = { ...continuationEntry, incarnation: 'local-state-owner' }

    render(
      <StateNamespaceProvider stateAtomAccessor={inheritedAccessor}>
        <ShellTabsProvider entry={entry}>
          <ShellTabStateProvider tabId="tab-1">
            <TabStateProbe />
          </ShellTabStateProvider>
        </ShellTabsProvider>
      </StateNamespaceProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Increment state' }))
    expect(
      sessionStorage.getItem(
        shellTabStateStorageKey('local-state-owner', 'tab-1'),
      ),
    ).not.toBeNull()
    expect(inheritedAccessor.value).not.toHaveBeenCalled()
  })

  it('keeps named tab state across reload, isolates fresh entries, and prunes remote close', async () => {
    seedShellTabs([{ id: 'tab-1', name: 'Home', path: '/' }])
    const entry = { ...continuationEntry, incarnation: 'state-incarnation' }

    const first = render(
      <ShellTabsProvider entry={entry}>
        <ShellTabStateProvider tabId="tab-1">
          <TabStateProbe />
        </ShellTabStateProvider>
      </ShellTabsProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Increment state' }))
    expect(screen.getByTestId('tab-state-count').textContent).toBe('1')
    expect(
      sessionStorage.getItem(
        shellTabStateStorageKey('state-incarnation', 'tab-1'),
      ),
    ).not.toBeNull()

    first.unmount()
    render(
      <ShellTabsProvider entry={entry}>
        <ShellTabStateProvider tabId="tab-1">
          <TabStateProbe />
        </ShellTabStateProvider>
      </ShellTabsProvider>,
    )
    expect(screen.getByTestId('tab-state-count').textContent).toBe('1')

    cleanup()
    const freshEntry = {
      ...entry,
      kind: 'fresh' as const,
      incarnation: 'fresh-state-incarnation',
      path: '/fresh',
    }
    render(
      <ShellTabsProvider entry={freshEntry}>
        <ShellTabStateProvider tabId="tab-1">
          <TabStateProbe />
        </ShellTabStateProvider>
      </ShellTabsProvider>,
    )
    expect(screen.getByTestId('tab-state-count').textContent).toBe('0')

    cleanup()
    render(
      <ShellTabsProvider entry={entry}>
        <ShellTabStateProvider tabId="tab-1">
          <TabStateProbe />
        </ShellTabStateProvider>
      </ShellTabsProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Increment state' }))
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: BROWSER_SHELL_TABS_STORAGE_KEY,
        newValue: JSON.stringify({
          schemaVersion: 1,
          epoch: 0,
          revision: 99,
          records: [],
        }),
      }),
    )
    await waitFor(() =>
      expect(
        sessionStorage.getItem(
          shellTabStateStorageKey('state-incarnation', 'tab-1'),
        ),
      ).toBeNull(),
    )
  })
})
