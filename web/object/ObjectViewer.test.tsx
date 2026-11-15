import { describe, it, expect, vi, beforeEach } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { ObjectViewer } from './ObjectViewer.js'
import type { ObjectInfo } from './object.pb.js'

const mockUseObjectViewer = vi.hoisted(() => vi.fn())
const mockBottomBarLevel = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: (props: {
    children?: ReactNode
    contextMenuItems?: unknown
    contextMenuLabel?: unknown
  }) => {
    mockBottomBarLevel(props)
    return <div data-testid="bottom-bar-level">{props.children}</div>
  },
}))

vi.mock('@s4wave/web/frame/bottom-bar-root.js', () => ({
  BottomBarRoot: ({ children }: { children?: ReactNode }) => (
    <div data-testid="bottom-bar-root">{children}</div>
  ),
}))

vi.mock('@s4wave/web/frame/ViewerFrame.js', () => ({
  ViewerFrame: ({ children }: { children?: ReactNode }) => (
    <div data-testid="viewer-frame">{children}</div>
  ),
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  HistoryRouter: ({ children }: { children?: ReactNode }) => (
    <div data-testid="history-router">{children}</div>
  ),
}))

vi.mock('@s4wave/web/state', () => ({
  StateNamespaceProvider: ({
    namespace,
    children,
  }: {
    namespace?: string[]
    children?: ReactNode
  }) => (
    <div data-testid="state-namespace" data-namespace={namespace?.join('/')}>
      {children}
    </div>
  ),
}))

vi.mock('./ObjectViewerContent.js', () => ({
  ObjectViewerContent: ({
    standalone,
    component,
  }: {
    standalone?: boolean
    component?: { disablePadding?: boolean }
  }) => (
    <div data-testid="object-viewer-content">
      viewer content
      <span data-testid="content-standalone">
        {standalone ? 'true' : 'false'}
      </span>
      <span data-testid="content-disable-padding">
        {component?.disablePadding ? 'true' : 'false'}
      </span>
    </div>
  ),
}))

