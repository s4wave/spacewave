import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { getAppPath } from '@s4wave/web/router/app-path.js'

import { ShellAppPanel } from './ShellAppPanel.js'
import { ShellTabsProvider, SHELL_TABS_STORAGE_KEY } from './ShellTabContext.js'

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
      </div>
    )
  }

  return { AppRoutes: MockAppRoutes }
})

function seedTabs(activeTabId: string) {
  sessionStorage.setItem(
    SHELL_TABS_STORAGE_KEY,
    JSON.stringify({
      tabs: [
        { id: 'tab-1', name: 'Home', path: '/' },
        { id: 'tab-2', name: 'Quickstart', path: '/quickstart/drive' },
      ],
      activeTabId,
    }),
  )
}

describe('ShellAppPanel', () => {
  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    window.history.replaceState({}, '', '/')
  })

  it('ignores inactive panel navigation', () => {
    seedTabs('tab-2')

    render(
      <ShellTabsProvider>
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

  it('syncs active panel navigation to the global app path', () => {
    seedTabs('tab-2')

    render(
      <ShellTabsProvider>
        <ShellAppPanel
          tabId="tab-2"
          initialPath="/quickstart/drive"
          namespace={['test', 'tab-2']}
          syncAppPath
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))

    expect(getAppPath()).toBe('/docs')
  })

  it('adds selected shell tabs from embedded tab-context requests', async () => {
    seedTabs('tab-1')

    render(
      <ShellTabsProvider>
        <ShellAppPanel
          tabId="tab-1"
          initialPath="/"
          namespace={['test', 'tab-1']}
        />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Add Docs Tab' }))

    await waitFor(() => {
      const stored = JSON.parse(
        sessionStorage.getItem(SHELL_TABS_STORAGE_KEY) ?? 'null',
      ) as {
        activeTabId: string
        tabs: Array<{ id: string; name: string; path: string }>
      }
      const activeTab = stored.tabs.find((tab) => tab.id === stored.activeTabId)
      expect(activeTab).toMatchObject({ name: 'Docs', path: '/docs' })
    })
  })
})
