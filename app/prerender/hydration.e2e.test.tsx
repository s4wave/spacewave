import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { hydrateRoot, type Root } from 'react-dom/client'

import { StaticProvider } from './StaticContext.js'
import { RouterProvider } from '@s4wave/web/router/router.js'
import { Landing } from '@s4wave/app/landing/Landing.js'
import { Pricing } from '@s4wave/app/landing/Pricing.js'
import { Community } from '@s4wave/app/landing/Community.js'

const noop = () => {}

// Wraps a component the same way hydrate.tsx does for static pages.
function StaticTree({
  path,
  children,
}: {
  path: string
  children: React.ReactNode
}) {
  return (
    <RouterProvider path={path} onNavigate={noop}>
      <StaticProvider>{children}</StaticProvider>
    </RouterProvider>
  )
}

describe('Hydration', () => {
  let container: HTMLDivElement
  let root: Root | null = null
  let restoreConsoleError: (() => void) | null = null
  let errorCalls: unknown[][] = []

  beforeEach(() => {
    localStorage.clear()
    window.location.hash = ''
    container = document.createElement('div')
    container.style.width = '1280px'
    container.style.height = '800px'
    document.body.appendChild(container)
    errorCalls = []
    const errorSpy = vi
      .spyOn(console, 'error')
      .mockImplementation((...args: unknown[]) => {
        errorCalls.push(args)
      })
    restoreConsoleError = () => errorSpy.mockRestore()
  })

  afterEach(() => {
    root?.unmount()
    root = null
    document.body.removeChild(container)
    restoreConsoleError?.()
    restoreConsoleError = null
  })

  function getHydrationErrors(): unknown[][] {
    return errorCalls.filter((call: unknown[]) => {
      const msg = String(call[0])
      return (
        msg.includes('Hydration') ||
        msg.includes('hydrat') ||
        msg.includes('did not match') ||
        msg.includes('server rendered') ||
        msg.includes('mismatch')
      )
    })
  }

  it('landing page hydrates without errors', async () => {
    const tree = (
      <StaticTree path="/">
        <Landing />
      </StaticTree>
    )

    container.innerHTML = renderToString(tree)
    root = hydrateRoot(container, tree)

    // Wait for hydration to settle.
    await new Promise((r) => setTimeout(r, 200))

    const errors = getHydrationErrors()
    expect(errors).toHaveLength(0)
  })

  it('pricing page hydrates without errors', async () => {
    const tree = (
      <StaticTree path="/pricing">
        <Pricing />
      </StaticTree>
    )

    container.innerHTML = renderToString(tree)
    root = hydrateRoot(container, tree)

    await new Promise((r) => setTimeout(r, 200))

    const errors = getHydrationErrors()
    expect(errors).toHaveLength(0)
  })

  it('community page hydrates without errors', async () => {
    const tree = (
      <StaticTree path="/community">
        <Community />
      </StaticTree>
    )

    container.innerHTML = renderToString(tree)
    root = hydrateRoot(container, tree)

    await new Promise((r) => setTimeout(r, 200))

    const errors = getHydrationErrors()
    expect(errors).toHaveLength(0)
  })

  it('landing page SVG animations hydrate deterministically', () => {
    const tree = (
      <StaticTree path="/">
        <Landing />
      </StaticTree>
    )

    // Render twice to verify deterministic output (no random/Date values).
    const html1 = renderToString(tree)
    const html2 = renderToString(tree)
    expect(html1).toBe(html2)

    // Verify SVG strokeDasharray values are rounded (no 15+ digit floats).
    const dashArrayMatch = html1.match(/strokeDasharray="([^"]+)"/g)
    if (dashArrayMatch) {
      for (const attr of dashArrayMatch) {
        const values = attr.match(/[\d.]+/g) ?? []
        for (const v of values) {
          const decimals = v.split('.')[1]
          if (decimals) {
            expect(decimals.length).toBeLessThanOrEqual(4)
          }
        }
      }
    }
  })
})
