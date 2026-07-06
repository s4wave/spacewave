import { afterEach, describe, expect, it, beforeEach, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import {
  APP_DRAG_MIME,
  APP_DRAG_VERSION,
  clearActiveAppDragEnvelope,
  writeAppDragEnvelope,
} from '@s4wave/web/dnd/app-drag.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'

const mockUseResourceValue = vi.hoisted(() => vi.fn())
const mockUseAccessTypedHandle = vi.hoisted(() => vi.fn())
const mockBaseLayout = vi.hoisted(() => vi.fn())

type MockFlexLayoutProps = {
  onExternalDrag?: (event: unknown) => unknown
  onContextMenu?: (
    node: unknown,
    event: {
      preventDefault: () => void
      clientX: number
      clientY: number
    },
  ) => void
  onRenderTab?: (
    node: unknown,
    renderValues: { content: ReactNode; buttons: ReactNode[] },
  ) => void
  onAction?: (action: { type: string; data?: unknown }) => unknown
  icons?: { close?: ReactNode }
}

type MockBaseLayoutProps = {
  flexLayoutProps?: MockFlexLayoutProps
}

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mockUseResourceValue,
}))

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: mockUseAccessTypedHandle,
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'world/layout-1',
}))

vi.mock('@s4wave/web/state', () => ({
  useStateNamespace: () => ['layout', 'world/layout-1'],
  useStateAtom: () => [{ tabSetSelected: {} }, vi.fn()],
}))

vi.mock('@s4wave/web/layout/BaseLayout.js', () => ({
  BaseLayout: (props: MockBaseLayoutProps) => {
    mockBaseLayout(props)
    return (
      <div
        data-testid="base-layout"
        data-has-external-drag={
          typeof props.flexLayoutProps?.onExternalDrag === 'function'
            ? 'yes'
            : 'no'
        }
        data-has-context-menu={
          typeof props.flexLayoutProps?.onContextMenu === 'function'
            ? 'yes'
            : 'no'
        }
        data-has-render-tab={
          typeof props.flexLayoutProps?.onRenderTab === 'function'
            ? 'yes'
            : 'no'
        }
        data-has-close-icon={props.flexLayoutProps?.icons?.close ? 'yes' : 'no'}
      />
    )
  },
}))

vi.mock('@s4wave/web/ui/DropdownMenu.js', () => ({
  DropdownMenu: ({ children, open }: { children: ReactNode; open?: boolean }) =>
    open === false ? null : (
      <div data-testid={open ? 'context-menu' : undefined}>{children}</div>
    ),
  DropdownMenuTrigger: ({ children }: { children: ReactNode }) => (
    <>{children}</>
  ),
  DropdownMenuContent: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onClick,
    disabled,
  }: {
    children: ReactNode
    onClick?: () => void
    disabled?: boolean
  }) => (
    <button disabled={disabled} onClick={onClick} type="button">
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}))

vi.mock('@s4wave/web/ui/DropdownMenuGhostAnchor.js', () => ({
  DropdownMenuGhostAnchor: ({ x, y }: { x: number; y: number }) => (
    <span data-testid="ghost-anchor" data-x={x} data-y={y} />
  ),
}))

vi.mock('./TabContent.js', () => ({
  TabContentContainer: ({ children }: { children?: ReactNode }) => (
    <div data-testid="tab-content-container">{children}</div>
  ),
}))

import { LayoutObjectViewer } from './LayoutObjectViewer.js'

const objectInfo = {
  info: {
    case: 'worldObjectInfo',
    value: {
      objectKey: 'world/layout-1',
      objectType: 'alpha/object-layout',
    },
  },
}

interface MockObjectLayoutTabConfig {
  id: string
  name: string
  enableClose?: boolean
}

function createMockObjectLayoutModel(tabs: MockObjectLayoutTabConfig[]) {
  const actions: unknown[] = []
  let nodes: Array<{
    getType: () => string
    getId: () => string
    getName: () => string
    getModel: () => unknown
    getParent: () => unknown
    getChildren: () => unknown[]
    isEnableClose: () => boolean
    toJson: () => {
      type: string
      id: string
      name: string
      component: string
      config: Uint8Array
      enableClose?: boolean
    }
  }> = []
  const model = {
    doAction: vi.fn((action: unknown) => {
      actions.push(action)
      return undefined
    }),
    visitNodes: vi.fn((callback: (node: (typeof nodes)[number]) => void) => {
      for (const node of nodes) {
        callback(node)
      }
    }),
    getNodeById: vi.fn((id: string) =>
      nodes.find((node) => node.getId() === id),
    ),
  }
  const parent = {
    getType: () => 'tabset',
    getId: () => 'tabset-1',
    getChildren: () => nodes,
  }
  nodes = tabs.map((tab) => ({
    getType: () => 'tab',
    getId: () => tab.id,
    getName: () => tab.name,
    getModel: () => model,
    getParent: () => parent,
    getChildren: () => [],
    isEnableClose: () => tab.enableClose === true,
    toJson: () => ({
      type: 'tab',
      id: tab.id,
      name: tab.name,
      component: 'tab-content',
      config: new Uint8Array([1, 2, 3]),
      enableClose: tab.enableClose,
    }),
  }))

  return { actions, model, nodes }
}

