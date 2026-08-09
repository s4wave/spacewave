import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { DisplayContainer } from './DisplayContainer.js'

interface TestResource<T = unknown> {
  value: T
  loading: boolean
  error: Error | null
  retry: () => void
}

interface ObjectViewerArgs {
  objectInfo?: {
    info?: {
      case?: string
      value?: {
        objectKey?: string
        objectType?: string
      }
    }
  }
  preferredComponentID?: string
  stateNamespace?: string[]
}

interface ObjectViewerContentProps {
  objectInfo?: ObjectViewerArgs['objectInfo']
  component?: { componentID?: string }
  standalone?: boolean
}

interface HistoryRouterProps {
  path?: string
  children?: ReactNode
}

const h = vi.hoisted(() => ({
  resourceCall: 0,
  watchCall: 0,
  resources: [] as TestResource[],
  watches: [] as unknown[],
  rootResource: undefined as TestResource | undefined,
  sessionList: undefined as TestResource | undefined,
  useObjectViewer: vi.fn(),
  objectViewerContent: vi.fn(),
  historyRouter: vi.fn<(props: HistoryRouterProps) => void>(),
}))

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: () => h.watches[h.watchCall++] ?? null,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: () => h.resources[h.resourceCall++],
  useResourceValue: (resource?: TestResource) => resource?.value,
}))

