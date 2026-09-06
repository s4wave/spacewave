import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import React, { type ReactNode } from 'react'

const initBootReportCollectorMock = vi.hoisted(() =>
  vi.fn(() => ({ start: () => {}, stop: () => {}, seal: () => ({}) })),
)
const createRootMock = vi.hoisted(() =>
  vi.fn(() => ({
    render: vi.fn(),
    unmount: vi.fn(),
  })),
)
const hydrateRootMock = vi.hoisted(() =>
  vi.fn(() => ({ render: vi.fn(), unmount: vi.fn() })),
)

// The collector does not exist yet at the red-confirmed gate. Mocking its
// module keeps this wiring test focused on the entrypoint contract: bootstrap
// installs exactly one BootReport collector per document.
vi.mock('./collector.js', () => ({
  initBootReportCollector: initBootReportCollectorMock,
}))

vi.mock('react-dom/client', () => ({
  createRoot: createRootMock,
  hydrateRoot: hydrateRootMock,
}))

vi.mock('@aptre/bldr-react', () => ({
  BldrRoot: ({ children }: { children?: ReactNode }) =>
    React.createElement(React.Fragment, null, children),
  WebViewErrorBoundary: ({ children }: { children?: ReactNode }) =>
    React.createElement(React.Fragment, null, children),
}))

vi.mock('@aptre/bldr', () => ({
  isDesktop: false,
  WebDocument: class {
    waitConn() {
      return Promise.resolve(true)
    }
  },
}))

vi.mock('../bldr/browser-release-update.js', () => ({
  initBrowserReleaseUpdates: vi.fn(),
}))

vi.mock('../bldr/startup-marks.js', () => ({
  markStartupBoundary: vi.fn(),
}))

vi.mock('../entrypoint/app-path.js', () => ({
  setAppPath: vi.fn(),
}))

vi.mock('../entrypoint/boot-status.js', () => ({
  bindBrowserBootStatusToStartupMarks: vi.fn(),
  writeBrowserBootStatus: vi.fn(),
}))

describe('entrypoint BootReport collector wiring', () => {
  beforeEach(() => {
    vi.resetModules()
    initBootReportCollectorMock.mockClear()
    document.body.innerHTML = '<div id="bldr-root"></div>'
    window.history.replaceState({}, '', '/')
  })

  afterEach(() => {
    document.body.innerHTML = ''
    window.history.replaceState({}, '', '/')
  })

  it('installs the boot report collector at bootstrap', async () => {
    await import('../entrypoint/entrypoint.js')

    expect(
      initBootReportCollectorMock.mock.calls.length,
      'missing BootReport durability behavior (no collector installed in ' +
        'entrypoint): bldr/web/entrypoint/entrypoint.tsx never installs the ' +
        'BootReport collector at bootstrap',
    ).toBe(1)
  })
})
