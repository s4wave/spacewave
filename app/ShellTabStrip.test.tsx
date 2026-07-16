import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { getAppPath } from '@s4wave/web/router/app-path.js'
import type * as WebState from '@s4wave/web/state/index.js'
import {
  getBrowserShellTabsStore,
  resetBrowserShellTabsStoreForTests,
} from './BrowserShellTabsStore.js'
import { useShellTabs } from './ShellTabContext.js'
import {
  installShellTabTestBrowser,
  readShellTabsSnapshot,
  seedShellTabs,
  type ShellTabTestBrowser,
} from './ShellTabTestHarness.js'
import { ShellTabStrip } from './ShellFlexLayout.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'

const mockOptimizedLayoutProps = vi.hoisted(() => vi.fn())
const continuationEntry: ShellDocumentEntry = {
  kind: 'continuation',
  path: '/',
  params: {},
  incarnation: 'test-document',
}

vi.mock('@s4wave/web/state/index.js', async () => {
  const [React, actual] = await Promise.all([
    import('react'),
    vi.importActual<typeof WebState>('@s4wave/web/state/index.js'),
  ])
  return {
    ...actual,
    useStateAtom: <T,>(_: unknown, __: string, initialValue: T) =>
      React.useState(initialValue),
  }
})

vi.mock('./ShellTabContent.js', () => ({
  ShellTabContent: ({ tabId, path }: { tabId: string; path: string }) => (
    <div data-testid={`tab-content-${tabId}`}>{path}</div>
  ),
}))

vi.mock('./ShellTabLabel.js', () => ({
  ShellTabLabel: ({ tab }: { tab: { name: string } }) => (
    <span>{tab.name}</span>
  ),
}))

vi.mock('./ShellTabContextMenu.js', () => ({
  ShellTabContextMenu: () => null,
}))

vi.mock('./shell-grid-utils.js', () => ({
  encodeGridLayout: () => 'grid-layout',
  hasGridLayout: () => false,
}))

