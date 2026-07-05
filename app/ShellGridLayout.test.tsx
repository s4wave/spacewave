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
import {
  APP_DRAG_MIME,
  clearActiveAppDragEnvelope,
  writeAppDragEnvelope,
} from '@s4wave/web/dnd/app-drag.js'

import { ShellGridLayout } from './ShellGridLayout.js'
import {
  ShellTabsProvider,
  SHELL_TABS_STORAGE_KEY,
  useShellTabs,
} from './ShellTabContext.js'
import { buildUnixFSEntryAppDragEnvelope } from './unixfs/unixfs-app-drag.js'

interface MockOptimizedLayoutProps {
  model: {
    borders: Array<{
      selected?: number
      children: Array<{ id?: string }>
    }>
    tabs: Array<{ id: string }>
    tabsets: Array<{
      selected?: number
      children: Array<{ id: string }>
    }>
    visitNodes: (
      callback: (node: { getType(): string; getId(): string }) => void,
    ) => void
    doAction?: (action?: MockLayoutAction) => void
  }
  onContextMenu?: unknown
  onExternalDrag?: (event: unknown) => unknown
  onModelChange?: (model: MockOptimizedLayoutProps['model']) => void
  onRenderTab?: unknown
}

interface MockHasGridLayoutModel {
  tabsets?: unknown[]
}

interface MockLayoutAction {
  type?: string
  tabId?: string
  node?: { id?: string; name?: string }
  tabsetId?: string
}

interface MockJsonTab {
  type: 'tab'
  id: string
  name: string
  config?: { path: string }
}

interface MockJsonTabset {
  type: 'tabset'
  id: string
  selected?: number
  children: MockJsonTab[]
}

interface MockJsonBorder {
  type: 'border'
  location: 'left' | 'right' | 'top' | 'bottom'
  selected?: number
  children: MockJsonTab[]
}

interface MockJsonModel {
  borders?: MockJsonBorder[]
  layout: {
    type: 'row'
    children: MockJsonTabset[]
  }
}

const mockNavigate = vi.fn()
const mockOptimizedLayoutProps = vi.hoisted(() =>
  vi.fn<(props: MockOptimizedLayoutProps) => void>(),
)
const mockHasGridLayout = vi.hoisted(() =>
  vi.fn((model: MockHasGridLayoutModel) => (model.tabsets?.length ?? 0) > 1),
)

const mockJsonModel: MockJsonModel = {
  layout: {
    type: 'row',
    children: [
      {
        type: 'tabset',
        id: 'tabset-1',
        children: [
          { type: 'tab', id: 'tab-1', name: 'Docs', config: { path: '/docs' } },
          { type: 'tab', id: 'tab-2', name: 'Home', config: { path: '/' } },
        ],
      },
      {
        type: 'tabset',
        id: 'tabset-2',
        children: [
          { type: 'tab', id: 'tab-3', name: 'Blog', config: { path: '/blog' } },
        ],
      },
    ],
  },
}

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ layoutData: 'grid-layout' }),
}))

vi.mock('./ShellGridPanel.js', () => ({
  ShellGridPanel: ({ tabId }: { tabId: string }) => <div>{tabId}</div>,
}))

vi.mock('./ShellTabLabel.js', () => ({
  ShellTabLabel: ({ tab }: { tab: { name: string } }) => (
    <span>{tab.name}</span>
  ),
}))

vi.mock('@s4wave/web/ui/DropdownMenu.js', () => ({
  DropdownMenu: ({
    children,
    open,
  }: {
    children: React.ReactNode
    open?: boolean
  }) =>
    open === false ? null : (
      <div data-testid={open ? 'context-menu' : undefined}>{children}</div>
    ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
  }) => (
    <button disabled={disabled} onClick={onClick} type="button">
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}))

