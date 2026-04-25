/**
 * E2E tests for the spacewave-app App.
 *
 * Tests the full application rendering in browser mode.
 * When no backend is available, tests verify the loading state.
 * When a backend is available (via VITE_E2E_SERVER_PORT), tests the full app.
 */
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

    // Without a backend connection, AppAPI shows the loading overlay
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

    // EditorShell should render the shell layout
    // In normal mode, it renders ShellTabStrip which includes the menu bar area
    // The Home tab should be present
    await expect
      .element(page.getByRole('button', { name: 'Home' }), { timeout: 5000 })
      .toBeInTheDocument()
  })

  it('renders landing page content in Home tab', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    // The landing page should show [SPACEWAVE] title
    await expect
      .element(page.getByText('[SPACEWAVE]'), { timeout: 5000 })
      .toBeInTheDocument()
  })

  it('renders menu bar with logo control', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    // The shell menu bar always renders the logo control even when
    // responsive layout collapses the top-level menu buttons.
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

    // Wait for initial render - Home tab button should be present
    await expect
      .element(page.getByRole('button', { name: 'Home' }), { timeout: 5000 })
      .toBeInTheDocument()

    // Find and click the add tab button (has title="New tab")
    // The button is inside the flexlayout tabset toolbar
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

    // After clicking, there should be two Home tabs
    // Use a simpler check - just look for any additional tab buttons
    await expect
      .poll(
        () => {
          // Count all tab buttons in the flexlayout tab strip
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
    // Set hash to grid mode before rendering
    window.location.hash = '#/g/test'

    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    // In grid mode with invalid layout data, it should redirect to home
    // Wait for the redirect to happen
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

    // The landing page should show navigation links
    await expect
      .element(page.getByText('the community'), { timeout: 5000 })
      .toBeInTheDocument()
  })

  it('shows Get Started button on landing page', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    // The landing page should show a Get Started button (hero section)
    await expect
      .element(page.getByRole('button', { name: /get started \(free\)/i }), {
        timeout: 5000,
      })
      .toBeInTheDocument()
  })
})
