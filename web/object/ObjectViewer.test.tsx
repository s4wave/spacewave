import { describe, it, expect, vi, beforeEach } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { ObjectViewer } from './ObjectViewer.js'
import type { ObjectInfo } from './object.pb.js'

const mockUseObjectViewer = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: ({ children }: { children?: ReactNode }) => (
    <div data-testid="bottom-bar-level">{children}</div>
  ),
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
  StateNamespaceProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
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
    onSelectComponent: vi.fn(),
    viewerContextValue: {
      visibleComponents: [],
      selectedComponent: undefined,
      onSelectComponent: vi.fn(),
    },
    buttonRender: vi.fn(),
    overlayContent: undefined,
    buttonKeyValue: 'button',
    overlayKeyValue: 'overlay',
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

    expect(screen.getByText('Loading object')).toBeDefined()
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
    expect(screen.queryByText('Loading object')).toBeNull()
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

    expect(screen.getByText('Object not found')).toBeDefined()
    expect(screen.getByText(/wizard\/git\/repo\/test/)).toBeDefined()
    expect(screen.queryByText('Loading object')).toBeNull()
  })

  it('does not use standalone mode when the selected viewer disables padding', () => {
    mockUseObjectViewer.mockReturnValue(
      buildViewerResult({
        typeID: 'canvas',
        selectedComponent: {
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
})