vi.mock('./shell-grid-utils.js', () => ({
  decodeGridLayout: () => ({ model: mockJsonModel, localState: undefined }),
  encodeGridLayout: () => 'encoded-grid',
  encodeGridLayoutStructure: (model: { tabs?: Array<{ id: string }> }) =>
    model.tabs?.map((tab) => tab.id).join('|') ?? 'grid-structure',
  hasGridLayout: (model: MockHasGridLayoutModel) => mockHasGridLayout(model),
  getSelectedTabId: () => 'tab-1',
  getActiveTabsetId: () => 'tabset-1',
  applyLocalStateToModel: () => {},
  SHELL_GRID_BASE_MODEL: {},
}))

vi.mock('@aptre/flex-layout', () => {
  class MockTabSetNode {
    id: string
    selected?: number
    children: MockTabNode[]

    constructor(id: string, selected: number | undefined) {
      this.id = id
      this.selected = selected
      this.children = []
    }

    getType() {
      return 'tabset'
    }

    getId() {
      return this.id
    }

    getSelectedNode() {
      return this.children[this.selected ?? 0] ?? null
    }

    isActive() {
      return this.id === 'tabset-1'
    }
  }

  class MockTabNode {
    id: string
    name: string
    config: { path?: string }
    parent: MockTabSetNode

    constructor(
      id: string,
      name: string,
      config: { path?: string },
      parent: MockTabSetNode,
    ) {
      this.id = id
      this.name = name
      this.config = config
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

    getConfig() {
      return this.config
    }
  }

  class MockModel {
    borders: Array<{
      selected?: number
      children: Array<{ id?: string }>
    }>
    tabsets: MockTabSetNode[]
    tabs: MockTabNode[]

    constructor(json: typeof mockJsonModel) {
      this.borders = (json.borders ?? []).map((border) => ({
        selected: border.selected,
        children: border.children.map((child) => ({ id: child.id })),
      }))
      this.tabsets = []
      this.tabs = []

      for (const tabsetJson of json.layout.children) {
        const tabset = new MockTabSetNode(tabsetJson.id, tabsetJson.selected)
        this.tabsets.push(tabset)
        for (const tabJson of tabsetJson.children) {
          const tab = new MockTabNode(
            tabJson.id,
            tabJson.name,
            tabJson.config ?? {},
            tabset,
          )
          tabset.children.push(tab)
          this.tabs.push(tab)
        }
      }
    }

    static fromJson(json: typeof mockJsonModel) {
      return new MockModel(json)
    }

    visitNodes(callback: (node: MockTabSetNode | MockTabNode) => void) {
      for (const tabset of this.tabsets) {
        callback(tabset)
        for (const tab of tabset.children) {
          callback(tab)
        }
      }
    }

    getNodeById(id: string) {
      return (
        this.tabs.find((tab) => tab.id === id) ??
        this.tabsets.find((tabset) => tabset.id === id) ??
        null
      )
    }

    doAction(action?: MockLayoutAction) {
      if (!action) return
      if (action.type === 'addNode' && action.node?.id) {
        const tabset =
          this.tabsets.find((item) => item.id === action.tabsetId) ??
          this.tabsets[0]
        if (!tabset) return
        const tab = new MockTabNode(
          action.node.id,
          action.node.name ?? 'Tab',
          {},
          tabset,
        )
        tabset.children.push(tab)
        tabset.selected = tabset.children.length - 1
        this.tabs.push(tab)
      }
      if (action.type === 'selectTab' && action.tabId) {
        const tab = this.tabs.find((item) => item.id === action.tabId)
        if (!tab) return
        const idx = tab.parent.children.findIndex((item) => item.id === tab.id)
        if (idx >= 0) {
          tab.parent.selected = idx
        }
      }
      if (action.type === 'deleteTab' && action.tabId) {
        this.tabs = this.tabs.filter((tab) => tab.id !== action.tabId)
        for (const tabset of this.tabsets) {
          tabset.children = tabset.children.filter(
            (tab) => tab.id !== action.tabId,
          )
          if ((tabset.selected ?? 0) >= tabset.children.length) {
            tabset.selected = Math.max(0, tabset.children.length - 1)
          }
        }
      }
    }
  }

  function OptimizedLayout({
    model,
    onContextMenu,
    onExternalDrag,
    onModelChange,
    onRenderTab,
  }: {
    model: MockModel
    onContextMenu?: (
      node: MockTabNode,
      event: React.MouseEvent<HTMLButtonElement>,
    ) => void
    onExternalDrag?: (event: unknown) => unknown
    onModelChange?: (model: MockOptimizedLayoutProps['model']) => void
    onRenderTab?: (
      node: MockTabNode,
      renderValues: { content?: React.ReactNode },
    ) => void
  }) {
    mockOptimizedLayoutProps({
      model,
      onContextMenu,
      onExternalDrag,
      onModelChange,
      onRenderTab,
    })
    return (
      <div>
        <div data-testid="external-drag-enabled">
          {onExternalDrag ? 'yes' : 'no'}
        </div>
        {model.tabs.map((tab) => {
          const renderValues: { content?: React.ReactNode } = {}
          onRenderTab?.(tab, renderValues)
          return (
            <button
              key={tab.id}
              onContextMenu={(event) => onContextMenu?.(tab, event)}
              type="button"
            >
              {renderValues.content ?? tab.getName()}
            </button>
          )
        })}
      </div>
    )
  }

  return {
    Actions: {
      addNode: vi.fn(
        (node: { id?: string; name?: string }, tabsetId?: string) => ({
          type: 'addNode',
          node,
          tabsetId,
        }),
      ),
      deleteTab: vi.fn((tabId: string) => ({ type: 'deleteTab', tabId })),
      selectTab: vi.fn((tabId: string) => ({ type: 'selectTab', tabId })),
    },
    BorderNode: class {},
    DockLocation: {
      CENTER: 'center',
    },
    Model: MockModel,
    OptimizedLayout,
    TabNode: MockTabNode,
    TabSetNode: MockTabSetNode,
  }
})

function RenamingStateProbe() {
  const { renamingTabId } = useShellTabs()
  return <div data-testid="renaming-tab-id">{renamingTabId ?? ''}</div>
}

function GridDocsCommandProbe() {
  const { activeTabId, openPathInNewTab } = useShellTabs()
  return (
    <button
      type="button"
      onClick={() =>
        openPathInNewTab('/docs', {
          afterTabId: activeTabId,
          focusExisting: true,
        })
      }
    >
      Open Docs
    </button>
  )
}

function GridCliCommandProbe() {
  const { activeTabId, openPathInActiveTabset } = useShellTabs()
  return (
    <button
      type="button"
      onClick={() =>
        openPathInActiveTabset('/u/7/settings/cli/terminal', {
          afterTabId: activeTabId,
          focusExisting: true,
          select: true,
        })
      }
    >
      Open CLI terminal
    </button>
  )
}

function createUnixFSRowDragEvent() {
  const envelope = buildUnixFSEntryAppDragEnvelope({
    entry: {
      id: 'report',
      name: 'report.md',
      isDir: false,
    },
    currentPath: '/docs',
    sessionIndex: 7,
    spaceId: 'space-1',
    unixfsId: 'files',
  })
  if (!envelope) {
    throw new Error('failed to build UnixFS row drag envelope')
  }
  return {
    dataTransfer: {
      types: [APP_DRAG_MIME],
      getData: (format: string) =>
        format === APP_DRAG_MIME ? JSON.stringify(envelope) : '',
    },
  }
}

function createUnixFSRowDragEnterEvent() {
  const envelope = buildUnixFSEntryAppDragEnvelope({
    entry: {
      id: 'report',
      name: 'report.md',
      isDir: false,
    },
    currentPath: '/docs',
    sessionIndex: 7,
    spaceId: 'space-1',
    unixfsId: 'files',
  })
  if (!envelope) {
    throw new Error('failed to build UnixFS row drag envelope')
  }
  writeAppDragEnvelope({ setData: vi.fn() }, envelope)
  return {
    dataTransfer: {
      types: [APP_DRAG_MIME],
      getData: () => '',
    },
  }
}

describe('ShellGridLayout', () => {
  beforeEach(() => {
    delete mockJsonModel.borders
    for (const tabset of mockJsonModel.layout.children) {
      delete tabset.selected
    }
    clearActiveAppDragEnvelope()
    mockHasGridLayout.mockImplementation(
      (model: MockHasGridLayoutModel) => (model.tabsets?.length ?? 0) > 1,
    )
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    clearActiveAppDragEnvelope()
    vi.clearAllMocks()
    mockOptimizedLayoutProps.mockReset()
  })

  it('prunes decoded grid tabs that are absent from shell tab state', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
        ],
        activeTabId: 'tab-1',
      }),
    )
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    try {
      render(
        <ShellTabsProvider>
          <ShellGridLayout />
        </ShellTabsProvider>,
      )

      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]

      expect(props?.model.tabs.map((tab) => tab.id) ?? []).toEqual([
        'tab-1',
        'tab-2',
      ])
      expect(warnSpy).not.toHaveBeenCalled()
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('remaps tabset selection when the selected decoded tab is pruned', () => {
    mockJsonModel.layout.children[0].selected = 1
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-3', name: 'Blog', path: '/blog' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    expect(props?.model.tabs.map((tab) => tab.id) ?? []).toEqual([
      'tab-1',
      'tab-3',
    ])
    expect(props?.model.tabsets[0]?.children.map((tab) => tab.id)).toEqual([
      'tab-1',
    ])
    expect(props?.model.tabsets[0]?.selected).toBe(0)
  })

  it('clears empty border selection when pruning the selected border tab', () => {
    mockJsonModel.borders = [
      {
        type: 'border',
        location: 'left',
        selected: 0,
        children: [{ type: 'tab', id: 'tab-3', name: 'Blog' }],
      },
    ]
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    expect(props?.model.borders).toEqual([{ selected: -1, children: [] }])
  })

  it('preserves closed border selection while pruning grid layout tabs', () => {
    mockJsonModel.borders = [
      {
        type: 'border',
        location: 'left',
        selected: -1,
        children: [{ type: 'tab', id: 'tab-1', name: 'Docs' }],
      },
    ]
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    expect(props?.model.borders).toEqual([
      { selected: -1, children: [{ id: 'tab-1' }] },
    ])
  })

  it('exits grid mode when pruning stale tabs collapses the decoded layout', async () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/docs',
        replace: true,
      })
    })
  })

  it('uses shell tab state path when model changes collapse grid mode', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/shell-docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
          { id: 'tab-3', name: 'Blog', path: '/blog' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    const onModelChange = props?.onModelChange
    if (typeof onModelChange !== 'function' || !props) {
      throw new Error('grid layout did not provide onModelChange')
    }

    props.model.tabsets = [props.model.tabsets[0]]
    props.model.tabs = [props.model.tabs[0]]

    act(() => {
      onModelChange(props.model)
    })

    expect(mockNavigate).toHaveBeenCalledWith({
      path: '/shell-docs',
      replace: true,
    })
  })

  it('opens the shared tab context menu in grid mode and routes rename through tab state', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
          { id: 'tab-3', name: 'Blog', path: '/blog' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <RenamingStateProbe />
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    expect(screen.queryByTestId('context-menu')).toBeNull()

    fireEvent.contextMenu(screen.getByRole('button', { name: 'Docs' }))

    expect(screen.getByTestId('context-menu')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: /rename tab/i }))

    expect(screen.getByTestId('renaming-tab-id').textContent).toBe('tab-1')
  })

  it('projects command-created docs tabs into the active grid tabset', async () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-2', name: 'Home', path: '/' },
          { id: 'tab-3', name: 'Blog', path: '/blog' },
        ],
        activeTabId: 'tab-2',
      }),
    )

    render(
      <ShellTabsProvider>
        <GridDocsCommandProbe />
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Docs' }))

    await waitFor(() => {
      const stored = JSON.parse(
        sessionStorage.getItem(SHELL_TABS_STORAGE_KEY) ?? 'null',
      ) as {
        activeTabId: string
        tabs: Array<{ id: string; name: string; path: string }>
      }
      const activeTab = stored.tabs.find((tab) => tab.id === stored.activeTabId)
      expect(activeTab).toMatchObject({ name: 'Docs', path: '/docs' })
      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      expect(
        props?.model.tabs.some((tab) => tab.id === stored.activeTabId),
      ).toBe(true)
      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/g/encoded-grid',
        replace: true,
      })
    })
  })

  it('projects command-line terminal tabs into the active grid tabset', async () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-2', name: 'Home', path: '/' },
          { id: 'tab-3', name: 'Blog', path: '/blog' },
        ],
        activeTabId: 'tab-2',
      }),
    )

    render(
      <ShellTabsProvider>
        <GridCliCommandProbe />
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open CLI terminal' }))

    await waitFor(() => {
      const stored = JSON.parse(
        sessionStorage.getItem(SHELL_TABS_STORAGE_KEY) ?? 'null',
      ) as {
        activeTabId: string
        tabs: Array<{ id: string; name: string; path: string }>
      }
      const activeTab = stored.tabs.find((tab) => tab.id === stored.activeTabId)
      expect(activeTab).toMatchObject({
        name: 'Settings',
        path: '/u/7/settings/cli/terminal',
      })

      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      const activeGridTab = props?.model.tabs.find(
        (tab) => tab.id === stored.activeTabId,
      )
      const selectedGridTab =
        props?.model.tabsets[0]?.children[props.model.tabsets[0].selected ?? -1]
      expect(activeGridTab).toBeDefined()
      expect(selectedGridTab?.id).toBe(stored.activeTabId)
    })
  })

  it('accepts UnixFS row drags through the grid layout external-drag handler', async () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
          { id: 'tab-3', name: 'Blog', path: '/blog' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    expect(screen.getByTestId('external-drag-enabled').textContent).toBe('yes')

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    const onExternalDrag = props?.onExternalDrag
    if (typeof onExternalDrag !== 'function') {
      throw new Error('grid layout did not provide onExternalDrag')
    }

    const externalDrag = onExternalDrag(createUnixFSRowDragEvent())
    expect(externalDrag).toMatchObject({
      json: {
        type: 'tab',
        name: 'report.md',
        component: 'shell-content',
      },
    })

    act(() => {
      ;(externalDrag as { onDrop?: (node?: unknown) => void })?.onDrop?.({
        getId: () => 'dropped-tab',
      })
    })

    await waitFor(() => {
      const stored = JSON.parse(
        sessionStorage.getItem(SHELL_TABS_STORAGE_KEY) ?? 'null',
      ) as {
        activeTabId: string
        tabs: Array<{ id: string; name: string; path: string }>
      }
      expect(stored.activeTabId).toBe('dropped-tab')
      expect(
        stored.tabs.some(
          (tab) =>
            tab.id === 'dropped-tab' &&
            tab.name === 'report.md' &&
            tab.path === '/u/7/so/space-1/-/files/-/docs/report.md',
        ),
      ).toBe(true)
    })
  })

  it('accepts UnixFS row drags on dragenter before custom drag data becomes readable', () => {
    sessionStorage.setItem(
      SHELL_TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'tab-1', name: 'Docs', path: '/docs' },
          { id: 'tab-2', name: 'Home', path: '/' },
        ],
        activeTabId: 'tab-1',
      }),
    )

    render(
      <ShellTabsProvider>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    const onExternalDrag = props?.onExternalDrag
    if (typeof onExternalDrag !== 'function') {
      throw new Error('grid layout did not provide onExternalDrag')
    }

    const externalDrag = onExternalDrag(createUnixFSRowDragEnterEvent())
    expect(externalDrag).toMatchObject({
      json: {
        type: 'tab',
        name: 'report.md',
        component: 'shell-content',
      },
    })
  })
})
