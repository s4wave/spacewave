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
      <DocsLayout sidebar={<div>Sidebar content</div>}>
        <div>Page content</div>
      </DocsLayout>,
    )

    expect(
      screen.getByRole('button', {
        name: 'Open documentation navigation',
      }),
    ).not.toBeNull()

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
  })
})
