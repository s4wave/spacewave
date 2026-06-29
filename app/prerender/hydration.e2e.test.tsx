import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { hydrateRoot, type Root } from 'react-dom/client'

import { StaticProvider } from './StaticContext.js'
import { RouterProvider } from '@s4wave/web/router/router.js'
import { Landing } from '@s4wave/app/landing/Landing.js'
import { Pricing } from '@s4wave/app/landing/Pricing.js'
import { Community } from '@s4wave/app/landing/Community.js'

const noop = () => {}

declare global {
  var __swStaticHandoffLinks: boolean | undefined
}

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

function setDocumentVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    value,
  })
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    value: value !== 'visible',
  })
}

function waitForHydration(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>()
  setTimeout(resolve, 200)
  return promise
}

describe('Hydration', () => {
  let container: HTMLDivElement
  let root: Root | null = null
  let restoreConsoleError: (() => void) | null = null
  let errorCalls: unknown[][] = []

  beforeEach(() => {
    localStorage.clear()
    window.location.hash = ''
    globalThis.__swStaticHandoffLinks = undefined
    container = document.createElement('div')
    Object.assign(container.style, {
      width: '1280px',
      height: '800px',
    })
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
    globalThis.__swStaticHandoffLinks = undefined
    setDocumentVisibility('visible')
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
    await waitForHydration()

    const errors = getHydrationErrors()
    expect(errors).toHaveLength(0)
  })

  it('landing page hydrates without errors when the first browser render starts hidden', async () => {
    const tree = (
      <StaticTree path="/">
        <Landing />
      </StaticTree>
    )

    setDocumentVisibility('visible')
    container.innerHTML = renderToString(tree)
    const serverButton = container.querySelector('[role="button"]')
    expect(serverButton?.getAttribute('class')).toContain(
      'animate-[pulse_8s_ease-in-out_infinite]',
    )

    setDocumentVisibility('hidden')
    root = hydrateRoot(container, tree)
    await waitForHydration()

    expect(getHydrationErrors()).toHaveLength(0)
    const hydratedButton = container.querySelector('[role="button"]')
    expect(hydratedButton?.getAttribute('class')).toContain(
      'animate-[pulse_8s_ease-in-out_infinite]',
    )
  })

  it('landing page hydrates after boot rewrites quickstart handoff links', async () => {
    const tree = (
      <StaticTree path="/">
        <Landing />
      </StaticTree>
    )

    container.innerHTML = renderToString(tree)
    for (const link of container.querySelectorAll('a[href^="/quickstart/"]')) {
      const href = link.getAttribute('href')
      if (href) link.setAttribute('href', `#${href}`)
    }
    globalThis.__swStaticHandoffLinks = true

    root = hydrateRoot(container, tree)

    await waitForHydration()

    const errors = getHydrationErrors()
    expect(errors).toHaveLength(0)
    expect(
      container
        .querySelector('a[href="#/quickstart/drive"]')
        ?.getAttribute('href'),
    ).toBe('#/quickstart/drive')
  })

  it('pricing page hydrates without errors', async () => {
    const tree = (
      <StaticTree path="/pricing">
        <Pricing />
      </StaticTree>
    )

    container.innerHTML = renderToString(tree)
    root = hydrateRoot(container, tree)

    await waitForHydration()

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

    await waitForHydration()

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
