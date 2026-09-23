import type { ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { afterEach, describe, expect, it } from 'vitest'

import { StaticProvider } from '@s4wave/app/prerender/StaticContext.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

import { LandingChat } from './LandingChat.js'
import { LandingCli } from './LandingCli.js'
import { LandingDevices } from './LandingDevices.js'
import { LandingDrive } from './LandingDrive.js'
import { LandingNotes } from './LandingNotes.js'
import { LandingPlugins } from './LandingPlugins.js'

function renderWithRouter(node: ReactNode) {
  return render(
    <RouterProvider path="/landing" onNavigate={() => {}}>
      {node}
    </RouterProvider>,
  )
}

function renderStaticWithRouter(node: ReactNode) {
  return render(
    <RouterProvider path="/landing" onNavigate={() => {}}>
      <StaticProvider>{node}</StaticProvider>
    </RouterProvider>,
  )
}

interface LandingCase {
  page: ReactNode
  label: string
  href: string
}

// LANDING_CASES pairs each use-case page with its primary action.
const LANDING_CASES: LandingCase[] = [
  {
    page: <LandingDrive />,
    label: 'Create a Drive',
    href: '#/quickstart/drive',
  },
  {
    page: <LandingNotes />,
    label: 'Create a notebook',
    href: '#/quickstart/notebook',
  },
  {
    page: <LandingChat />,
    label: 'Start a chat',
    href: '#/quickstart/chat',
  },
  {
    page: <LandingDevices />,
    label: 'Add a device',
    href: '#/quickstart/device',
  },
  {
    page: <LandingPlugins />,
    label: 'Create a SQL Database',
    href: '#/quickstart/sql',
  },
  {
    page: <LandingCli />,
    label: 'Download the CLI',
    href: '#/download/cli',
  },
]

describe('use-case landing CTAs', () => {
  afterEach(() => {
    cleanup()
  })

  it('wires the primary action for each use-case page to a real app entry point', () => {
    for (const testCase of LANDING_CASES) {
      renderWithRouter(testCase.page)
      expect(
        screen.getByRole('link', { name: testCase.label }).getAttribute('href'),
      ).toBe(testCase.href)
      cleanup()
    }
  })

  it('keeps static-to-static links crawlable while app-entry actions stay hash routes', () => {
    renderStaticWithRouter(<LandingDrive />)

    expect(
      screen.getByRole('link', { name: 'Create a Drive' }).getAttribute('href'),
    ).toBe('#/quickstart/drive')
    expect(
      screen
        .getByRole('link', { name: 'See all features' })
        .getAttribute('href'),
    ).toBe('/landing')
  })

  it('server-renders every use-case page without client-only state', () => {
    for (const testCase of LANDING_CASES) {
      const html = renderToString(
        <RouterProvider path="/landing" onNavigate={() => {}}>
          <StaticProvider>{testCase.page}</StaticProvider>
        </RouterProvider>,
      )
      expect(html).toContain(testCase.label)
      expect(html).not.toContain('data-spacewave-app')
    }
  })
})
