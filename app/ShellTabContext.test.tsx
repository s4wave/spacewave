import { useEffect } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import {
  ShellTabsProvider,
  SHELL_TABS_STORAGE_KEY,
  useShellTabs,
} from './ShellTabContext.js'

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

function ActiveTabProbe() {
  const { activeTabId } = useShellTabs()
  return <div data-testid="active-tab-id">{activeTabId}</div>
}

function TabPathProbe() {
  const { tabs } = useShellTabs()
  return (
    <div data-testid="tab-paths">{tabs.map((tab) => tab.path).join('|')}</div>
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
  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
  })

  it('treats same-path tab updates as a no-op', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [{ id: 'tab-1', name: 'Home', path: '/' }],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <NoopPathUpdateProbe />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('tab-count').textContent).toBe('1')
  })

  it('treats semantically equal tab arrays as a no-op', async () => {
    const onCommit = vi.fn()
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [{ id: 'tab-1', name: 'Home', path: '/' }],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <NoopSetTabsProbe onCommit={onCommit} />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('probe-mounted').textContent).toBe('mounted')
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(onCommit).toHaveBeenCalledTimes(1)
  })

  it('ignores cross-window tab storage changes', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Home', path: '/' },
          { id: 'tab-2', name: 'Docs', path: '/docs' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ActiveTabProbe />
        <TabPathProbe />
      </ShellTabsProvider>,
    )

    window.dispatchEvent(
      new StorageEvent('storage', {
        key: SHELL_TABS_STORAGE_KEY,
        newValue: JSON.stringify({
          tabs: [
            { id: 'tab-1', name: 'Home', path: '/' },
            { id: 'tab-2', name: 'Docs', path: '/docs' },
            { id: 'tab-3', name: 'Chat', path: '/chat' },
          ],
          activeTabId: 'tab-2',
        }),
      }),
    )

    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1')
    expect(screen.getByTestId('tab-paths').textContent).toBe('/|/docs')
  })

  it('replaces the active tab path without creating a new tab', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Space', path: '/u/0/so/space' },
          { id: 'tab-2', name: 'Docs', path: '/docs' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ReplaceActiveTabPathProbe path="/u/0/so/space/-/files" />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('tab-count').textContent).toBe('2')
    expect(screen.getByTestId('tab-paths').textContent).toBe(
      '/u/0/so/space/-/files|/docs',
    )
  })

  it('normalizes loaded active tab selection to a real tab', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [{ id: 'tab-1', name: 'Home', path: '/' }],
        activeTabId: 'missing-tab',
      }),
    )

    render(
      <ShellTabsProvider>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1')
  })

  it('creates a selected docs tab with the resolved Docs label', async () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [{ id: 'home', name: 'Home', path: '/' }],
        activeTabId: 'home',
      }),
    )

    render(
      <ShellTabsProvider>
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
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'home', name: 'Home', path: '/' },
          {
            id: 'docs-tab',
            name: 'Tab',
            path: '/docs',
            customName: 'Reference',
          },
        ],
        activeTabId: 'home',
      }),
    )

    render(
      <ShellTabsProvider>
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

  it('falls back to opening a selected new tab when no active-tabset opener is registered', async () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'settings-tab', name: 'Settings', path: '/u/7/settings/cli' },
        ],
        activeTabId: 'settings-tab',
      }),
    )

    render(
      <ShellTabsProvider>
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
        name: 'Settings',
        path: '/u/7/settings/cli/terminal',
      })
      expect(screen.getByTestId('active-tab-id').textContent).toBe(
        terminalTab.id,
      )
    })
  })
})