vi.mock('@s4wave/app/hooks/useSessionList.js', () => ({
  useSessionList: () => h.sessionList,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => h.rootResource,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => {
  function Provider({ children }: { children?: ReactNode }) {
    return <>{children}</>
  }
  return {
    SessionContext: { Provider },
    SessionIndexContext: { Provider },
    SharedObjectBodyContext: { Provider },
    SharedObjectContext: { Provider },
    SpaceContentsContext: { Provider },
    SpaceContext: { Provider },
  }
})

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => {
  function Provider({ children }: { children?: ReactNode }) {
    return <>{children}</>
  }
  return {
    SpaceContainerContext: { Provider },
  }
})

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

vi.mock('@s4wave/web/object/ObjectViewerContent.js', () => ({
  ObjectViewerContent: (props: ObjectViewerContentProps) => {
    h.objectViewerContent(props)
    return (
      <div data-testid="object-viewer-content">
        <span data-testid="content-object-key">
          {props.objectInfo?.info?.value?.objectKey ?? ''}
        </span>
        <span data-testid="content-component">
          {props.component?.componentID ?? ''}
        </span>
        <span data-testid="content-standalone">
          {props.standalone ? 'true' : 'false'}
        </span>
      </div>
    )
  },
}))

vi.mock('@s4wave/web/object/ObjectViewerContext.js', () => ({
  ObjectViewerProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/object/ObjectViewerLoadingState.js', () => ({
  ObjectViewerLoadingState: () => <div>Loading object</div>,
}))

vi.mock('@s4wave/web/object/ObjectViewerNotFoundState.js', () => ({
  ObjectViewerNotFoundState: ({ objectKey }: { objectKey: string }) => (
    <div>
      <span>Object not found</span>
      <span data-testid="not-found-object-key">{objectKey}</span>
    </div>
  ),
}))

vi.mock('@s4wave/web/object/useObjectViewer.js', () => ({
  useObjectViewer: h.useObjectViewer,
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  HistoryRouter: (props: HistoryRouterProps) => {
    h.historyRouter(props)
    return <div data-testid="history-router">{props.children}</div>
  },
}))

vi.mock('@s4wave/web/state', () => ({
  StateNamespaceProvider: ({ children }: { children?: ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/ui/loading/LoadingCard.js', () => ({
  LoadingCard: ({
    view,
  }: {
    view: { title: string; detail?: string; error?: string }
  }) => (
    <div>
      <span>{view.title}</span>
      <span>{view.detail ?? view.error ?? ''}</span>
    </div>
  ),
}))

function readyResource<T>(value: T): TestResource<T> {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

function buildViewerResult(overrides: Record<string, unknown> = {}) {
  const component = {
    componentID: 'viewer.markdown',
    typeID: 'spacewave/document',
    name: 'Markdown',
    requiresObjectState: true,
    component: () => null,
  }
  return {
    objectState: { value: { id: 'object-state' }, loading: false, error: null },
    typeID: 'spacewave/document',
    visibleComponents: [component],
    selectedComponent: component,
    missingComponentID: undefined,
    onSelectComponent: vi.fn(),
    viewerContextValue: {
      visibleComponents: [component],
      selectedComponent: component,
      onSelectComponent: vi.fn(),
    },
    buttonRender: vi.fn(),
    overlayContent: undefined,
    buttonKeyValue: 'button',
    overlayKeyValue: 'overlay',
    contextMenuItems: undefined,
    contextMenuKey: 'context',
    contextMenuLabel: 'Object actions',
    ...overrides,
  }
}

function setupDisplay(options: {
  path: string
  component?: string
  objects?: Array<{ objectKey: string; objectType: string }>
}) {
  h.resourceCall = 0
  h.watchCall = 0
  h.rootResource = readyResource({})
  h.sessionList = readyResource({ sessions: [{ sessionIndex: 3 }] })
  h.resources = [
    readyResource({}),
    readyResource({}),
    readyResource({}),
    readyResource({}),
    readyResource({}),
    readyResource({}),
  ]
  h.watches = [
    {
      spacesList: [
        {
          entry: {
            ref: {
              providerResourceRef: { id: 'space/git' },
            },
          },
        },
      ],
    },
    {
      ready: true,
      worldContents: {
        objects: options.objects ?? [
          {
            objectKey: 'docs/hello',
            objectType: 'spacewave/document',
          },
        ],
      },
    },
  ]

  const url = new URL('https://app.test/display')
  url.searchParams.set('path', options.path)
  if (options.component) url.searchParams.set('component', options.component)
  window.history.replaceState({}, '', url.pathname + url.search)
}

describe('DisplayContainer', () => {
  beforeEach(() => {
    cleanup()
    h.useObjectViewer.mockReset()
    h.objectViewerContent.mockReset()
    h.historyRouter.mockReset()
    setupDisplay({ path: 'docs/hello/-/child file.md' })
    h.useObjectViewer.mockReturnValue(buildViewerResult())
  })

  it('resolves query path and component into the standalone world object viewer', () => {
    setupDisplay({
      path: 'docs/hello/-/child file.md',
      component: 'viewer.markdown',
    })

    render(<DisplayContainer />)

    const viewerArgs = h.useObjectViewer.mock.calls[0]?.[0] as ObjectViewerArgs
    expect(viewerArgs.objectInfo?.info?.case).toBe('worldObjectInfo')
    expect(viewerArgs.objectInfo?.info?.value?.objectKey).toBe('docs/hello')
    expect(viewerArgs.objectInfo?.info?.value?.objectType).toBe(
      'spacewave/document',
    )
    expect(viewerArgs.preferredComponentID).toBe('viewer.markdown')
    expect(viewerArgs.stateNamespace).toEqual([
      'display',
      'space/git',
      'docs/hello',
      'viewer.markdown',
    ])
    expect(h.historyRouter.mock.calls[0]?.[0]?.path).toBe('/child file.md')
    expect(screen.getByTestId('object-viewer-content')).toBeDefined()
    expect(screen.getByTestId('content-object-key').textContent).toBe(
      'docs/hello',
    )
    expect(screen.getByTestId('content-standalone').textContent).toBe('true')
  })

  it('renders the not-found state when the display path does not resolve to a world object', () => {
    setupDisplay({
      path: 'missing/object',
      component: 'viewer.markdown',
      objects: [{ objectKey: 'docs/hello', objectType: 'spacewave/document' }],
    })

    render(<DisplayContainer />)

    expect(screen.getByText('Object not found')).toBeDefined()
    expect(screen.getByTestId('not-found-object-key').textContent).toBe(
      'missing/object',
    )
    expect(h.useObjectViewer).not.toHaveBeenCalled()
    expect(screen.queryByTestId('object-viewer-content')).toBeNull()
  })

  it('fails loud for an unmatched component id instead of rendering viewer fallback content', () => {
    setupDisplay({
      path: 'docs/hello',
      component: 'viewer.missing',
    })
    h.useObjectViewer.mockReturnValue(
      buildViewerResult({
        missingComponentID: 'viewer.missing',
        selectedComponent: {
          componentID: 'viewer.fallback',
          typeID: 'spacewave/document',
          name: 'Fallback',
          requiresObjectState: true,
          component: () => null,
        },
      }),
    )

    render(<DisplayContainer />)

    expect(screen.getByText('Display component not found')).toBeDefined()
    expect(screen.getByText(/viewer\.missing/)).toBeDefined()
    expect(screen.getByText(/docs\/hello/)).toBeDefined()
    expect(screen.queryByTestId('object-viewer-content')).toBeNull()
    expect(h.objectViewerContent).not.toHaveBeenCalled()
  })
})