vi.mock('./ObjectViewerContext.js', () => ({
  ObjectViewerProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('./useObjectViewer.js', () => ({
  useObjectViewer: mockUseObjectViewer,
}))

function buildViewerResult(overrides: Record<string, unknown> = {}) {
  return {
    objectState: { value: null },
    typeID: undefined,
    rootRef: undefined,
    objectKey: undefined,
    visibleComponents: [],
    selectedComponent: undefined,
    missingComponentID: undefined,
    onSelectComponent: vi.fn(),
    viewerContextValue: {
      visibleComponents: [],
      selectedComponent: undefined,
      onSelectComponent: vi.fn(),
    },
    buttonRender: vi.fn(),
    overlayContent: undefined,
    contextMenuItems: undefined,
    contextMenuLabel: 'Object actions',
    ...overrides,
  }
}

function buildWorldState(value: IWorldState | null): Resource<IWorldState> {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

describe('ObjectViewer', () => {
  beforeEach(() => {
    cleanup()
    mockUseObjectViewer.mockReset()
    mockBottomBarLevel.mockReset()
  })

  it('renders the extracted loading state while viewer state is resolving', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        objectKey: 'world/demo',
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: 'world/demo',
          objectType: '',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState(null)}
      />,
    )

    expect(screen.getByText('Loading the object')).toBeDefined()
    expect(screen.queryByTestId('object-viewer-content')).toBeNull()
  })

  it('renders viewer content once the viewer is ready', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'unixfs/fs-node',
        objectState: { value: { id: 'obj-1' } },
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'unixfsObjectInfo',
        value: {
          unixfsId: 'files',
          path: '/docs',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState(null)}
      />,
    )

    expect(screen.getByTestId('object-viewer-content')).toBeDefined()
    expect(screen.getByTestId('content-standalone').textContent).toBe('false')
    expect(screen.queryByText('Loading the object')).toBeNull()
  })

  it('renders typed world viewers that do not require the generic object handle', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'unixfs/fs-node',
        selectedComponent: {
          componentID: 'spacewave.unixfs.viewer',
          typeID: 'unixfs/fs-node',
          name: 'UnixFS Viewer',
          requiresObjectState: false,
          component: () => null,
        },
        objectState: {
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        },
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: 'files',
          objectType: 'unixfs/fs-node',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState({} as IWorldState)}
      />,
    )

    expect(screen.getByTestId('object-viewer-content')).toBeDefined()
    expect(screen.queryByText('Loading the object')).toBeNull()
    expect(screen.queryByText(/^Object not found/)).toBeNull()
  })

  it('renders not found when a world object lookup resolves missing', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        objectKey: 'wizard/git/repo/test',
        objectState: {
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        },
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: 'wizard/git/repo/test',
          objectType: '',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState({} as IWorldState)}
      />,
    )

    expect(screen.getByText(/^Object not found/)).toBeDefined()
    expect(screen.getByText(/wizard\/git\/repo\/test/)).toBeDefined()
    expect(screen.queryByText('Loading the object')).toBeNull()
  })

  it('leaves loading when a world object lookup transitions to missing', () => {
    const objectInfo: ObjectInfo = {
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: 'files',
          objectType: '',
        },
      },
    }
    const worldState = buildWorldState({} as IWorldState)
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        objectKey: 'files',
        objectState: {
          value: null,
          loading: true,
          error: null,
          retry: vi.fn(),
        },
      }),
    )

    const { rerender } = render(
      <ObjectViewer objectInfo={objectInfo} worldState={worldState} />,
    )

    expect(screen.getByText('Loading the object')).toBeDefined()
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        objectKey: 'files',
        objectState: {
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        },
      }),
    )

    rerender(<ObjectViewer objectInfo={objectInfo} worldState={worldState} />)

    expect(screen.getByText(/^Object not found/)).toBeDefined()
    expect(screen.queryByText('Loading the object')).toBeNull()
  })

  describe('the Debug Viewer selected', () => {
    const debugViewer = {
      componentID: 'spacewave.debug.viewer',
      typeID: '*',
      name: 'Debug Viewer',
      component: () => null,
    }

    it('shows a UnixFS path, which has no object to be missing', () => {
      mockUseObjectViewer.mockReturnValue(
        buildViewerResult({
          typeID: 'unixfs/fs-node',
          selectedComponent: debugViewer,
          objectState: {
            value: null,
            loading: false,
            error: null,
            retry: vi.fn(),
          },
        }),
      )
      const objectInfo: ObjectInfo = {
        info: {
          case: 'unixfsObjectInfo',
          value: { unixfsId: 'files', path: '/' },
        },
      }

      render(
        <ObjectViewer
          objectInfo={objectInfo}
          worldState={buildWorldState({} as IWorldState)}
        />,
      )

      expect(screen.getByTestId('object-viewer-content')).toBeDefined()
      expect(screen.queryByText(/^Object not found/)).toBeNull()
    })

    it('waits on the watch of a UnixFS-typed object, then shows it', () => {
      const objectInfo: ObjectInfo = {
        info: {
          case: 'worldObjectInfo',
          value: { objectKey: 'files', objectType: 'unixfs/fs-node' },
        },
      }
      const worldState = buildWorldState({} as IWorldState)
      const viewing = (value: object | null, loading: boolean) =>
        buildViewerResult({
          typeID: 'unixfs/fs-node',
          selectedComponent: debugViewer,
          objectState: { value, loading, error: null, retry: vi.fn() },
        })
      mockUseObjectViewer.mockReturnValue(viewing(null, true))
      const { rerender } = render(
        <ObjectViewer objectInfo={objectInfo} worldState={worldState} />,
      )
      expect(screen.getByText('Loading the object')).toBeDefined()
      expect(screen.queryByText(/^Object not found/)).toBeNull()

      mockUseObjectViewer.mockReturnValue(viewing({ id: 'files' }, false))
      rerender(<ObjectViewer objectInfo={objectInfo} worldState={worldState} />)
      expect(screen.getByTestId('object-viewer-content')).toBeDefined()
    })
  })

  it('does not use standalone mode when the selected viewer disables padding', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'canvas',
        selectedComponent: {
          componentID: 'spacewave.canvas.viewer',
          typeID: 'canvas',
          name: 'Canvas',
          disablePadding: true,
          component: () => null,
        },
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'unixfsObjectInfo',
        value: {
          unixfsId: 'files',
          path: '/',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState(null)}
      />,
    )

    expect(screen.queryByTestId('bottom-bar-root')).toBeNull()
    expect(screen.queryByTestId('viewer-frame')).toBeNull()
    expect(screen.getByTestId('bottom-bar-level')).toBeDefined()
    expect(screen.getByTestId('content-standalone').textContent).toBe('false')
    expect(screen.getByTestId('content-disable-padding').textContent).toBe(
      'true',
    )
  })

  it('uses standalone mode only when explicitly requested', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'unixfs/fs-node',
        selectedComponent: {
          componentID: 'spacewave.unixfs.viewer',
          typeID: 'unixfs/fs-node',
          name: 'UnixFS Viewer',
          component: () => null,
        },
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'unixfsObjectInfo',
        value: {
          unixfsId: 'files',
          path: '/',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState(null)}
        standalone
      />,
    )

    expect(screen.getByTestId('bottom-bar-root')).toBeDefined()
    expect(screen.getByTestId('viewer-frame')).toBeDefined()
    expect(screen.getByTestId('content-standalone').textContent).toBe('true')
  })

  it('wraps primary viewer content in the explicit state namespace', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'unixfs/fs-node',
        selectedComponent: {
          componentID: 'spacewave.unixfs.viewer',
          typeID: 'unixfs/fs-node',
          name: 'UnixFS Viewer',
          component: () => null,
        },
        objectState: {
          value: { id: 'files-object' },
          loading: false,
          error: null,
          retry: vi.fn(),
        },
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: 'files',
          objectType: 'unixfs/fs-node',
        },
      },
    }
    const stateNamespace = ['objectViewer', 'files']

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState({} as IWorldState)}
        stateNamespace={stateNamespace}
      />,
    )

    expect(mockUseObjectViewer).toHaveBeenCalledWith(
      expect.objectContaining({ stateNamespace }),
    )
    expect(screen.getByTestId('state-namespace').dataset.namespace).toBe(
      'objectViewer/files',
    )
    expect(screen.queryByTestId('bottom-bar-root')).toBeNull()
  })

  it('passes object context-menu actions to the bottom bar item', () => {
    const contextMenuItems = [
      {
        type: 'action' as const,
        id: 'open-details',
        label: 'Open Details',
        onSelect: vi.fn(),
      },
    ]
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'unixfs/fs-node',
        selectedComponent: {
          componentID: 'spacewave.unixfs.viewer',
          typeID: 'unixfs/fs-node',
          name: 'UnixFS Viewer',
          component: () => null,
        },
        contextMenuItems,
        contextMenuLabel: 'files actions',
      }),
    )

    const objectInfo: ObjectInfo = {
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: 'files',
          objectType: 'unixfs/fs-node',
        },
      },
    }

    render(
      <ObjectViewer
        objectInfo={objectInfo}
        worldState={buildWorldState({} as IWorldState)}
      />,
    )

    expect(mockBottomBarLevel).toHaveBeenCalledWith(
      expect.objectContaining({
        contextMenuItems,
        contextMenuLabel: 'files actions',
      }),
    )
  })
})

