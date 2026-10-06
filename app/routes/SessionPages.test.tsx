import { useState } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { useNavLinks } from '@s4wave/app/nav-links.js'
import { MarkdownLink } from '@s4wave/app/docs/MarkdownLink.js'
import { StaticProvider } from '@s4wave/app/prerender/StaticContext.js'
import { SessionIndexContext } from '@s4wave/web/contexts/SessionIndexContext.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { HistoryRouter } from '@s4wave/web/router/HistoryRouter.js'
import {
  Route,
  RouterProvider,
  Routes,
  type To,
} from '@s4wave/web/router/router.js'

import { SessionPages } from './SessionPages.js'
import { SessionPageRoutes } from './SessionPageRoutes.js'

function Navigation() {
  const nav = useNavLinks()
  return (
    <>
      <button type="button" onClick={nav.docs}>
        Docs
      </button>
      <button type="button" onClick={nav.legal}>
        Legal
      </button>
      <button type="button" onClick={nav.blog}>
        Blog
      </button>
      <button type="button" onClick={nav.support}>
        Support
      </button>
      <button type="button" onClick={nav.download}>
        Download
      </button>
      <button type="button" onClick={nav.changelog}>
        Changelog
      </button>
    </>
  )
}

function SessionHarness({ initialPath = '/u/7' }: { initialPath?: string }) {
  const [path, setPath] = useState(initialPath)
  return (
    <HistoryRouter path={path} onNavigate={(to) => setPath(to.path)}>
      <SessionIndexContext value={7}>
        <output data-testid="path">{path}</output>
        {path === '/u/7' ? <Navigation /> : <SessionPages />}
      </SessionIndexContext>
    </HistoryRouter>
  )
}

describe('session informational pages', () => {
  afterEach(cleanup)

  it('renders the registered session footer on a nested informational route', async () => {
    render(
      <RouterProvider path="/u/7/legal/tos" onNavigate={() => {}}>
        <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
          <BottomBarLevel
            id="account"
            button={() => <button type="button">Session 7</button>}
          >
            <SessionIndexContext value={7}>
              <Routes>
                <Route path="/u/:sessionIndex/*">
                  <Routes>{SessionPageRoutes}</Routes>
                </Route>
              </Routes>
            </SessionIndexContext>
          </BottomBarLevel>
        </BottomBarRoot>
      </RouterProvider>,
    )
    expect(
      await screen.findByRole('heading', { name: 'Terms of Service' }),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Session 7' })).toBeTruthy()
  })

  it('keeps docs browsing, search, previous/next, and home in the session', () => {
    render(<SessionHarness />)
    fireEvent.click(screen.getByRole('button', { name: 'Docs' }))
    expect(screen.getByTestId('path').textContent).toBe('/u/7/docs')

    fireEvent.click(screen.getByRole('button', { name: /Users/ }))
    expect(screen.getByTestId('path').textContent).toBe('/u/7/docs/users')
    fireEvent.click(
      screen.getByRole('button', { name: /Create your first Space/ }),
    )
    expect(
      screen.getByRole('heading', { name: 'Create Your First Space' }),
    ).toBeTruthy()
    expect(screen.getByTestId('path').textContent).toBe(
      '/u/7/docs/users/start/create-your-first-space',
    )

    const articlePath = screen.getByTestId('path').textContent
    fireEvent.click(screen.getByRole('button', { name: /^Next/ }))
    expect(screen.getByTestId('path').textContent).toMatch(/^\/u\/7\/docs\//)
    fireEvent.click(screen.getByRole('button', { name: /^Previous/ }))
    expect(screen.getByTestId('path').textContent).toBe(articlePath)

    fireEvent.change(screen.getByPlaceholderText('Search docs…'), {
      target: { value: 'backup' },
    })
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Backup and Lock Setup',
      }),
    )
    expect(screen.getByTestId('path').textContent).toBe(
      '/u/7/docs/users/accounts/backup-and-lock-setup',
    )
    expect(
      screen.getByRole('link', { name: 'Terms' }).getAttribute('href'),
    ).toBe('#/u/7/legal/tos')

    fireEvent.click(
      screen.getByRole('button', { name: 'Back to Documentation' }),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Back to Home' }))
    expect(screen.getByTestId('path').textContent).toBe('/u/7')
  })

  it('opens legal pages with scoped body and footer links, then returns to the dashboard', () => {
    render(<SessionHarness />)
    fireEvent.click(screen.getByRole('button', { name: 'Legal' }))
    expect(
      screen.getByRole('heading', { name: 'Terms of Service' }),
    ).toBeTruthy()
    expect(screen.getByTestId('path').textContent).toBe('/u/7/legal/tos')
    expect(
      screen.getByRole('link', { name: 'Privacy Policy' }).getAttribute('href'),
    ).toBe('#/u/7/legal/privacy')
    expect(
      screen.getByRole('link', { name: 'Download' }).getAttribute('href'),
    ).toBe('#/u/7/download')
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.getByTestId('path').textContent).toBe('/u/7')
  })

  it.each([
    [
      '/u/7/docs/users/cli/install',
      '/u/7/docs/users/cli/command-line-basics',
      'Command Line Basics',
    ],
    ['/u/7/docs/unknown', '/u/7/docs', 'Documentation'],
    ['/u/7/legal/', '/u/7/legal/', 'Terms of Service'],
  ])(
    'resolves deep link %s within the session',
    async (initialPath, expectedPath, title) => {
      render(<SessionHarness initialPath={initialPath} />)
      expect(await screen.findByRole('heading', { name: title })).toBeTruthy()
      expect(screen.getByTestId('path').textContent).toBe(expectedPath)
    },
  )

  it('uses the session dashboard for Back when a deep link has no history', () => {
    render(<SessionHarness initialPath="/u/7/legal/privacy" />)
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.getByTestId('path').textContent).toBe('/u/7')
  })

  it.each([0, 7])(
    'keeps navigation and markdown links in their public or session context (%i)',
    (sessionIndex) => {
      const destinations: To[] = []
      render(
        <RouterProvider path="/" onNavigate={(to) => destinations.push(to)}>
          <SessionIndexContext value={sessionIndex}>
            <Navigation />
            <MarkdownLink href="/docs/users">Guide</MarkdownLink>
          </SessionIndexContext>
        </RouterProvider>,
      )
      for (const name of [
        'Docs',
        'Legal',
        'Blog',
        'Support',
        'Download',
        'Changelog',
      ]) {
        fireEvent.click(screen.getByRole('button', { name }))
      }
      expect(destinations.map((to) => to.path)).toEqual(
        sessionIndex
          ? [
              '/u/7/docs',
              '/u/7/legal/tos',
              '/u/7/blog',
              '/u/7/community',
              '/u/7/download',
              '/u/7/changelog',
            ]
          : ['/docs', '/tos', '/blog', '/community', '/download', '/changelog'],
      )
      expect(
        screen.getByRole('link', { name: 'Guide' }).getAttribute('href'),
      ).toBe(sessionIndex ? '#/u/7/docs/users' : '#/docs/users')
    },
  )

  it('preserves crawlable public links while prerendering', () => {
    render(
      <StaticProvider>
        <MarkdownLink href="/tos">Terms</MarkdownLink>
      </StaticProvider>,
    )
    expect(screen.getByRole('link').getAttribute('href')).toBe('/tos')
  })
})
