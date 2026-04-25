import { afterEach, describe, expect, it, beforeEach, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
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
  BaseLayout: (props: { flexLayoutProps?: { onExternalDrag?: unknown } }) => {
    mockBaseLayout(props)
    return (
      <div
        data-testid="base-layout"
        data-has-external-drag={
          typeof props.flexLayoutProps?.onExternalDrag === 'function' ?
            'yes'
          : 'no'
        }
      />
    )
  },
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