describe('ObjectViewer view states', () => {
  const objectKey = 'world/doc'
  const objectInfo: ObjectInfo = {
    info: {
      case: 'worldObjectInfo',
      value: { objectKey, objectType: '' },
    },
  }
  const world = buildWorldState({} as IWorldState)
  const handle = { id: 'doc' }

  // mockView sets what the viewer hook returns for the next render.
  function mockView(objectState: Record<string, unknown>) {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({ objectKey, typeID: 'type/doc', objectState }),
    )
  }

  // show renders the viewer over objectState with the World and type given.
  function show(
    objectState: Record<string, unknown>,
    typeID: string | undefined = 'type/doc',
    worldState: Resource<IWorldState> = world,
  ) {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({ objectKey, typeID, objectState }),
    )
    return render(
      <ObjectViewer objectInfo={objectInfo} worldState={worldState} />,
    )
  }

  beforeEach(() => {
    cleanup()
    mockUseObjectViewer.mockReset()
  })

  it('loads while the watch has not answered', () => {
    show({ value: null, loading: true, error: null, retry: vi.fn() })
    expect(screen.getByText('Loading the object')).toBeDefined()
    expect(screen.queryByText(/^Object not found/)).toBeNull()
  })

  it('shows the object once it arrives', () => {
    show({ value: handle, loading: false, error: null, retry: vi.fn() })
    expect(screen.getByTestId('object-viewer-content')).toBeDefined()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('shows not found only when the watch answered absent', () => {
    show({ value: null, loading: false, error: null, retry: vi.fn() })
    expect(screen.getByText(/^Object not found/)).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
  })

  it('shows a watch that failed before answering as an error with Retry', () => {
    const retry = vi.fn()
    show(
      {
        value: null,
        loading: false,
        error: new Error('stream reset'),
        last: null,
        retry,
      },
      undefined,
    )
    expect(
      screen.getByText('Could not read world/doc: stream reset'),
    ).toBeDefined()
    expect(screen.queryByText(/^Object not found/)).toBeNull()
    expect(screen.queryByText('Loading the object')).toBeNull()
    screen.getByRole('button', { name: 'Retry' }).click()
    expect(retry).toHaveBeenCalledOnce()
  })

  it('keeps the object on screen when the watch fails after answering', () => {
    const { rerender } = show({
      value: handle,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const content = screen.getByTestId('object-viewer-content')

    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        objectKey,
        typeID: 'type/doc',
        objectState: {
          value: null,
          loading: false,
          error: new Error('stream reset'),
          last: handle,
          retry: vi.fn(),
        },
      }),
    )
    rerender(<ObjectViewer objectInfo={objectInfo} worldState={world} />)

    expect(
      screen.getByText('Could not read world/doc: stream reset'),
    ).toBeDefined()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDefined()
    expect(screen.getByTestId('object-viewer-content')).toBe(content)
  })

  it('shows a World that failed to open with its reason and Retry', () => {
    const retry = vi.fn()
    show({ value: null, loading: false, error: null, retry: vi.fn() }, 'x', {
      value: null,
      loading: false,
      error: new Error('session released'),
      last: null,
      retry,
    })
    expect(
      screen.getByText(
        'Could not open the Space that holds world/doc: session released',
      ),
    ).toBeDefined()
    expect(screen.queryByText('Loading the object')).toBeNull()
    screen.getByRole('button', { name: 'Retry' }).click()
    expect(retry).toHaveBeenCalledOnce()
  })

  it('loads with no Retry while the World waits on its parent', () => {
    show(
      { value: null, loading: false, error: null, retry: vi.fn() },
      'x',
      buildWorldState(null),
    )
    expect(screen.getByText('Loading the object')).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
  })

  it('keeps the content under a notice while the watch reopens', () => {
    const settled = {
      value: handle,
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    const { rerender } = show(settled)
    const content = screen.getByTestId('object-viewer-content')

    // The watch failed after answering and reopens: the content stays.
    mockView({
      ...settled,
      loading: true,
      reopening: new Error('stream reset'),
    })
    rerender(<ObjectViewer objectInfo={objectInfo} worldState={world} />)
    expect(
      screen.getByText('Reconnecting. This view may be out of date.'),
    ).toBeDefined()
    expect(screen.getByTestId('object-viewer-content')).toBe(content)

    // The first value after the reopen clears the notice.
    mockView(settled)
    rerender(<ObjectViewer objectInfo={objectInfo} worldState={world} />)
    expect(
      screen.queryByText('Reconnecting. This view may be out of date.'),
    ).toBeNull()
    expect(screen.getByTestId('object-viewer-content')).toBe(content)
  })

  describe('a viewer that needs no object handle', () => {
    const selectedComponent = {
      componentID: 'alarm.viewer',
      typeID: 'alarm',
      name: 'Alarm',
      requiresObjectState: false,
      component: () => null,
    }

    // showTyped renders the viewer for a component that reads the World
    // itself, over objectState and the type the watch reported.
    // mockTyped sets what the viewer hook returns for the next render.
    function mockTyped(
      objectState: Record<string, unknown>,
      typeID: string | undefined,
    ) {
      mockUseObjectViewer.mockReturnValue(
        buildViewerResult({
          objectKey,
          typeID,
          objectState,
          selectedComponent,
        }),
      )
    }

    function showTyped(
      objectState: Record<string, unknown>,
      typeID: string | undefined,
    ) {
      mockTyped(objectState, typeID)
      return render(<ObjectViewer objectInfo={objectInfo} worldState={world} />)
    }

    it('shows a watch that failed before answering as an error', () => {
      const retry = vi.fn()
      showTyped(
        {
          value: null,
          loading: false,
          error: new Error('stream reset'),
          last: null,
          retry,
        },
        undefined,
      )
      expect(
        screen.getByText('Could not read world/doc: stream reset'),
      ).toBeDefined()
      expect(screen.queryByText('Loading the object')).toBeNull()
      screen.getByRole('button', { name: 'Retry' }).click()
      expect(retry).toHaveBeenCalledOnce()
    })

    it('keeps its content under the failure after answering', () => {
      const { rerender } = showTyped(
        { value: null, loading: false, error: null, retry: vi.fn() },
        'alarm',
      )
      const content = screen.getByTestId('object-viewer-content')
      mockTyped(
        {
          value: null,
          loading: false,
          error: new Error('stream reset'),
          last: null,
          retry: vi.fn(),
        },
        'alarm',
      )
      rerender(<ObjectViewer objectInfo={objectInfo} worldState={world} />)
      expect(
        screen.getByText('Could not read world/doc: stream reset'),
      ).toBeDefined()
      expect(screen.getByTestId('object-viewer-content')).toBe(content)
    })

    it('shows not found when the watch answered absent', () => {
      showTyped(
        { value: null, loading: false, error: null, retry: vi.fn() },
        undefined,
      )
      expect(screen.getByText(/^Object not found/)).toBeDefined()
      expect(screen.queryByTestId('object-viewer-content')).toBeNull()
    })
  })
})
