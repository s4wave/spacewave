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
  renderPage: () => void
  label: string
  href: string
}

describe('use-case landing CTAs', () => {
  afterEach(() => {
    cleanup()
  })

  it('wires the primary action for each use-case page to a real app entry point', () => {
    const cases: LandingCase[] = [
      {
        renderPage: () => renderWithRouter(<LandingDrive />),
        label: 'Create a Drive',
        href: '#/quickstart/drive',
      },
      {
        renderPage: () => renderWithRouter(<LandingDevices />),
        label: 'Link a device',
        href: '#/pair',
      },
      {
        renderPage: () => renderWithRouter(<LandingPlugins />),
        label: 'Read the SDK docs',
        href: '#/docs',
      },
      {
        renderPage: () => renderWithRouter(<LandingNotes />),
        label: 'Start writing',
        href: '#/quickstart/notebook',
      },
      {
        renderPage: () => renderWithRouter(<LandingChat />),
        label: 'Start a conversation',
        href: '#/quickstart/chat',
      },
      {
        renderPage: () => renderWithRouter(<LandingCli />),
        label: 'Download the CLI',
        href: '#/download/cli',
      },
    ]

    for (const testCase of cases) {
      testCase.renderPage()
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

  it('server-renders the drive landing page without falling back to client-only state', () => {
    const tree = (
      <RouterProvider path="/landing/drive" onNavigate={() => {}}>
        <StaticProvider>
          <LandingDrive />
        </StaticProvider>
      </RouterProvider>
    )

    expect(() => renderToString(tree)).not.toThrow()
  })
})
