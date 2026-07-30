import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { getAppPath, setAppPath } from '@s4wave/web/router/app-path.js'

import { ShellAppPanel } from './ShellAppPanel.js'
import { ShellTabsProvider, useShellTabs } from './ShellTabContext.js'
import {
  installShellTabTestBrowser,
  readShellTabsSnapshot,
  seedShellTabs,
  type ShellTabTestBrowser,
} from './ShellTabTestHarness.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'

const continuationEntry: ShellDocumentEntry = {
  kind: 'continuation',
  path: '/',
  params: {},
  incarnation: 'test-document',
}
function handoffEntry(tabId: string): ShellDocumentEntry {
  return { ...continuationEntry, kind: 'handoff', tabId }
}

vi.mock('@s4wave/web/frame/bottom-bar-root.js', () => ({
  BottomBarRoot: ({ children }: { children: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/state/index.js', async () => {
  const [React, actual] = await Promise.all([
    import('react'),
    vi.importActual<typeof import('@s4wave/web/state/index.js')>(
      '@s4wave/web/state/index.js',
    ),
  ])
  return {
    ...actual,
    StateNamespaceProvider: ({
      children,
    }: {
      children: ReactNode
      namespace?: string[]
    }) => <>{children}</>,
    useStateAtom: <T,>(_: unknown, __: string, initialValue: T) =>
      React.useState(initialValue),
  }
})

vi.mock('./routes/AppRoutes.js', async () => {
  const [{ useNavigate, usePath }, { useTabContext }, { ObjectLayoutTab }] =
    await Promise.all([
      import('@s4wave/web/router/router.js'),
      import('@s4wave/web/object/TabContext.js'),
      import('@s4wave/sdk/layout/world/world.pb.js'),
    ])

  function MockAppRoutes() {
    const navigate = useNavigate()
    const path = usePath()
    const tabContext = useTabContext()

    return (
      <div>
        <span data-testid="path">{path}</span>
        <button onClick={() => navigate({ path: '/docs' })} type="button">
          Docs
        </button>
        <button
          onClick={() =>
            void tabContext?.addTab({
              tab: {
                data: ObjectLayoutTab.toBinary({ path: '/docs' }),
              },
              select: true,
            })
          }
          type="button"
        >
          Add Docs Tab
        </button>
        <button
          onClick={() => void tabContext?.navigateTab('/docs')}
          type="button"
        >
          Navigate Tab
        </button>
      </div>
    )
  }

  return { AppRoutes: MockAppRoutes }
})

function seedTabs(_activeTabId: string) {
  seedShellTabs([
    { id: 'tab-1', name: 'Home', path: '/' },
    { id: 'tab-2', name: 'Quickstart', path: '/quickstart/drive' },
  ])
}
function ActiveTabProbe() {
  const { activeTabId } = useShellTabs()
  return <span data-testid="active-tab-id">{activeTabId}</span>
}
function ActivateTabButton({ tabId }: { tabId: string }) {
  const { setActiveTabId } = useShellTabs()
  return (
    <button onClick={() => setActiveTabId(tabId)} type="button">
      Activate {tabId}
    </button>
  )
}

describe('ShellAppPanel', () => {
  let restoreTestBrowser: ShellTabTestBrowser | undefined

  beforeEach(() => {
    restoreTestBrowser = installShellTabTestBrowser()
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    window.history.replaceState({}, '', '/')
    restoreTestBrowser?.()
    restoreTestBrowser = undefined
  })

  it('ignores inactive panel navigation', () => {
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ShellAppPanel
          tabId="tab-1"
          initialPath="/"
          namespace={['test', 'tab-1']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))

    expect(screen.getByTestId('path').textContent).toBe('/')
    expect(getAppPath()).toBe('/')
  })

  it('syncs active panel navigation to the global app path', async () => {
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ShellAppPanel
          tabId="tab-2"
          initialPath="/quickstart/drive"
          namespace={['test', 'tab-2']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))

    await waitFor(() => expect(getAppPath()).toBe('/docs'))
  })

  it('syncs active tab-context navigation to the global app path', async () => {
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ShellAppPanel
          tabId="tab-2"
          initialPath="/quickstart/drive"
          namespace={['test', 'tab-2']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Navigate Tab' }))

    await waitFor(() => {
      const record = readShellTabsSnapshot().records.find(
        (tab) => tab.id === 'tab-2',
      )
      expect(record?.path).toBe('/docs')
      expect(getAppPath()).toBe('/docs')
    })
  })

  it('keeps the global app path when an inactive tab context navigates', async () => {
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ShellAppPanel
          tabId="tab-1"
          initialPath="/"
          namespace={['test', 'tab-1']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Navigate Tab' }))

    await waitFor(() => {
      const record = readShellTabsSnapshot().records.find(
        (tab) => tab.id === 'tab-1',
      )
      expect(record?.path).toBe('/docs')
    })
    expect(getAppPath()).toBe('/')
  })

  it('keeps the global app path when the panel deactivates before the path commits', async () => {
    if (!restoreTestBrowser) throw new Error('test browser not installed')
    const browser = restoreTestBrowser
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ActiveTabProbe />
        <ActivateTabButton tabId="tab-1" />
        <ShellAppPanel
          tabId="tab-2"
          initialPath="/quickstart/drive"
          namespace={['test', 'tab-2']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    const blocked = browser.blockNextMutation()
    fireEvent.click(screen.getByRole('button', { name: 'Navigate Tab' }))
    await blocked

    fireEvent.click(screen.getByRole('button', { name: 'Activate tab-1' }))
    await waitFor(() =>
      expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1'),
    )

    browser.releaseBlockedMutation()

    await waitFor(() => {
      const record = readShellTabsSnapshot().records.find(
        (tab) => tab.id === 'tab-2',
      )
      expect(record?.path).toBe('/docs')
    })
    expect(getAppPath()).toBe('/')
  })

  it('syncs the global app path when the panel activates before the path commits', async () => {
    if (!restoreTestBrowser) throw new Error('test browser not installed')
    const browser = restoreTestBrowser
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ActiveTabProbe />
        <ActivateTabButton tabId="tab-1" />
        <ShellAppPanel
          tabId="tab-1"
          initialPath="/"
          namespace={['test', 'tab-1']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    const blocked = browser.blockNextMutation()
    fireEvent.click(screen.getByRole('button', { name: 'Navigate Tab' }))
    await blocked

    fireEvent.click(screen.getByRole('button', { name: 'Activate tab-1' }))
    await waitFor(() =>
      expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1'),
    )

    browser.releaseBlockedMutation()

    await waitFor(() => {
      const record = readShellTabsSnapshot().records.find(
        (tab) => tab.id === 'tab-1',
      )
      expect(record?.path).toBe('/docs')
      expect(getAppPath()).toBe('/docs')
    })
  })

  it('leaves a newer document navigation in place when an older path commits', async () => {
    if (!restoreTestBrowser) throw new Error('test browser not installed')
    const browser = restoreTestBrowser
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ActiveTabProbe />
        <ShellAppPanel
          tabId="tab-2"
          initialPath="/quickstart/drive"
          namespace={['test', 'tab-2']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    const blocked = browser.blockNextMutation()
    fireEvent.click(screen.getByRole('button', { name: 'Navigate Tab' }))
    await blocked

    // The user moves the document while the commit waits on the Web Lock.
    setAppPath('/files')

    browser.releaseBlockedMutation()

    await waitFor(() => {
      const record = readShellTabsSnapshot().records.find(
        (tab) => tab.id === 'tab-2',
      )
      expect(record?.path).toBe('/docs')
    })
    expect(getAppPath()).toBe('/files')
  })

  it('leaves a document navigated away and back in place when an older path commits', async () => {
    if (!restoreTestBrowser) throw new Error('test browser not installed')
    const browser = restoreTestBrowser
    seedTabs('tab-2')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-2')}>
        <ActiveTabProbe />
        <ShellAppPanel
          tabId="tab-2"
          initialPath="/quickstart/drive"
          namespace={['test', 'tab-2']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    const blocked = browser.blockNextMutation()
    fireEvent.click(screen.getByRole('button', { name: 'Navigate Tab' }))
    await blocked

    // A round trip through history restores the path the commit started
    // from, so only a navigation counter can tell that the user moved.
    setAppPath('/files')
    setAppPath('/')

    browser.releaseBlockedMutation()

    await waitFor(() => {
      const record = readShellTabsSnapshot().records.find(
        (tab) => tab.id === 'tab-2',
      )
      expect(record?.path).toBe('/docs')
    })
    expect(getAppPath()).toBe('/')
  })

  it('adds selected shell tabs from embedded tab-context requests', async () => {
    seedTabs('tab-1')

    render(
      <ShellTabsProvider entry={handoffEntry('tab-1')}>
        <ActiveTabProbe />
        <ShellAppPanel
          tabId="tab-1"
          initialPath="/"
          namespace={['test', 'tab-1']}
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Add Docs Tab' }))

    await waitFor(() => {
      const docsTab = readShellTabsSnapshot().records.find(
        (tab) => tab.path === '/docs',
      )
      expect(docsTab).toMatchObject({ name: 'Docs', path: '/docs' })
      expect(screen.getByTestId('active-tab-id').textContent).toBe(docsTab?.id)
    })
  })
})