function createUnixFSRowAppDragDataTransfer() {
  const envelope = {
    version: APP_DRAG_VERSION,
    items: [
      {
        id: 'report',
        label: 'report.md',
        capabilities: [
          {
            kind: 'openable',
            value: {
              case: 'object',
              value: {
                objectInfo: {
                  info: {
                    case: 'unixfsObjectInfo',
                    value: {
                      unixfsId: 'files',
                      path: '/docs/report.md',
                    },
                  },
                },
                path: '',
                routePath: '/u/7/so/space-1/-/files/-/docs/report.md',
              },
            },
          },
        ],
      },
    ],
  }
  return {
    types: [APP_DRAG_MIME],
    getData: (format: string) =>
      format === APP_DRAG_MIME ? JSON.stringify(envelope) : '',
  }
}

function createUnixFSRowDragEnterDataTransfer() {
  const envelope = {
    version: APP_DRAG_VERSION,
    items: [
      {
        id: 'report',
        label: 'report.md',
        capabilities: [
          {
            kind: 'openable' as const,
            value: {
              case: 'object' as const,
              value: {
                objectInfo: {
                  info: {
                    case: 'unixfsObjectInfo' as const,
                    value: {
                      unixfsId: 'files',
                      path: '/docs/report.md',
                    },
                  },
                },
                path: '',
                routePath: '/u/7/so/space-1/-/files/-/docs/report.md',
              },
            },
          },
        ],
      },
    ],
  }
  writeAppDragEnvelope({ setData: vi.fn() }, envelope)
  return {
    types: [APP_DRAG_MIME],
    getData: () => '',
  }
}

