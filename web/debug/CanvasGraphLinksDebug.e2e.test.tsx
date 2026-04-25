import type { ReactNode } from 'react'
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'
import '@s4wave/web/style/app.css'

import { CanvasGraphLinksDebug } from '@s4wave/web/debug/CanvasGraphLinksDebug.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

function TestWrapper({ children }: { children: ReactNode }) {
  return (
    <RouterProvider path="/debug/ui/canvas-graph-links" onNavigate={() => {}}>
      {children}
    </RouterProvider>
  )
}

const variants = ['compact', 'balanced', 'metadata'] as const

describe('CanvasGraphLinksDebug', () => {
  beforeEach(() => void cleanup())
  afterEach(() => void cleanup())

  it('renders all graph-link pill variants', async () => {
    await render(
      <TestWrapper>
        <CanvasGraphLinksDebug />
      </TestWrapper>,
    )

    for (const variant of variants) {
      await expect
        .poll(() =>
          document.querySelector(
            `[data-testid="graph-link-variant-${variant}"]`,
          ),
        )
        .not.toBeNull()

      const section = document.querySelector(
        `[data-testid="graph-link-variant-${variant}"]`,
      )
      expect(section?.textContent).toContain(
        variant === 'balanced' ? 'Selected direction' : variant,
      )
    }
  })

  it('renders loaded and unloaded fixtures for each variant', async () => {
    await render(
      <TestWrapper>
        <CanvasGraphLinksDebug />
      </TestWrapper>,
    )

    for (const variant of variants) {
      await expect
        .poll(
          () =>
            document.querySelectorAll(
              `[data-testid="graph-link-pill-${variant}-unloaded"]`,
            ).length,
        )
        .toBeGreaterThanOrEqual(1)

      expect(
        document.querySelectorAll(
          `[data-testid="graph-link-pill-${variant}-loaded"]`,
        ).length,
      ).toBeGreaterThanOrEqual(1)
    }
  })

  it('renders production state coverage fixtures', async () => {
    await render(
      <TestWrapper>
        <CanvasGraphLinksDebug />
      </TestWrapper>,
    )

    await expect
      .poll(
        () => document.body.textContent?.includes('State Coverage') ?? false,
      )
      .toBe(true)

    const text = document.body.textContent ?? ''
    expect(text).toContain('hidden on canvas')
    expect(text).toContain('capped')
    expect(text).toContain('protected')
    expect(text).toContain('relatedTo')
    expect(text).toContain('Incoming')
    expect(text).toContain('Outgoing')
  })
})
