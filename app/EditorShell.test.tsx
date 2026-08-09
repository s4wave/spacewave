import { Window } from 'happy-dom'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'

import { EditorShell } from './EditorShell.js'

interface MockSession {
  id: string
}

interface MockMountSessionResult {
  session: MockSession
}

interface MockRootResource {
  mountSessionByIdx: (
    request: { sessionIdx: number },
    signal?: AbortSignal,
  ) => Promise<MockMountSessionResult>
}

interface MockResource {
  value: unknown
  loading: boolean
  error: Error | null
}

const h = vi.hoisted(() => {
  const session = { id: 'session-1' }
  return {
    appPath: '/',
    appPathSubscribers: new Set<() => void>(),
    currentSessionResource: null as MockResource | null,
    mountedSessionResource: {
      value: session,
      loading: false,
      error: null,
    },
    mountSessionByIdx: vi.fn<MockRootResource['mountSessionByIdx']>(),
    setOpenMenu: vi.fn(),
    shellTabs: {
      tabs: [{ id: 'tab-active', name: 'Home', path: '/' }],
      activeTabId: 'tab-active',
    },
    session,
    sessionProviderResources: [] as MockResource[],
    shellGridModes: [] as boolean[],
  }
})

if (typeof document === 'undefined') {
  const happyDomWindow = new Window({ url: 'http://localhost/' })

  Object.defineProperties(globalThis, {
    window: { value: happyDomWindow, configurable: true },
    document: { value: happyDomWindow.document, configurable: true },
    HTMLElement: { value: happyDomWindow.HTMLElement, configurable: true },
    Element: { value: happyDomWindow.Element, configurable: true },
    Node: { value: happyDomWindow.Node, configurable: true },
    Text: { value: happyDomWindow.Text, configurable: true },
    DocumentFragment: {
      value: happyDomWindow.DocumentFragment,
      configurable: true,
    },
    SVGElement: { value: happyDomWindow.SVGElement, configurable: true },
    AbortController: {
      value: happyDomWindow.AbortController,
      configurable: true,
    },
    AbortSignal: { value: happyDomWindow.AbortSignal, configurable: true },
    Event: { value: happyDomWindow.Event, configurable: true },
    CustomEvent: { value: happyDomWindow.CustomEvent, configurable: true },
    KeyboardEvent: { value: happyDomWindow.KeyboardEvent, configurable: true },
    MouseEvent: { value: happyDomWindow.MouseEvent, configurable: true },
    FocusEvent: { value: happyDomWindow.FocusEvent, configurable: true },
    MutationObserver: {
      value: happyDomWindow.MutationObserver,
      configurable: true,
    },
    navigator: { value: happyDomWindow.navigator, configurable: true },
  })
}

