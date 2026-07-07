import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RouterProvider, Routes } from '@s4wave/web/router/router.js'

import { DebugRoutes } from './DebugRoutes.js'

vi.mock('@s4wave/web/debug/CanvasGraphLinksDebug.js', () => ({
  CanvasGraphLinksDebug: () => <div data-testid="canvas-graph-links-debug" />,
}))

vi.mock('@s4wave/web/debug/DebugDbBench.js', () => ({
  DebugDbBench: () => <div data-testid="debug-db-bench" />,
}))

vi.mock('@s4wave/web/debug/ForgeViewerDebug.js', () => ({
  ForgeViewerDebug: () => <div data-testid="forge-viewer-debug" />,
}))

vi.mock('@s4wave/web/debug/HDRDebug.js', () => ({
  HDRDebug: () => <div data-testid="hdr-debug" />,
}))

vi.mock('@s4wave/web/debug/LayoutDebug.js', () => ({
  LayoutDebug: () => <div data-testid="layout-debug" />,
}))

vi.mock('@s4wave/web/debug/LayoutColorsDebug.js', () => ({
  LayoutColorsDebug: () => <div data-testid="layout-colors-debug" />,
}))

vi.mock('@s4wave/web/debug/LoadingDebug.js', () => ({
  LoadingDebug: () => <div data-testid="loading-debug" />,
}))

vi.mock('@s4wave/web/debug/SessionSettingsDebug.js', () => ({
  SessionSettingsDebug: () => <div data-testid="session-settings-debug" />,
}))

vi.mock('@s4wave/web/debug/UnixFSBrowserDebug.js', () => ({
  UnixFSBrowserDebug: () => <div data-testid="unixfs-browser-debug" />,
}))

const debugChildRoutes = [
  { path: '/debug/db/bench', testId: 'debug-db-bench' },
  { path: '/debug/hdr', testId: 'hdr-debug' },
  { path: '/debug/ui/layout', testId: 'layout-debug' },
  { path: '/debug/ui/layout/colors', testId: 'layout-colors-debug' },
  {
    path: '/debug/ui/canvas-graph-links',
    testId: 'canvas-graph-links-debug',
  },
  { path: '/debug/ui/session-settings', testId: 'session-settings-debug' },
  { path: '/debug/ui/loading', testId: 'loading-debug' },
  { path: '/debug/ui/forge-viewer', testId: 'forge-viewer-debug' },
  { path: '/debug/ui/unixfs-browser', testId: 'unixfs-browser-debug' },
]

function renderDebugRoute(path: string) {
  render(
    <RouterProvider path={path} onNavigate={vi.fn()}>
      <Routes fullPath>{DebugRoutes}</Routes>
    </RouterProvider>,
  )
}

describe('DebugRoutes', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders a visible /debug index with links to every debug child route', () => {
    renderDebugRoute('/debug')

    const links = screen.getAllByRole('link')
    const linkTargets = links.map((link) => link.getAttribute('href'))
    expect(linkTargets).toEqual(
      expect.arrayContaining(
        debugChildRoutes.map((route) => `#${route.path}`),
      ),
    )
    for (const link of links) {
      expect(link.textContent?.trim()).not.toBe('')
    }
  })

  it.each(debugChildRoutes)('resolves $path to its debug child route', (route) => {
    renderDebugRoute(route.path)

    expect(screen.getByTestId(route.testId)).toBeDefined()
  })
})
