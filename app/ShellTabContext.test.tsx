import { useEffect } from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'

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

function ActiveTabProbe() {
  const { activeTabId } = useShellTabs()
  return <div data-testid="active-tab-id">{activeTabId}</div>
}

describe('ShellTabContext', () => {
  afterEach(() => {
    cleanup()
    localStorage.clear()
  })

  it('treats same-path tab updates as a no-op', () => {
    localStorage.setItem(
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

  it('preserves active tab selection when hydrating external tab changes', async () => {
    localStorage.setItem(
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

    await waitFor(() => {
      expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1')
    })
  })

  it('falls back locally when an external tab change removes the active tab', async () => {
    localStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Home', path: '/' },
          { id: 'tab-2', name: 'Docs', path: '/docs' },
        ],
        activeTabId: 'tab-2',
      }),
    )

    render(
      <ShellTabsProvider>
        <ActiveTabProbe />
      </ShellTabsProvider>,
    )

    window.dispatchEvent(
      new StorageEvent('storage', {
        key: SHELL_TABS_STORAGE_KEY,
        newValue: JSON.stringify({
          tabs: [{ id: 'tab-1', name: 'Home', path: '/' }],
          activeTabId: 'tab-1',
        }),
      }),
    )

    await waitFor(() => {
      expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1')
    })
  })
})