describe('LayoutObjectViewer', () => {
  beforeEach(() => {
    cleanup()
    clearActiveAppDragEnvelope()
    mockUseAccessTypedHandle.mockReset()
    mockUseAccessTypedHandle.mockReturnValue({
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    mockUseResourceValue.mockReset()
    mockBaseLayout.mockReset()
  })

  afterEach(() => {
    clearActiveAppDragEnvelope()
  })

  it('renders loading state while the layout host is unresolved', () => {
    mockUseResourceValue.mockReturnValue(null)

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    expect(screen.getByText('Loading layout')).toBeDefined()
  })

  it('renders failure state when the layout host failed to load', () => {
    mockUseResourceValue.mockReturnValue(undefined)

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    expect(screen.getByText('Failed to load layout')).toBeDefined()
  })

  it('passes an external-drag handler into BaseLayout when the host is ready', () => {
    mockUseResourceValue.mockReturnValue({ id: 'layout-host' })

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    expect(
      screen.getByTestId('base-layout').getAttribute('data-has-external-drag'),
    ).toBe('yes')
    expect(
      screen.getByTestId('base-layout').getAttribute('data-has-context-menu'),
    ).toBe('yes')
    expect(
      screen.getByTestId('base-layout').getAttribute('data-has-render-tab'),
    ).toBe('yes')
    expect(
      screen.getByTestId('base-layout').getAttribute('data-has-close-icon'),
    ).toBe('yes')
  })

  it('renders inline rename and a shell-shaped close button for object tabs', () => {
    mockUseResourceValue.mockReturnValue({ id: 'layout-host' })

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    const props = mockBaseLayout.mock.calls.at(-1)?.[0] as
      | MockBaseLayoutProps
      | undefined
    const onRenderTab = props?.flexLayoutProps?.onRenderTab
    if (typeof onRenderTab !== 'function') {
      throw new Error('LayoutObjectViewer did not pass onRenderTab')
    }

    const { model, nodes } = createMockObjectLayoutModel([
      { id: 'tab-a', name: 'Alpha' },
      { id: 'tab-b', name: 'Beta' },
    ])
    const renderValues = { content: null as ReactNode, buttons: [] }

    onRenderTab(nodes[0], renderValues)

    render(
      <>
        {renderValues.content}
        {renderValues.buttons}
      </>,
    )

    const closeButton = screen.getByRole('button', { name: /close alpha/i })
    expect(closeButton.className).toContain('flexlayout__tab_button_trailing')
    fireEvent.click(closeButton)
    expect(model.doAction).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'FlexLayout_DeleteTab',
        data: { node: 'tab-a' },
      }),
    )

    model.doAction.mockClear()
    fireEvent.doubleClick(screen.getByText('Alpha'))
    const input = screen.getByLabelText('Rename tab-a')
    fireEvent.change(input, { target: { value: 'Proof' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(model.doAction).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'FlexLayout_RenameTab',
        data: { node: 'tab-a', text: 'Proof' },
      }),
    )
  })

  it('uses the shared tab menu for object tab actions', async () => {
    mockUseResourceValue.mockReturnValue({ id: 'layout-host' })

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    const props = mockBaseLayout.mock.calls.at(-1)?.[0] as
      | MockBaseLayoutProps
      | undefined
    const onContextMenu = props?.flexLayoutProps?.onContextMenu
    if (typeof onContextMenu !== 'function') {
      throw new Error('LayoutObjectViewer did not pass onContextMenu')
    }

    const { model, nodes } = createMockObjectLayoutModel([
      { id: 'tab-a', name: 'Alpha' },
      { id: 'tab-b', name: 'Beta' },
    ])
    const preventDefault = vi.fn()

    act(() => {
      onContextMenu(nodes[0], { preventDefault, clientX: 20, clientY: 30 })
    })

    expect(preventDefault).toHaveBeenCalledOnce()
    expect(screen.getByTestId('ghost-anchor').getAttribute('data-x')).toBe('20')
    expect(screen.getByTestId('ghost-anchor').getAttribute('data-y')).toBe('30')
    expect(screen.queryByRole('button', { name: /^new tab$/i })).toBeNull()
    expect(
      screen.queryByRole('button', { name: /open in new tab/i }),
    ).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /duplicate tab/i }))
    fireEvent.click(screen.getByRole('button', { name: /close other tabs/i }))
    fireEvent.click(screen.getByRole('button', { name: /^close tab$/i }))

    await waitFor(() => {
      expect(model.doAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'FlexLayout_AddNode',
          data: expect.objectContaining({
            toNode: 'tabset-1',
            index: 1,
            select: true,
          }),
        }),
      )
      expect(model.doAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'FlexLayout_DeleteTab',
          data: { node: 'tab-b' },
        }),
      )
      expect(model.doAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'FlexLayout_DeleteTab',
          data: { node: 'tab-a' },
        }),
      )
    })
  })

  it('blocks the final object tab from closing', () => {
    mockUseResourceValue.mockReturnValue({ id: 'layout-host' })

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    const props = mockBaseLayout.mock.calls.at(-1)?.[0] as
      | MockBaseLayoutProps
      | undefined
    const { nodes } = createMockObjectLayoutModel([
      { id: 'tab-a', name: 'Alpha' },
    ])
    const renderValues = { content: null as ReactNode, buttons: [] }

    props?.flexLayoutProps?.onRenderTab?.(nodes[0], renderValues)

    expect(
      props?.flexLayoutProps?.onAction?.({
        type: 'FlexLayout_DeleteTab',
        data: { node: 'tab-a' },
      }),
    ).toBeUndefined()

    render(<>{renderValues.buttons}</>)
    const closeButton = screen.getByRole('button', { name: /close alpha/i })
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('expected object tab close button')
    }
    expect(closeButton.disabled).toBe(true)
  })

  it('uses the live viewer handler to accept openable drags and reject unsupported drags', () => {
    mockUseResourceValue.mockReturnValue({ id: 'layout-host' })

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    const props = mockBaseLayout.mock.calls.at(-1)?.[0] as
      | { flexLayoutProps?: { onExternalDrag?: (event: unknown) => unknown } }
      | undefined
    const onExternalDrag = props?.flexLayoutProps?.onExternalDrag
    if (typeof onExternalDrag !== 'function') {
      throw new Error('LayoutObjectViewer did not pass onExternalDrag')
    }

    const accepted = onExternalDrag({
      dataTransfer: {
        ...createUnixFSRowAppDragDataTransfer(),
      },
    })

    const layoutTab = ObjectLayoutTab.fromBinary(
      (accepted as { json: { config: Uint8Array } }).json.config,
    )
    expect(layoutTab).toMatchObject({
      objectInfo: {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/docs/report.md',
          },
        },
      },
    })

    const rejected = onExternalDrag({
      dataTransfer: {
        types: [APP_DRAG_MIME],
        getData: () =>
          JSON.stringify({
            version: APP_DRAG_VERSION,
            items: [
              {
                id: 'folder',
                capabilities: [
                  {
                    kind: 'movable',
                    value: {
                      case: 'unixfs-entry',
                      value: {
                        unixfsId: 'files',
                        path: '/docs',
                        isDir: true,
                      },
                    },
                  },
                ],
              },
            ],
          }),
      },
    })

    expect(rejected).toBeUndefined()
  })

  it('accepts openable drags on dragenter before custom drag data becomes readable', () => {
    mockUseResourceValue.mockReturnValue({ id: 'layout-host' })

    render(
      <LayoutObjectViewer
        objectInfo={objectInfo as never}
        worldState={null as never}
      />,
    )

    const props = mockBaseLayout.mock.calls.at(-1)?.[0] as
      | { flexLayoutProps?: { onExternalDrag?: (event: unknown) => unknown } }
      | undefined
    const onExternalDrag = props?.flexLayoutProps?.onExternalDrag
    if (typeof onExternalDrag !== 'function') {
      throw new Error('LayoutObjectViewer did not pass onExternalDrag')
    }

    const accepted = onExternalDrag({
      dataTransfer: createUnixFSRowDragEnterDataTransfer(),
    })
    const layoutTab = ObjectLayoutTab.fromBinary(
      (accepted as { json: { config: Uint8Array } }).json.config,
    )
    expect(layoutTab).toMatchObject({
      objectInfo: {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/docs/report.md',
          },
        },
      },
    })
  })
})
