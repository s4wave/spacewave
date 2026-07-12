import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RouterProvider } from '@s4wave/web/router/router.js'

import { KeybindingsDebug } from './KeybindingsDebug.js'

describe('KeybindingsDebug', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the gallery content on its debug route', () => {
    render(
      <RouterProvider path="/debug/ui/keybindings" onNavigate={vi.fn()}>
        <KeybindingsDebug />
      </RouterProvider>,
    )

    expect(
      screen.getByRole('heading', {
        name: 'Five ways to make keybindings feel approachable',
      }),
    ).toBeDefined()
    expect(screen.getAllByText('Inventory table').length).toBeGreaterThan(0)
    expect(screen.getByText('Open command finder')).toBeDefined()
  })
})
