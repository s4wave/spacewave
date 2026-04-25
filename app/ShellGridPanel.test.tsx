import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import { ShellGridPanel } from './ShellGridPanel.js'
import { ShellTabsProvider, SHELL_TABS_STORAGE_KEY } from './ShellTabContext.js'

vi.mock('@s4wave/web/frame/bottom-bar-root.js', () => ({
  BottomBarRoot: ({ children }: { children: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/state/index.js', async () => {
  const React = await import('react')
  const actual = await vi.importActual<
    typeof import('@s4wave/web/state/index.js')
  >('@s4wave/web/state/index.js')
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
  const { useHistory } = await import('@s4wave/web/router/HistoryRouter.js')
  const { useTabId } = await import('@s4wave/web/object/TabContext.js')
  const { useNavigate, usePath } = await import('@s4wave/web/router/router.js')

  function MockAppRoutes() {
    const history = useHistory()
    const navigate = useNavigate()
    const path = usePath()
    const tabId = useTabId()

    return (
      <div>
        <span data-testid="path">{path}</span>
        <span data-testid="tab-id">{tabId ?? ''}</span>
        <span data-testid="can-go-back">
          {history?.canGoBack ? 'true' : 'false'}
        </span>
        <button onClick={() => navigate({ path: '/docs' })} type="button">
          Docs
        </button>
        <button onClick={() => history?.goBack()} type="button">
          Back
        </button>
      </div>
    )
  }

  return { AppRoutes: MockAppRoutes }
})

describe('ShellGridPanel', () => {
  afterEach(() => {
    cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  it('reuses the shared app routes and keeps navigation and back history inside the grid panel', () => {
    localStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [{ id: 'tab-1', name: 'Home', path: '/' }],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridPanel tabId="tab-1" />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('path').textContent).toBe('/')
    expect(screen.getByTestId('tab-id').textContent).toBe('tab-1')
    expect(screen.getByTestId('can-go-back').textContent).toBe('false')

    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))

    expect(screen.getByTestId('path').textContent).toBe('/docs')
    expect(screen.getByTestId('can-go-back').textContent).toBe('true')
    expect(window.location.hash).toBe('')

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    expect(screen.getByTestId('path').textContent).toBe('/')
    expect(window.location.hash).toBe('')
  })
})
