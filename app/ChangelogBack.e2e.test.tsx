import { beforeEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { AppShell } from './AppShell.js'
import { EditorShell } from './EditorShell.js'

describe('Changelog Back In Shell', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  it('returns to landing without breaking the flex shell tab layout', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .element(page.getByText('[SPACEWAVE]'), { timeout: 5000 })
      .toBeInTheDocument()

    window.location.hash = '#/changelog'

    await expect
      .element(page.getByText('See what is new in Spacewave.'), {
        timeout: 5000,
      })
      .toBeInTheDocument()

    const backButton = document.querySelector<HTMLButtonElement>(
      'button[class*="text-foreground-alt"][class*="cursor-pointer"]',
    )
    backButton?.click()

    await expect
      .element(page.getByText('[SPACEWAVE]'), { timeout: 5000 })
      .toBeInTheDocument()

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tab_button').length, {
        timeout: 5000,
      })
      .toBe(1)
  })
})
