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

vi.mock('@s4wave/app/debug/keybindings/KeybindingsDebug.js', () => ({
  KeybindingsDebug: () => <div data-testid="keybindings-debug" />,
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

vi.mock('@s4wave/web/debug/FactListDebug.js', () => ({
  FactListDebug: () => <div data-testid="fact-list-debug" />,
}))

vi.mock('@s4wave/web/debug/ViewerSideDebug.js', () => ({
  ViewerSideDebug: () => <div data-testid="viewer-side-debug" />,
}))

vi.mock('@s4wave/web/debug/ViewerFloatDebug.js', () => ({
  ViewerFloatDebug: () => <div data-testid="viewer-float-debug" />,
}))

vi.mock('@s4wave/app/debug/git/GitSideDebug.js', () => ({
  GitSideDebug: () => <div data-testid="git-side-debug" />,
}))

vi.mock('@s4wave/app/debug/terminal/TerminalViewerDebug.js', () => ({
  TerminalViewerDebug: () => <div data-testid="terminal-viewer-debug" />,
}))

vi.mock('@s4wave/app/debug/device/DeviceCapabilityDebug.js', () => ({
  DeviceCapabilityDebug: () => <div data-testid="device-capability-debug" />,
}))

vi.mock('@s4wave/web/debug/TableDebug.js', () => ({
  TableDebug: () => <div data-testid="table-debug" />,
}))

vi.mock('@s4wave/web/debug/FileListDebug.js', () => ({
  FileListDebug: () => <div data-testid="file-list-debug" />,
}))

vi.mock('@s4wave/web/debug/ViewerStatusDebug.js', () => ({
  ViewerStatusDebug: () => <div data-testid="viewer-status-debug" />,
}))

vi.mock('@s4wave/app/debug/wizard/WizardShellDebug.js', () => ({
  WizardShellDebug: () => <div data-testid="wizard-shell-debug" />,
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
  { path: '/debug/ui/keybindings', testId: 'keybindings-debug' },
  { path: '/debug/ui/session-settings', testId: 'session-settings-debug' },
  { path: '/debug/ui/loading', testId: 'loading-debug' },
  { path: '/debug/ui/forge-viewer', testId: 'forge-viewer-debug' },
  { path: '/debug/ui/unixfs-browser', testId: 'unixfs-browser-debug' },
  { path: '/debug/ui/file-list', testId: 'file-list-debug' },
  { path: '/debug/ui/table', testId: 'table-debug' },
  { path: '/debug/ui/facts', testId: 'fact-list-debug' },
  { path: '/debug/ui/side', testId: 'viewer-side-debug' },
  { path: '/debug/ui/git-side', testId: 'git-side-debug' },
  { path: '/debug/ui/terminal', testId: 'terminal-viewer-debug' },
  { path: '/debug/ui/device-capabilities', testId: 'device-capability-debug' },
  { path: '/debug/ui/viewer-status', testId: 'viewer-status-debug' },
  { path: '/debug/ui/viewer-float', testId: 'viewer-float-debug' },
  { path: '/debug/ui/wizard', testId: 'wizard-shell-debug' },
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
    // Open the index.
    renderDebugRoute('/debug')

    // It links every child route, each with a visible name.
    const links = screen.getAllByRole('link')
    const linkTargets = links.map((link) => link.getAttribute('href'))
    expect(linkTargets).toEqual(
      expect.arrayContaining(debugChildRoutes.map((route) => `#${route.path}`)),
    )
    for (const link of links) {
      expect(link.textContent?.trim()).not.toBe('')
    }
  })

  it('resolves a file list fixture state to the file list debug page', () => {
    renderDebugRoute('/debug/ui/file-list/selected')

    expect(screen.getByTestId('file-list-debug')).toBeDefined()
  })

  it('resolves a viewer status fixture state to its debug page', () => {
    renderDebugRoute('/debug/ui/viewer-status/failed')

    expect(screen.getByTestId('viewer-status-debug')).toBeDefined()
  })

  it('resolves a wizard fixture state to the wizard debug page', () => {
    renderDebugRoute('/debug/ui/wizard/invalid')

    expect(screen.getByTestId('wizard-shell-debug')).toBeDefined()
  })

  it('resolves a table fixture state to the table debug page', () => {
    renderDebugRoute('/debug/ui/table/by-latency')

    expect(screen.getByTestId('table-debug')).toBeDefined()
  })

  it('resolves a facts fixture state to the facts debug page', () => {
    renderDebugRoute('/debug/ui/facts/two-fact')

    expect(screen.getByTestId('fact-list-debug')).toBeDefined()
  })

  it('resolves a side column fixture state to the side column debug page', () => {
    renderDebugRoute('/debug/ui/side/rail')

    expect(screen.getByTestId('viewer-side-debug')).toBeDefined()
  })

  it('resolves a Git side column fixture state to its debug page', () => {
    renderDebugRoute('/debug/ui/git-side/empty')

    expect(screen.getByTestId('git-side-debug')).toBeDefined()
  })

  it('resolves a viewer float fixture state to its debug page', () => {
    renderDebugRoute('/debug/ui/viewer-float/hidden')

    expect(screen.getByTestId('viewer-float-debug')).toBeDefined()
  })

  it.each(debugChildRoutes)(
    'resolves $path to its debug child route',
    (route) => {
      renderDebugRoute(route.path)

      expect(screen.getByTestId(route.testId)).toBeDefined()
    },
  )
})
