import { describe, it, expect, beforeEach } from 'vitest'
import { page } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { App } from './App.js'
import { AppShell } from './AppShell.js'
import { EditorShell } from './EditorShell.js'

describe('App E2E', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  it('renders the App component and shows loading state without backend', async () => {
    await render(<App />)

    await expect.element(page.getByText('Initializing')).toBeInTheDocument()
  })

  it('renders AppShell with children', async () => {
    await render(
      <AppShell>
        <div data-testid="shell-content">Shell Content</div>
      </AppShell>,
    )

    await expect
      .element(page.getByTestId('shell-content'))
      .toHaveTextContent('Shell Content')
  })
})

describe('EditorShell E2E', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  it('renders EditorShell in normal mode (not grid)', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByRole('button', { name: 'Home' }).first(), {
        timeout: 5000,
      })
      .toBeInTheDocument()
  })

  it('renders landing page content in Home tab', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByRole('heading', { name: '[SPACEWAVE]' }).first(), {
        timeout: 5000,
      })
      .toBeInTheDocument()
  })

  it('renders menu bar with logo control', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByRole('button', { name: 'Open command palette' }), {
        timeout: 5000,
      })
      .toBeInTheDocument()
  })

  it('supports creating new tabs', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByRole('button', { name: 'Home' }).first(), {
        timeout: 5000,
      })
      .toBeInTheDocument()

    await expect
      .poll(
        () => {
          const btn = document.querySelector('button[title="New tab"]')
          return btn
        },
        { timeout: 5000 },
      )
      .not.toBeNull()

    const addButton = document.querySelector(
      'button[title="New tab"]',
    ) as HTMLElement
    addButton.click()

    await expect
      .poll(
        () => {
          const tabButtons = document.querySelectorAll(
            '.flexlayout__tab_button',
          )
          return tabButtons.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2)
  })

  it('navigates to grid mode when URL has /g/ prefix', async () => {
    window.location.hash = '#/g/test'

    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .poll(
        () => {
          return (
            !window.location.hash.startsWith('#/g/') ||
            window.location.hash === '#/'
          )
        },
        { timeout: 5000 },
      )
      .toBe(true)
  })

  it('shows navigation links on landing page', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByRole('button', { name: 'the community' }).first(), {
        timeout: 5000,
      })
      .toBeInTheDocument()
  })

  it('shows Get Started button on landing page', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByRole('button', { name: /get started \(free\)/i }), {
        timeout: 5000,
      })
      .toBeInTheDocument()
  })
})
