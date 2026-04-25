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

const mockNavigate = vi.fn()
const mockOptimizedLayoutProps = vi.hoisted(() => vi.fn())

const mockJsonModel = {
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
  encodeGridLayoutStructure: () => 'grid-structure',
  hasGridLayout: () => true,
  getSelectedTabId: () => 'tab-1',
  getActiveTabsetId: () => 'tabset-1',
  applyLocalStateToModel: () => {},
  SHELL_GRID_BASE_MODEL: {},
}))

vi.mock('@aptre/flex-layout', () => {
  class MockTabSetNode {
    id: string
    children: MockTabNode[]

    constructor(id: string) {
      this.id = id
      this.children = []
    }

    getType() {
      return 'tabset'
    }

    getId() {
      return this.id
    }

    getSelectedNode() {
      return this.children[0] ?? null
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
    tabsets: MockTabSetNode[]
    tabs: MockTabNode[]

    constructor(json: typeof mockJsonModel) {
      this.tabsets = []
      this.tabs = []

      for (const tabsetJson of json.layout.children) {
        const tabset = new MockTabSetNode(tabsetJson.id)
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

    doAction() {}
  }

  function OptimizedLayout({
    model,
    onContextMenu,
    onExternalDrag,
    onRenderTab,
  }: {
    model: MockModel
    onContextMenu?: (
      node: MockTabNode,
      event: React.MouseEvent<HTMLButtonElement>,
    ) => void
    onExternalDrag?: unknown
    onRenderTab?: (
      node: MockTabNode,
      renderValues: { content?: React.ReactNode },
    ) => void
  }) {
    mockOptimizedLayoutProps({
      model,
      onContextMenu,
      onExternalDrag,
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
      addNode: vi.fn(),
      deleteTab: vi.fn(),
      selectTab: vi.fn(),
    },
    BorderNode: class {},
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
    clearActiveAppDragEnvelope()
  })

  afterEach(() => {
    cleanup()
    localStorage.clear()
    clearActiveAppDragEnvelope()
    vi.clearAllMocks()
    mockOptimizedLayoutProps.mockReset()
  })

  it('opens the shared tab context menu in grid mode and routes rename through tab state', () => {
    localStorage.setItem(
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

  it('accepts UnixFS row drags through the grid layout external-drag handler', async () => {
    localStorage.setItem(
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

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0] as
      | { onExternalDrag?: (event: unknown) => unknown }
      | undefined
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
        localStorage.getItem(SHELL_TABS_STORAGE_KEY) ?? 'null',
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
    localStorage.setItem(
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

    const props = mockOptimizedLayoutProps.mock.calls.at(-1)?.[0] as
      | { onExternalDrag?: (event: unknown) => unknown }
      | undefined
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
