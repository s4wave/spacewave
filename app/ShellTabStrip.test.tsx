import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import {
  addTab as addShellTab,
  SHELL_TABS_STORAGE_KEY,
  useShellTabs,
} from './ShellTabContext.js'
import { ShellTabStrip } from './ShellFlexLayout.js'

const mockOptimizedLayoutProps = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/state/index.js', async () => {
  const React = await import('react')
  const actual = await vi.importActual<
    typeof import('@s4wave/web/state/index.js')
  >('@s4wave/web/state/index.js')
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
    tabs: MockTabNode[]
    actions: Array<{
      type: string
      tabId?: string
      node?: { id: string; name: string }
    }>

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
        typeof tabsetJson.selected === 'number' ?
          (tabsetJson.children[tabsetJson.selected]?.id ?? null)
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
      callback(this.tabset)
      for (const tab of this.tabs) {
        callback(tab)
      }
    }

    getNodeById(id: string) {
      return (
        this.tabs.find((tab) => tab.id === id) ??
        (this.tabset.id === id ? this.tabset : null)
      )
    }

    get selectedTabId() {
      return this.tabset.selectedTabId
    }

    doAction(action: {
      type: string
      tabId?: string
      node?: { id: string; name: string }
      name?: string
    }) {
      this.actions.push(action)

      if (action.type === 'addNode' && action.node) {
        const tab = new MockTabNode(
          action.node.id,
          action.node.name,
          this.tabset,
        )
        this.tabset.children.push(tab)
        this.tabs.push(tab)
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
    }
  }

  function OptimizedLayout({ model }: { model: MockModel }) {
    mockOptimizedLayoutProps({ model })
    return <div data-testid="layout-tab-count">{model.tabs.length}</div>
  }

  return {
    Actions: {
      addNode: (node: { id: string; name: string }) => ({
        type: 'addNode',
        node,
      }),
      deleteTab: (tabId: string) => ({ type: 'deleteTab', tabId }),
      selectTab: (tabId: string) => ({ type: 'selectTab', tabId }),
      updateModelAttributes: () => ({ type: 'updateModelAttributes' }),
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
  const { tabs, activeTabId, setTabs, setActiveTabId } = useShellTabs()

  return (
    <button
      onClick={() => {
        const result = addShellTab(tabs, '/docs', activeTabId || undefined)
        setTabs(result.tabs)
        setActiveTabId(result.newTab.id)
      }}
      type="button"
    >
      Open Docs
    </button>
  )
}

describe('ShellTabStrip', () => {
  beforeEach(() => {
    class ResizeObserverMock {
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    window.location.hash = ''
    vi.unstubAllGlobals()
    mockOptimizedLayoutProps.mockReset()
  })

  it('materializes and selects a state-only docs tab in the flex layout model', async () => {
    localStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [{ id: 'home', name: 'Home', path: '/' }],
        activeTabId: 'home',
      }),
    )

    render(
      <ShellTabStrip>
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
      const stored = JSON.parse(
        localStorage.getItem(SHELL_TABS_STORAGE_KEY) ?? 'null',
      ) as {
        activeTabId: string
        tabs: Array<{ id: string; path: string }>
      }
      const activeTabId = stored.activeTabId
      expect(stored.tabs).toHaveLength(2)
      expect(stored.tabs.some((tab) => tab.path === '/docs')).toBe(true)
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
})