vi.mock('@aptre/flex-layout', () => {
  class MockTabSetNode {
    id: string
    children: MockTabNode[]
    selectedTabId: string | null

    constructor(id: string, selectedTabId: string | null) {
      this.id = id
      this.children = []
      this.selectedTabId = selectedTabId
    }

    getType() {
      return 'tabset'
    }

    getId() {
      return this.id
    }

    getSelectedNode() {
      const tabId = this.selectedTabId ?? this.children[0]?.id ?? null
      return this.children.find((child) => child.id === tabId) ?? null
    }
  }

  class MockTabNode {
    id: string
    name: string
    parent: MockTabSetNode

    constructor(id: string, name: string, parent: MockTabSetNode) {
      this.id = id
      this.name = name
      this.parent = parent
    }

    getType() {
      return 'tab'
    }

    getId() {
      return this.id
    }

    getName() {
      return this.name
    }

    getParent() {
      return this.parent
    }
  }

  class MockModel {
    tabset: MockTabSetNode
    tabsetDeleted = false
    tabSetEnableDeleteWhenEmpty = true
    tabs: MockTabNode[]
    actions: Array<{
      type: string
      attributes?: { tabSetEnableDeleteWhenEmpty?: boolean }
      tabId?: string
      node?: { id: string; name: string }
      select?: boolean
    }>
    onModelChange?: (model: MockModel) => void

    constructor(json: {
      layout: {
        children: Array<{
          id: string
          selected?: number
          children: Array<{ id: string; name: string }>
        }>
      }
    }) {
      const tabsetJson = json.layout.children[0]
      const selected =
        typeof tabsetJson.selected === 'number'
          ? (tabsetJson.children[tabsetJson.selected]?.id ?? null)
          : null
      this.tabset = new MockTabSetNode(tabsetJson.id, selected)
      this.tabs = tabsetJson.children.map((child) => {
        const tab = new MockTabNode(child.id, child.name, this.tabset)
        this.tabset.children.push(tab)
        return tab
      })
      this.actions = []
    }

    static fromJson(json: {
      layout: {
        children: Array<{
          id: string
          selected?: number
          children: Array<{ id: string; name: string }>
        }>
      }
    }) {
      return new MockModel(json)
    }

    visitNodes(callback: (node: MockTabSetNode | MockTabNode) => void) {
      if (this.tabsetDeleted) return
      callback(this.tabset)
      for (const tab of this.tabs) {
        callback(tab)
      }
    }

    getNodeById(id: string) {
      if (this.tabsetDeleted && id === this.tabset.id) return null
      return (
        this.tabs.find((tab) => tab.id === id) ??
        (this.tabset.id === id ? this.tabset : null)
      )
    }

    get selectedTabId() {
      return this.tabset.selectedTabId
    }

    toJson() {
      return {
        layout: {
          children: [
            {
              id: this.tabset.id,
              selected: this.tabs.findIndex(
                (tab) => tab.id === this.tabset.selectedTabId,
              ),
              children: this.tabs.map((tab) => ({
                id: tab.id,
                name: tab.name,
              })),
            },
          ],
        },
      }
    }

    doAction(action: {
      type: string
      tabId?: string
      node?: { id: string; name: string }
      select?: boolean
      name?: string
      attributes?: { tabSetEnableDeleteWhenEmpty?: boolean }
    }) {
      this.actions.push(action)

      if (action.type === 'updateModelAttributes') {
        const configured = action.attributes?.tabSetEnableDeleteWhenEmpty
        if (configured !== undefined) {
          this.tabSetEnableDeleteWhenEmpty = configured
        }
        if (this.tabs.length === 0 && this.tabSetEnableDeleteWhenEmpty) {
          this.tabsetDeleted = true
        }
      }

      if (action.type === 'addNode' && action.node && !this.tabsetDeleted) {
        const tab = new MockTabNode(
          action.node.id,
          action.node.name,
          this.tabset,
        )
        this.tabset.children.push(tab)
        this.tabs.push(tab)
        if (action.select !== false) {
          this.tabset.selectedTabId = tab.id
        }
      }
      if (action.type === 'deleteTab' && action.tabId) {
        this.tabs = this.tabs.filter((tab) => tab.id !== action.tabId)
        this.tabset.children = this.tabset.children.filter(
          (tab) => tab.id !== action.tabId,
        )
        if (this.tabset.selectedTabId === action.tabId) {
          this.tabset.selectedTabId = this.tabset.children[0]?.id ?? null
        }
      }
      if (action.type === 'updateNodeAttributes' && action.tabId) {
        const tab = this.tabs.find((item) => item.id === action.tabId)
        if (tab && action.name) {
          tab.name = action.name
        }
      }
      if (action.type === 'selectTab' && action.tabId) {
        this.tabset.selectedTabId = action.tabId
      }
      this.onModelChange?.(this)
    }
  }

  function OptimizedLayout({
    model,
    onModelChange,
  }: {
    model: MockModel
    onModelChange?: (model: MockModel) => void
  }) {
    model.onModelChange = onModelChange
    mockOptimizedLayoutProps({ model, onModelChange })
    return <div data-testid="layout-tab-count">{model.tabs.length}</div>
  }

  return {
    Actions: {
      addNode: (
        node: { id: string; name: string },
        _tabsetId: string,
        _location: string,
        _index: number,
        select?: boolean,
      ) => ({
        type: 'addNode',
        node,
        select,
      }),
      deleteTab: (tabId: string) => ({ type: 'deleteTab', tabId }),
      selectTab: (tabId: string) => ({ type: 'selectTab', tabId }),
      updateModelAttributes: (attributes: {
        tabSetEnableDeleteWhenEmpty?: boolean
      }) => ({ type: 'updateModelAttributes', attributes }),
      updateNodeAttributes: (tabId: string, attrs: { name?: string }) => ({
        type: 'updateNodeAttributes',
        tabId,
        name: attrs.name,
      }),
    },
    BorderNode: class {},
    DockLocation: {
      CENTER: 'center',
    },
    ITabRenderValues: class {},
    ITabSetRenderValues: class {},
    IJsonModel: class {},
    Model: MockModel,
    OptimizedLayout,
    TabNode: MockTabNode,
    TabSetNode: MockTabSetNode,
  }
})

function StateOnlyDocsOpener() {
  const { activeTabId, openPathInNewTab } = useShellTabs()

  return (
    <button
      onClick={() => {
        openPathInNewTab('/docs', {
          afterTabId: activeTabId || undefined,
          focusExisting: true,
        })
      }}
      type="button"
    >
      Open Docs
    </button>
  )
}

function FlexCliCommandProbe() {
  const { activeTabId, openPathInActiveTabset } = useShellTabs()

  return (
    <button
      onClick={() => {
        openPathInActiveTabset('/u/7/settings/cli/terminal', {
          afterTabId: activeTabId || undefined,
          focusExisting: true,
          select: true,
        })
      }}
      type="button"
    >
      Open CLI terminal
    </button>
  )
}
function ActiveTabProbe() {
  const { activeTabId } = useShellTabs()
  return <output data-testid="active-tab">{activeTabId}</output>
}

