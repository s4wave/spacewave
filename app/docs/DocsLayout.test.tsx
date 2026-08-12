import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { DocsLayout } from './DocsLayout.js'

describe('DocsLayout', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the mobile sheet inside the docs container', async () => {
    const user = userEvent.setup()
    const { container } = render(
      <DocsLayout
        sidebar={<div>Sidebar content</div>}
        mobileTitle="Create Your First Space"
      >
        <div>Page content</div>
      </DocsLayout>,
    )

    const trigger = screen.getByRole('button', {
      name: 'Open documentation navigation',
    })
    expect(trigger).not.toBeNull()
    expect(screen.getByText('Create Your First Space')).not.toBeNull()

    await user.click(
      screen.getByRole('button', {
        name: 'Open documentation navigation',
      }),
    )

    const root = container.firstElementChild
    const sheetContent = container.querySelector('[data-slot="sheet-content"]')
    const sheetOverlay = container.querySelector('[data-slot="sheet-overlay"]')

    expect(root).not.toBeNull()
    expect(sheetContent).not.toBeNull()
    expect(sheetOverlay).not.toBeNull()
    expect(root?.contains(sheetContent)).toBe(true)
    expect(root?.contains(sheetOverlay)).toBe(true)
    expect(
      screen.getByRole('button', {
        name: 'Close documentation navigation',
      }),
    ).not.toBeNull()

    await user.keyboard('{Escape}')
    expect(
      screen.queryByRole('button', {
        name: 'Close documentation navigation',
      }),
    ).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })
})