vi.mock('@s4wave/web/router/app-path.js', () => ({
  getAppPath: () => h.appPath,
  getAppNavigation: () => ({ path: h.appPath, params: {} }),
  subscribeAppPath: (onChange: () => void) => {
    h.appPathSubscribers.add(onChange)
    return () => h.appPathSubscribers.delete(onChange)
  },
}))
vi.mock('@s4wave/web/state/index.js', () => ({
  useStateAtom: <T,>(_: unknown, __: string, initialValue: T) => [
    initialValue,
    h.setOpenMenu,
  ],
  useStateNamespace: (parts: string[]) => parts,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => ({
    value: { mountSessionByIdx: h.mountSessionByIdx },
    loading: false,
    error: null,
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (
    parentResource: { value?: MockRootResource | null } | null,
    factory: (
      root: MockRootResource,
      signal: AbortSignal,
      cleanup: (session: MockSession) => MockSession,
    ) => Promise<MockSession | null> | MockSession | null,
  ) => {
    if (parentResource?.value) {
      void factory(
        parentResource.value,
        new AbortController().signal,
        (session) => session,
      )
    }
    return h.mountedSessionResource
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    Provider: ({
      children,
      resource,
    }: {
      children?: ReactNode
      resource: MockResource
    }) => {
      h.currentSessionResource = resource
      h.sessionProviderResources.push(resource)
      return children
    },
    useContext: () => h.currentSessionResource,
  },
}))

vi.mock('@s4wave/web/frame/bottom-bar-root.js', () => ({
  BottomBarRoot: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/router/HashRouter.js', () => ({
  HashRouter: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  Route: ({ children }: { children?: ReactNode }) => <>{children}</>,
  Routes: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: () => null,
}))

vi.mock('@s4wave/web/command/FocusContext.js', () => ({
  ShellTabFocusContextProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/command/KeyDispatcher.js', () => ({
  KeyDispatcher: ({ children }: { children?: ReactNode }) => <>{children}</>,
}))

vi.mock('@s4wave/web/command/CommandPalette.js', () => ({
  CommandPalette: () => {
    const sessionResource = h.currentSessionResource
    const session = sessionResource?.value as MockSession | null | undefined
    return (
      <div data-testid="command-palette-session">
        {session?.id ?? 'no-session'}
      </div>
    )
  },
}))

vi.mock('@s4wave/web/command/WhichKeyPanel.js', () => ({
  WhichKeyPanel: () => null,
}))

vi.mock('@s4wave/app/BuiltinCommands.js', () => ({
  BuiltinCommands: () => null,
}))

vi.mock('@s4wave/app/DebugCommands.js', () => ({
  DebugCommands: () => null,
}))

vi.mock('@s4wave/web/object/TabContext.js', () => ({
  TabContextProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('./ShellContext.js', () => ({
  ShellProvider: ({
    children,
    isGridMode,
  }: {
    children?: ReactNode
    isGridMode: boolean
  }) => {
    h.shellGridModes.push(isGridMode)
    return children
  },
}))

vi.mock('./ShellFlexLayout.js', () => ({
  ShellTabStrip: ({ children }: { children?: ReactNode }) => (
    <div data-testid="shell-overlay">{children}</div>
  ),
}))

vi.mock('./ShellGridLayout.js', () => ({
  ShellGridLayout: () => <div data-testid="grid-layout" />,
}))

vi.mock('./ShellMenuBar.js', () => ({
  ShellMenuBar: () => <div data-testid="shell-menu-bar" />,
}))

vi.mock('./ShellTabContext.js', () => ({
  useShellTabs: () => h.shellTabs,
}))

vi.mock('./ShellTabsProvider.js', () => ({
  ShellTabsProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('./ShellTabStateProvider.js', () => ({
  ShellTabStateProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

function setActiveTabPath(path: string) {
  h.shellTabs = {
    tabs: [{ id: 'tab-active', name: 'Active', path }],
    activeTabId: 'tab-active',
  }
}

describe('EditorShell command session scope', () => {
  beforeEach(() => {
    h.appPath = '/'
    h.appPathSubscribers.clear()
    h.currentSessionResource = null
    h.mountSessionByIdx.mockReset()
    h.mountSessionByIdx.mockResolvedValue({ session: h.session })
    h.sessionProviderResources = []
    h.setOpenMenu.mockReset()
    h.shellGridModes = []
    h.shellTabs = {
      tabs: [{ id: 'tab-active', name: 'Home', path: '/' }],
      activeTabId: 'tab-active',
    }
  })

  afterEach(() => {
    cleanup()
  })

  it('mounts the active /u session for shell-level command UI', () => {
    setActiveTabPath('/u/1/settings/keybindings')

    render(<EditorShell />)

    expect(h.mountSessionByIdx).toHaveBeenCalledTimes(1)
    expect(h.mountSessionByIdx.mock.calls[0]?.[0]).toEqual({ sessionIdx: 1 })
    expect(h.sessionProviderResources).toEqual([h.mountedSessionResource])
    expect(screen.getByTestId('command-palette-session').textContent).toBe(
      'session-1',
    )
  })

  it('does not mount a session for non-session shell tabs', () => {
    setActiveTabPath('/settings/keybindings')

    render(<EditorShell />)

    expect(h.mountSessionByIdx).not.toHaveBeenCalled()
    expect(h.sessionProviderResources).toEqual([])
    expect(screen.getByTestId('command-palette-session').textContent).toBe(
      'no-session',
    )
  })

  it('tracks the active Shell Tab route in the document title', async () => {
    const view = render(<EditorShell />)
    await waitFor(() => expect(document.title).toBe('Spacewave'))

    h.shellTabs = {
      tabs: [{ id: 'tab-pricing', name: 'Pricing', path: '/pricing' }],
      activeTabId: 'tab-pricing',
    }
    view.rerender(<EditorShell />)

    await waitFor(() => expect(document.title).toBe('Pricing - Spacewave'))
  })
  it('tracks a programmatic login-to-grid transition immediately', async () => {
    h.appPath = '/login'
    render(<EditorShell />)
    expect(h.shellGridModes.at(-1)).toBe(false)

    act(() => {
      h.appPath = '/g/split-layout'
      for (const subscriber of h.appPathSubscribers) subscriber()
    })

    await waitFor(() => expect(h.shellGridModes.at(-1)).toBe(true))
  })
})
