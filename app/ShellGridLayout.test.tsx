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
import { useShellTabs } from './ShellTabContext.js'
import { ShellTabsProvider } from './ShellTabsProvider.js'
import {
  installShellTabTestBrowser,
  readShellTabsSnapshot,
  seedShellTabs,
} from './ShellTabTestHarness.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'
import {
  buildUnixFSEntryAppDragEnvelope,
  buildUnixFSSelectionAppDragEnvelope,
} from './unixfs/unixfs-app-drag.js'

const continuationEntry: ShellDocumentEntry = {
  kind: 'continuation',
  path: '/',
  params: {},
  incarnation: 'test-document',
}

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
  renderTab?: unknown
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
const mockPanelMounts = vi.hoisted(() => new Map<string, number>())

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
  ShellGridPanel: ({ tabId }: { tabId: string }) => {
    React.useEffect(() => {
      mockPanelMounts.set(tabId, (mockPanelMounts.get(tabId) ?? 0) + 1)
    }, [tabId])
    return <div>{tabId}</div>
  },
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
    renderTab,
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
    renderTab?: (node: MockTabNode) => React.ReactNode
  }) {
    mockOptimizedLayoutProps({
      model,
      onContextMenu,
      onExternalDrag,
      onModelChange,
      onRenderTab,
      renderTab,
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
            <React.Fragment key={tab.id}>
              <button
                onContextMenu={(event) => onContextMenu?.(tab, event)}
                type="button"
              >
                {renderValues.content ?? tab.getName()}
              </button>
              {renderTab?.(tab)}
            </React.Fragment>
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
function createUnixFSSelectionDragEvent() {
  const envelope = buildUnixFSSelectionAppDragEnvelope({
    entries: [
      { id: 'docs', name: 'docs', isDir: true },
      { id: 'report', name: 'report.md', isDir: false },
      { id: 'image', name: 'image.png', isDir: false },
    ],
    currentPath: '/docs',
    sessionIndex: 7,
    spaceId: 'space-1',
    unixfsId: 'files',
  })
  if (!envelope) {
    throw new Error('failed to build UnixFS selection drag envelope')
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

function TrackedNormalPane({ tabId }: { tabId: string }) {
  React.useEffect(() => {
    mockPanelMounts.set(tabId, (mockPanelMounts.get(tabId) ?? 0) + 1)
  }, [tabId])
  return <div>{tabId}</div>
}

function GridRouteHarness() {
  const [path, setPath] = React.useState('/g/grid-layout')

  React.useEffect(() => {
    mockNavigate.mockImplementation(({ path: nextPath }: { path: string }) => {
      setPath(nextPath)
    })
    return () => {
      mockNavigate.mockReset()
    }
  }, [])

  if (!path.startsWith('/g/')) {
    return <TrackedNormalPane tabId="tab-3" />
  }

  return (
    <ShellTabsProvider entry={continuationEntry}>
      <ShellGridLayout />
    </ShellTabsProvider>
  )
}

describe('ShellGridLayout', () => {
  let restoreTestBrowser: (() => void) | undefined

  beforeEach(() => {
    restoreTestBrowser = installShellTabTestBrowser()
    delete mockJsonModel.borders
    for (const tabset of mockJsonModel.layout.children) {
      delete tabset.selected
    }
    clearActiveAppDragEnvelope()
    mockHasGridLayout.mockImplementation(
      (model: MockHasGridLayoutModel) => (model.tabsets?.length ?? 0) > 1,
    )
    mockPanelMounts.clear()
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    clearActiveAppDragEnvelope()
    vi.clearAllMocks()
    mockOptimizedLayoutProps.mockReset()
    restoreTestBrowser?.()
    restoreTestBrowser = undefined
  })

  it('prunes decoded grid tabs that are absent from shell tab state', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
    ])
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    try {
      render(
        <ShellTabsProvider entry={continuationEntry}>
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
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
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
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
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
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    expect(props?.model.borders).toEqual([
      { selected: -1, children: [{ id: 'tab-1' }] },
    ])
  })

  it('keeps grid ownership when pruning stale tabs collapses the decoded layout', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('keeps model changes in the grid route when the layout collapses', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/shell-docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
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
      path: '/g/encoded-grid',
      replace: true,
    })
  })

  it('keeps a surviving pane mounted when closing its horizontal split sibling', async () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-3', name: 'Terminal', path: '/u/7/settings/cli/terminal' },
    ])

    render(<GridRouteHarness />)

    await waitFor(() => {
      expect(mockPanelMounts.get('tab-3')).toBe(1)
    })

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    const onModelChange = props?.onModelChange
    if (typeof onModelChange !== 'function' || !props) {
      throw new Error('grid layout did not provide onModelChange')
    }

    props.model.doAction?.({ type: 'deleteTab', tabId: 'tab-1' })
    props.model.tabsets = [props.model.tabsets[1]]

    act(() => {
      onModelChange(props.model)
    })

    await waitFor(() => {
      expect(mockPanelMounts.get('tab-3')).toBe(1)
    })
  })

  it('opens the shared tab context menu in grid mode and routes rename through tab state', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
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
    seedShellTabs([
      { id: 'tab-2', name: 'Home', path: '/' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <GridDocsCommandProbe />
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open Docs' }))

    await waitFor(() => {
      const stored = readShellTabsSnapshot()
      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      const selectedTabset = props?.model.tabsets[0]
      const selectedId =
        selectedTabset?.children[selectedTabset.selected ?? -1]?.id
      const activeTab = stored.records.find((tab) => tab.id === selectedId)
      expect(activeTab).toMatchObject({ name: 'Docs', path: '/docs' })
      expect(props?.model.tabs.some((tab) => tab.id === selectedId)).toBe(true)
      expect(mockNavigate).toHaveBeenCalledWith({
        path: '/g/encoded-grid',
        replace: true,
      })
    })
  })

  it('projects command-line terminal tabs into the active grid tabset with the Terminal name', async () => {
    seedShellTabs([
      { id: 'tab-2', name: 'Home', path: '/' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <GridCliCommandProbe />
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open CLI terminal' }))

    await waitFor(() => {
      const stored = readShellTabsSnapshot()
      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      const selectedTabset = props?.model.tabsets[0]
      const selectedId =
        selectedTabset?.children[selectedTabset.selected ?? -1]?.id
      const activeTab = stored.records.find((tab) => tab.id === selectedId)
      expect(activeTab).toMatchObject({
        name: 'Terminal',
        path: '/u/7/settings/cli/terminal',
      })
      const activeGridTab = props?.model.tabs.find(
        (tab) => tab.id === selectedId,
      )
      expect(activeGridTab).toBeDefined()
      expect(selectedId).toBe(activeTab?.id)
    })
  })

  it('accepts UnixFS row drags through the grid layout external-drag handler', async () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
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
      const stored = readShellTabsSnapshot()
      const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      const tabset = props?.model.tabsets[0]
      const selectedId = tabset?.children[tabset.selected ?? -1]?.id
      expect(selectedId).toBe('dropped-tab')
      expect(
        stored.records.some(
          (tab) =>
            tab.id === 'dropped-tab' &&
            tab.name === 'report.md' &&
            tab.path === '/u/7/so/space-1/-/files/-/docs/report.md',
        ),
      ).toBe(true)
    })
  })

  it('opens a dragged UnixFS selection in order within the dropped tabset', async () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
      { id: 'tab-3', name: 'Blog', path: '/blog' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
        <ShellGridLayout />
      </ShellTabsProvider>,
    )

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
    const onExternalDrag = props?.onExternalDrag
    if (typeof onExternalDrag !== 'function') {
      throw new Error('grid layout did not provide onExternalDrag')
    }

    const externalDrag = onExternalDrag(createUnixFSSelectionDragEvent())
    expect(externalDrag).toMatchObject({
      json: {
        type: 'tab',
        name: 'docs',
        component: 'shell-content',
      },
    })

    const droppedNode = { getId: () => 'dropped-tab' }
    const externalDrop = externalDrag as {
      onDrop?: (node?: unknown) => void
    }
    act(() => {
      externalDrop.onDrop?.(droppedNode)
    })

    await waitFor(() => {
      const stored = readShellTabsSnapshot()
      const draggedTabs = stored.records.filter((tab) =>
        tab.path.startsWith('/u/7/so/space-1/-/files/'),
      )
      expect(draggedTabs.map((tab) => tab.name)).toEqual([
        'docs',
        'report.md',
        'image.png',
      ])
      expect(draggedTabs.map((tab) => tab.path)).toEqual([
        '/u/7/so/space-1/-/files/-/docs/docs',
        '/u/7/so/space-1/-/files/-/docs/report.md',
        '/u/7/so/space-1/-/files/-/docs/image.png',
      ])

      const latestProps = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0]
      const tabset = latestProps?.model.tabsets[0]
      expect(tabset?.children.slice(-3).map((tab) => tab.id)).toEqual(
        draggedTabs.map((tab) => tab.id),
      )
      expect(tabset?.children[tabset.selected ?? -1]?.id).toBe('dropped-tab')
    })
  })

  it('accepts UnixFS row drags on dragenter before custom drag data becomes readable', () => {
    seedShellTabs([
      { id: 'tab-1', name: 'Docs', path: '/docs' },
      { id: 'tab-2', name: 'Home', path: '/' },
    ])

    render(
      <ShellTabsProvider entry={continuationEntry}>
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