describe('ShellTabStrip', () => {
  let restoreTestBrowser: ShellTabTestBrowser | undefined

  beforeEach(() => {
    restoreTestBrowser = installShellTabTestBrowser()
    class ResizeObserverMock {
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  })
  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    window.location.hash = ''
    restoreTestBrowser?.()
    restoreTestBrowser = undefined
    vi.unstubAllGlobals()
    mockOptimizedLayoutProps.mockReset()
  })
  it('materializes and selects a state-only docs tab in the flex layout model', async () => {
    seedShellTabs([{ id: 'home', name: 'Home', path: '/' }])

    render(
      <ShellTabStrip entry={continuationEntry}>
        <StateOnlyDocsOpener />
      </ShellTabStrip>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Docs' }))

    await waitFor(() => {
      const call = mockOptimizedLayoutProps.mock.calls.at(-1) as
        | [
            {
              model: {
                tabs: Array<{ id: string }>
                actions: Array<{
                  type: string
                  tabId?: string
                  node?: { id: string }
                }>
                selectedTabId: string | null
              }
            },
          ]
        | undefined
      const model = call?.[0].model
      const stored = readShellTabsSnapshot()
      const activeTabId = model?.selectedTabId
      expect(stored.records).toHaveLength(2)
      expect(stored.records.some((tab) => tab.path === '/docs')).toBe(true)
      expect(activeTabId).not.toBe('home')
      expect(model?.tabs.some((tab) => tab.id === activeTabId)).toBe(true)
      expect(model?.selectedTabId).toBe(activeTabId)
      expect(
        model?.actions.some(
          (action) =>
            action.type === 'addNode' && action.node?.id === activeTabId,
        ),
      ).toBe(true)
      expect(
        model?.actions.some(
          (action) =>
            action.type === 'selectTab' && action.tabId === activeTabId,
        ),
      ).toBe(true)
    })
  })

  it('activates a committed fresh record before the Web Lock releases', async () => {
    seedShellTabs([{ id: 'existing', name: 'Files', path: '/files' }])
    let releaseMutation!: () => void
    const mutationRelease = new Promise<void>((resolve) => {
      releaseMutation = resolve
    })
    let mutationCommitted!: () => void
    const committed = new Promise<void>((resolve) => {
      mutationCommitted = resolve
    })
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: async (
          _name: string,
          _options: unknown,
          callback: (lock: object) => unknown,
        ) => {
          const result = await callback({})
          mutationCommitted()
          await mutationRelease
          return result
        },
      },
    })

    render(
      <ShellTabStrip
        entry={{
          kind: 'fresh',
          path: '/files',
          params: {},
          incarnation: 'fresh-document',
        }}
      >
        <ActiveTabProbe />
      </ShellTabStrip>,
    )

    await committed
    const created = readShellTabsSnapshot().records.find(
      (record) => record.id !== 'existing',
    )
    expect(created).toBeDefined()
    await waitFor(() => {
      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      expect(props.model.tabs).toHaveLength(2)
      expect(props.model.selectedTabId).toBe(created?.id)
      expect(screen.getByTestId('active-tab').textContent).toBe(created?.id)
    })

    releaseMutation()
    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe(created?.id)
    })
  })

  it('fences stale model feedback while a local hash path mutation is pending', async () => {
    seedShellTabs([
      { id: 'other', name: 'Other', path: '/other' },
      { id: 'files', name: 'Files', path: '/files' },
    ])
    const browser = restoreTestBrowser
    if (!browser) throw new Error('Shell Tab test browser is not installed.')

    render(
      <ShellTabStrip entry={continuationEntry}>
        <ActiveTabProbe />
      </ShellTabStrip>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe('other')
    })
    const initialCall = mockOptimizedLayoutProps.mock.calls[0] as
      | [
          {
            model: {
              doAction: (action: { type: string; tabId?: string }) => void
            }
            onModelChange?: (model: unknown) => void
          },
        ]
      | undefined
    const staleModelChange = initialCall?.[0].onModelChange
    const model = initialCall?.[0].model
    expect(staleModelChange).toBeTypeOf('function')
    expect(model).toBeDefined()

    await act(async () => {
      model?.doAction({ type: 'selectTab', tabId: 'files' })
    })
    await waitFor(() => {
      expect(screen.getByTestId('active-tab').textContent).toBe('files')
      expect(getAppPath()).toBe('/files')
    })

    const navigations: string[] = []
    const onHashChange = () => navigations.push(getAppPath())
    window.addEventListener('hashchange', onHashChange)
    const blockedMutation = browser.blockNextMutation()
    act(() => {
      window.location.hash = '#/docs'
    })
    await blockedMutation

    await act(async () => {
      staleModelChange?.(model)
    })
    expect(getAppPath()).toBe('/docs')

    browser.releaseBlockedMutation()
    await waitFor(() => {
      expect(
        readShellTabsSnapshot().records.find((record) => record.id === 'files')
          ?.path,
      ).toBe('/docs')
      expect(getAppPath()).toBe('/docs')
    })
    window.removeEventListener('hashchange', onHashChange)
    expect(navigations).not.toContain('/files')
  })

  it('preserves every shared record while projecting an empty stored model', async () => {
    seedShellTabs([
      { id: 'home', name: 'Home', path: '/' },
      { id: 'blog', name: 'Blog', path: '/blog' },
    ])
    sessionStorage.setItem(
      'shell-tabs-layout',
      JSON.stringify({
        nonce: 4,
        model: {
          global: {},
          layout: {
            type: 'row',
            weight: 100,
            children: [
              {
                type: 'tabset',
                id: 'shell-tabset',
                selected: 0,
                children: [],
              },
            ],
          },
        },
      }),
    )

    render(<ShellTabStrip entry={continuationEntry} />)

    await waitFor(() => {
      const call = mockOptimizedLayoutProps.mock.calls.at(-1) as
        | [
            {
              model: {
                tabs: Array<{ id: string }>
                selectedTabId: string | null
              }
            },
          ]
        | undefined
      expect(call?.[0].model.tabs.map((tab) => tab.id)).toEqual([
        'home',
        'blog',
      ])
      expect(readShellTabsSnapshot().records).toHaveLength(2)
      expect(call?.[0].model.selectedTabId).toBe('home')
    })
  })

  it('retains the shell tabset while records hydrate asynchronously', async () => {
    localStorage.clear()
    resetBrowserShellTabsStoreForTests()

    render(<ShellTabStrip entry={continuationEntry} />)

    const store = getBrowserShellTabsStore()
    await waitFor(() => {
      expect(store.getSnapshot().records).toHaveLength(1)
    })

    await store.create({ id: 'blog', path: '/blog', name: 'Blog' })

    await waitFor(() => {
      const call = mockOptimizedLayoutProps.mock.calls.at(-1) as
        | [{ model: { tabs: Array<{ id: string }> } }]
        | undefined
      expect(call?.[0].model.tabs.map((tab) => tab.id)).toHaveLength(2)
      expect(readShellTabsSnapshot().records).toHaveLength(2)
    })
  })

  it('opens and selects the command-line terminal in the active flex tabset with the Terminal name', async () => {
    seedShellTabs([
      { id: 'home', name: 'Home', path: '/' },
      { id: 'blog', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabStrip entry={continuationEntry}>
        <FlexCliCommandProbe />
      </ShellTabStrip>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open CLI terminal' }))

    await waitFor(() => {
      const stored = readShellTabsSnapshot()
      expect(stored.records).toHaveLength(3)
      expect(stored.records.map((tab) => tab.path)).toEqual(
        expect.arrayContaining(['/', '/u/7/settings/cli/terminal', '/blog']),
      )
      expect(getAppPath()).toBe('/u/7/settings/cli/terminal')
      expect(window.location.hash).toBe('#/u/7/settings/cli/terminal')
    })
  })

  it('does not synthesize blank shell routes from model-only flex tabs', async () => {
    seedShellTabs([{ id: 'home', name: 'Home', path: '/' }])

    render(<ShellTabStrip entry={continuationEntry} />)

    const call = mockOptimizedLayoutProps.mock.calls.at(-1) as
      | [
          {
            model: {
              doAction: (action: {
                type: string
                tabId?: string
                node?: { id: string; name: string }
              }) => void
            }
            onModelChange?: (model: unknown) => void
          },
        ]
      | undefined
    const props = call?.[0]
    if (!props?.onModelChange) {
      throw new Error('layout did not provide onModelChange')
    }

    act(() => {
      props.model.doAction({
        type: 'addNode',
        node: { id: 'model-only', name: 'Model Only' },
      })
      props.model.doAction({ type: 'selectTab', tabId: 'model-only' })
      props.onModelChange?.(props.model)
    })

    await waitFor(() => {
      const stored = readShellTabsSnapshot()
      expect(stored.records).toEqual([
        { id: 'home', name: 'Home', path: '/', creationSequence: 1 },
      ])
    })
  })
})
