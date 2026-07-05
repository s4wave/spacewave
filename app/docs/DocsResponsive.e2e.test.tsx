import { useState } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { Routes, RouterProvider, type To } from '@s4wave/web/router/router.js'
import { DocsRoutes } from '@s4wave/app/routes/DocsRoutes.js'
import { DocsLayout } from '@s4wave/app/docs/DocsLayout.js'
import { DocsPage } from '@s4wave/app/docs/DocsPage.js'
import type { DocPage } from '@s4wave/app/docs/types.js'

function DocsRouteHarness({ initialPath }: { initialPath: string }) {
  const [path, setPath] = useState(initialPath)
  const navigate = (to: To) => setPath(to.path)

  return (
    <RouterProvider path={path} onNavigate={navigate}>
      <Routes fullPath>{DocsRoutes}</Routes>
    </RouterProvider>
  )
}

// WIDTHS are the responsive breakpoints under test: narrow phone, tablet, and
// desktop. 360 is the production floor the docs viewer must stay readable at.
const WIDTHS: [string, number][] = [
  ['360', 360],
  ['768', 768],
  ['1280', 1280],
]

async function shot(name: string) {
  await page.screenshot({ path: `__screenshots__/docs-responsive/${name}.png` })
}

// wideBody is a synthetic markdown page exercising the overflow-prone prose
// primitives: a long unbroken code line, a wide many-column table, and a long
// inline token. None of these may force the layout wider than its container.
const wideBody = `A paragraph of body text that should wrap to a readable measure and never force the page to scroll sideways on a narrow phone display at all.

\`\`\`bash
spacewave session create --provider spacewave-cloud --region us-east-1 --name "my-very-long-session-name-that-does-not-wrap" --lock-mode strict --verbose
\`\`\`

| Command | Description | Provider | Region | Notes |
| --- | --- | --- | --- | --- |
| session create | Creates a new collaborative session bound to a provider | spacewave-cloud | us-east-1 | Requires an authenticated provider account |
| space attach | Attaches an existing Space to the current session context | local | n/a | Space must already exist on disk |

Reference the identifier \`urn:spacewave:object:0xdeadbeefcafebabe0123456789abcdef\` inline.
`

const wideDoc: DocPage = {
  site: 'developers',
  section: 'cli',
  filename: '01-cli-reference.md',
  slug: 'cli-reference',
  url: '/docs/developers/cli/cli-reference',
  order: 1,
  title: 'CLI Reference',
  summary: 'Wide-content overflow fixture for responsive proof.',
  body: wideBody,
}

function WideDocHarness() {
  return (
    <RouterProvider
      path="/docs/developers/cli/cli-reference"
      onNavigate={() => {}}
    >
      <DocsLayout sidebar={<div>nav</div>}>
        <DocsPage doc={wideDoc} />
      </DocsLayout>
    </RouterProvider>
  )
}

// horizontalOverflow returns the pixels by which the page can scroll sideways.
// Anything above a sub-pixel rounding tolerance is a responsive defect.
function horizontalOverflow() {
  return (
    document.documentElement.scrollWidth - document.documentElement.clientWidth
  )
}

describe('docs viewer responsive layout', () => {
  beforeEach(async () => {
    await cleanup()
    localStorage.clear()
  })

  it('hub, site home, and article stay within narrow, tablet, and desktop widths', async () => {
    for (const [label, width] of WIDTHS) {
      await page.viewport(width, 900)

      await render(<DocsRouteHarness initialPath="/docs" />)
      await expect
        .element(page.getByRole('heading', { name: 'Documentation' }))
        .toBeInTheDocument()
      await shot(`hub-${label}`)
      expect(horizontalOverflow()).toBeLessThanOrEqual(1)
      await cleanup()

      await render(<DocsRouteHarness initialPath="/docs/users" />)
      await expect
        .element(page.getByRole('heading', { name: 'Users', level: 1 }))
        .toBeInTheDocument()
      await shot(`site-users-${label}`)
      expect(horizontalOverflow()).toBeLessThanOrEqual(1)
      await cleanup()

      await render(
        <DocsRouteHarness initialPath="/docs/users/start/create-your-first-space" />,
      )
      await expect
        .element(page.getByRole('heading', { name: 'Create Your First Space' }))
        .toBeInTheDocument()
      await shot(`article-${label}`)
      expect(horizontalOverflow()).toBeLessThanOrEqual(1)
      await cleanup()
    }
  }, 120000)

  it('wide code blocks and tables never overflow the narrow content column', async () => {
    await page.viewport(360, 900)
    await render(<WideDocHarness />)
    await expect
      .element(page.getByRole('heading', { name: 'CLI Reference' }))
      .toBeInTheDocument()
    await shot('wide-content-360')

    // The page itself must not scroll sideways: wide prose primitives scroll
    // inside their own bounded box, they do not push the layout.
    expect(horizontalOverflow()).toBeLessThanOrEqual(1)

    const main = document.querySelector('main')
    expect(main).not.toBeNull()
    if (main) expect(main.clientWidth).toBeLessThanOrEqual(360)

    // The prose column is the containment boundary: nothing wide may extend
    // past its right edge, wide primitives scroll within it instead.
    const prose = document.querySelector('.docs-prose') as HTMLElement | null
    expect(prose).not.toBeNull()
    const columnRight = prose!.getBoundingClientRect().right

    // The code block is its own horizontal scroll box: genuinely wider than its
    // visible width (so it scrolls), yet clipped to the content column.
    const codeBox = document.querySelector(
      '.docs-prose pre',
    ) as HTMLElement | null
    expect(codeBox).not.toBeNull()
    expect(codeBox!.scrollWidth).toBeGreaterThan(codeBox!.clientWidth)
    expect(codeBox!.getBoundingClientRect().right).toBeLessThanOrEqual(
      columnRight + 1,
    )

    // The table wrapper is likewise a bounded, scrollable box inside the column.
    const tableBox = document.querySelector(
      '.docs-prose .docs-table-scroll',
    ) as HTMLElement | null
    expect(tableBox).not.toBeNull()
    expect(tableBox!.scrollWidth).toBeGreaterThan(tableBox!.clientWidth)
    expect(tableBox!.getBoundingClientRect().right).toBeLessThanOrEqual(
      columnRight + 1,
    )
  })

  it('collapses the sidebar into an overlay trigger on narrow widths', async () => {
    await page.viewport(360, 900)
    await render(<DocsRouteHarness initialPath="/docs" />)
    await expect
      .element(
        page.getByRole('button', { name: 'Open documentation navigation' }),
      )
      .toBeInTheDocument()

    await cleanup()

    await page.viewport(1280, 900)
    await render(<DocsRouteHarness initialPath="/docs" />)
    // The desktop rail is visible; the mobile trigger is hidden at wide widths.
    const trigger = document.querySelector(
      'button[aria-label="Open documentation navigation"]',
    )
    const triggerVisible =
      trigger != null && (trigger as HTMLElement).offsetParent != null
    expect(triggerVisible).toBe(false)
  })
})
