import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

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
  const { useNavigate, usePath } = await import('@s4wave/web/router/router.js')

  function MockAppRoutes() {
    const navigate = useNavigate()
    const path = usePath()

    return (
      <div>
        <span data-testid="path">{path}</span>
        <button onClick={() => navigate({ path: '/docs' })} type="button">
          Docs
        </button>
      </div>
    )
  }

  return { AppRoutes: MockAppRoutes }
})

function seedTabs(activeTabId: string) {
  localStorage.setItem(
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
})
