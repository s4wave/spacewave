import { useState } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { Routes, RouterProvider, type To } from '@s4wave/web/router/router.js'
import { DocsRoutes } from '@s4wave/app/routes/DocsRoutes.js'

function DocsRouteHarness({ initialPath }: { initialPath: string }) {
  const [path, setPath] = useState(initialPath)
  const navigate = (to: To) => setPath(to.path)

  return (
    <RouterProvider path={path} onNavigate={navigate}>
      <Routes fullPath>{DocsRoutes}</Routes>
    </RouterProvider>
  )
}

function docsPageBodyTextLength() {
  return (
    document.querySelector('main .docs-prose')?.textContent?.trim().length ?? 0
  )
}

describe('DocsRoutes browser smoke', () => {
  beforeEach(async () => {
    await cleanup()
    localStorage.clear()
  })

  it('renders hub, site home, article, search, source link, and navigation', async () => {
    await render(<DocsRouteHarness initialPath="/docs" />)

    await expect
      .element(page.getByRole('heading', { name: 'Documentation' }))
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: /Users/ }))
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: /Self-Hosters/ }))
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: /Developers/ }))
      .toBeInTheDocument()

    await page.getByRole('button', { name: /Users/ }).click()

    await expect
      .element(page.getByRole('button', { name: /Create your first Space/ }))
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: /Drive and files/ }))
      .toBeInTheDocument()

    await page.getByRole('button', { name: /Create your first Space/ }).click()

    await expect
      .element(page.getByRole('heading', { name: 'Create Your First Space' }))
      .toBeInTheDocument()
    await expect.element(page.getByText('Previous')).toBeInTheDocument()
    await expect.element(page.getByText('Next')).toBeInTheDocument()

    await expect
      .poll(() =>
        document
          .querySelector<HTMLAnchorElement>(
            'a[title="Open raw Markdown on GitHub"]',
          )
          ?.getAttribute('href'),
      )
      .toBe(
        'https://raw.githubusercontent.com/s4wave/spacewave/master/app/docs/content/users/start/02-create-your-first-space.md',
      )

    await page.getByPlaceholder('Search docs…').fill('backup')
    await expect
      .element(
        page.getByRole('button', {
          name: 'Backup and Lock Setup',
          exact: true,
        }),
      )
      .toBeInTheDocument()
    await page
      .getByRole('button', { name: 'Backup and Lock Setup', exact: true })
      .click()
    await expect
      .element(page.getByRole('heading', { name: 'Backup and Lock Setup' }))
      .toBeInTheDocument()
  })

  it('renders developer and self-hoster entry routes', async () => {
    await render(
      <DocsRouteHarness initialPath="/docs/developers/cli/cli-reference" />,
    )

    await expect
      .poll(() => document.querySelector('main h1')?.textContent)
      .toBe('CLI Reference')
    await expect.poll(docsPageBodyTextLength).toBeGreaterThan(0)

    await cleanup()

    await render(<DocsRouteHarness initialPath="/docs/self-hosters" />)

    await expect
      .poll(() => document.querySelector('main h1')?.textContent)
      .toBe('Self-Hosters')
    await expect
      .element(
        page.getByRole('button', { name: /Choose how to run Spacewave/ }),
      )
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: /Storage modes/ }))
      .toBeInTheDocument()
  })

  it('redirects old CLI docs links to current pages', async () => {
    await render(<DocsRouteHarness initialPath="/docs/users/cli/install" />)

    await expect
      .poll(() => document.querySelector('main h1')?.textContent)
      .toBe('Command Line Basics')

    await cleanup()

    await render(
      <DocsRouteHarness initialPath="/docs/developers/cli/installation-and-commands" />,
    )

    await expect
      .poll(() => document.querySelector('main h1')?.textContent)
      .toBe('CLI Reference')
  })
})
