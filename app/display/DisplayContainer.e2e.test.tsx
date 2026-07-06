import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import { DisplayContainer } from './DisplayContainer.js'

interface TestResource<T = unknown> {
  value?: T
  loading: boolean
  error?: Error | null
  retry: () => void
}

interface ObjectViewerArgs {
  preferredComponentID?: string
  stateNamespace?: string[]
  objectInfo?: {
    info?: {
      case?: string
      value?: {
        objectKey?: string
        objectType?: string
      }
    }
  }
}

interface ObjectViewerContentProps {
  objectInfo?: {
    info?: {
      value?: {
        objectKey?: string
        objectType?: string
      }
    }
  }
  component?: { componentID?: string }
  standalone?: boolean
}

interface HistoryRouterProps {
  path: string
  onNavigate: (to: { path: string; replace?: boolean }) => void
  children?: ReactNode
}

interface SpaceContainerProviderProps {
  children?: ReactNode
  objectKey?: string
  objectPath?: string
  navigateToObjects: (objectKeys: string[]) => void
  buildObjectUrls: (objectKeys: string[]) => string[]
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
  historyRouter: vi.fn(),
  spaceContainerProvider: vi.fn(),
}))

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: () => {
    const response = h.watches.length
      ? (h.watches[h.watchCall % h.watches.length] ?? null)
      : null
    h.watchCall += 1
    return response
  },
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: () => {
    const resource = h.resources.length
      ? h.resources[h.resourceCall % h.resources.length]
      : undefined
    h.resourceCall += 1
    return resource
  },
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
  function Provider(props: SpaceContainerProviderProps) {
    h.spaceContainerProvider(props)
    const retargetUrl = props.buildObjectUrls(['dashboards/status'])[0] ?? ''
    return (
      <div data-testid="display-space-container">
        <p>Space object: {props.objectKey ?? ''}</p>
        <p>Space object path: {props.objectPath ?? ''}</p>
        <p>Retarget URL: {retargetUrl}</p>
        <button
          type="button"
          onClick={() => props.navigateToObjects(['dashboards/status'])}
        >
          Retarget status dashboard
        </button>
        {props.children}
      </div>
    )
  }
  return {
    SpaceContainerContext: { Provider },
  }
})

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: ({ children }: { children?: ReactNode }) => (
    <div data-testid="display-bottom-bar-level">{children}</div>
  ),
}))

vi.mock('@s4wave/web/frame/bottom-bar-root.js', () => ({
  BottomBarRoot: ({ children }: { children?: ReactNode }) => (
    <div data-testid="display-bottom-bar-root">{children}</div>
  ),
}))

vi.mock('@s4wave/web/frame/ViewerFrame.js', () => ({
  ViewerFrame: ({ children }: { children?: ReactNode }) => (
    <main data-testid="display-viewer-frame">{children}</main>
  ),
}))

vi.mock('@s4wave/web/object/ObjectViewerContent.js', () => ({
  ObjectViewerContent: (props: ObjectViewerContentProps) => {
    h.objectViewerContent(props)
    return (
      <section
        aria-label="Browser display object"
        data-testid="object-viewer-content"
      >
        <p>Object key: {props.objectInfo?.info?.value?.objectKey ?? ''}</p>
        <p>Object type: {props.objectInfo?.info?.value?.objectType ?? ''}</p>
        <p>Component: {props.component?.componentID ?? ''}</p>
        <p>Standalone: {props.standalone ? 'true' : 'false'}</p>
      </section>
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
      <span>{objectKey}</span>
    </div>
  ),
}))

vi.mock('@s4wave/web/object/useObjectViewer.js', () => ({
  useObjectViewer: h.useObjectViewer,
}))

vi.mock('@s4wave/web/router/HistoryRouter.js', () => ({
  HistoryRouter: (props: HistoryRouterProps) => {
    h.historyRouter(props)
    return (
      <div data-testid="history-router">
        <p>History path: {props.path}</p>
        <button
          type="button"
          onClick={() => props.onNavigate({ path: '/nested note.md' })}
        >
          Navigate nested note
        </button>
        {props.children}
      </div>
    )
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

function buildViewerResult(
  options: {
    componentID?: string
    missingComponentID?: string
  } = {},
) {
  const component = {
    componentID: options.componentID ?? 'viewer.markdown',
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
    missingComponentID: options.missingComponentID,
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
  }
}

function viewerForRequestedComponent(args: ObjectViewerArgs) {
  const requestedComponent = args.preferredComponentID ?? 'viewer.markdown'
  if (requestedComponent === 'viewer.missing') {
    return buildViewerResult({
      componentID: 'viewer.fallback',
      missingComponentID: requestedComponent,
    })
  }
  return buildViewerResult({ componentID: requestedComponent })
}

function setupDisplayRoute(options: {
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
    readyResource({ id: 'world-state' }),
    readyResource({ id: 'space-contents' }),
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
  h.useObjectViewer.mockImplementation(viewerForRequestedComponent)

  const url = new URL('/display', window.location.origin)
  url.searchParams.set('path', options.path)
  if (options.component) url.searchParams.set('component', options.component)
  window.history.replaceState({}, '', `${url.pathname}${url.search}`)
}

function encodedDisplayRoutePath(path: string): string {
  return `/display/${path.split('/').map(encodeURIComponent).join('/')}`
}

function setupDisplayRoutePath(options: {
  path: string
  component?: string
  objects?: Array<{ objectKey: string; objectType: string }>
}) {
  setupDisplayRoute(options)

  const url = new URL(
    encodedDisplayRoutePath(options.path),
    window.location.origin,
  )
  if (options.component) url.searchParams.set('component', options.component)
  window.history.replaceState({}, '', `${url.pathname}${url.search}`)
}

function latestObjectViewerArgs(): ObjectViewerArgs {
  const call =
    h.useObjectViewer.mock.calls[h.useObjectViewer.mock.calls.length - 1]
  if (!call) throw new Error('ObjectViewer was not called')
  return call[0] as ObjectViewerArgs
}

function DisplayBrowserSurface() {
  return (
    <div className="bg-background-primary text-foreground fixed inset-0">
      <DisplayContainer />
    </div>
  )
}

async function capture(name: string) {
  return page.screenshot({ path: `__screenshots__/display-mode/${name}.png` })
}

describe('display route browser render', () => {
  beforeEach(async () => {
    await cleanup()
    h.useObjectViewer.mockReset()
    h.objectViewerContent.mockReset()
    h.historyRouter.mockReset()
    h.spaceContainerProvider.mockReset()
  })

  it('renders a decoded object subpath and requested component through HistoryRouter and ObjectViewerContent', async () => {
    setupDisplayRoute({
      path: 'docs/hello/-/child file.md',
      component: 'viewer.markdown',
    })

    await render(<DisplayBrowserSurface />)

    expect(window.location.pathname).toBe('/display')
    expect(new URLSearchParams(window.location.search).get('path')).toBe(
      'docs/hello/-/child file.md',
    )
    await expect
      .element(page.getByText('History path: /child file.md'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Object key: docs/hello'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Object type: spacewave/document'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Component: viewer.markdown'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Standalone: true')).toBeInTheDocument()

    await capture('object-route')

    await page.getByRole('button', { name: 'Navigate nested note' }).click()

    await expect
      .element(page.getByText('History path: /nested note.md'))
      .toBeInTheDocument()
    const nextSearch = new URLSearchParams(window.location.search)
    expect(nextSearch.get('path')).toBe('docs/hello/-/nested note.md')
    expect(nextSearch.get('component')).toBe('viewer.markdown')
    await cleanup()
  })

  it('resolves a route-path display target when no path query is present', async () => {
    const routePath = 'docs/hello/-/route child.md'
    setupDisplayRoutePath({
      path: routePath,
      component: 'viewer.diagram',
    })

    await render(<DisplayBrowserSurface />)

    const search = new URLSearchParams(window.location.search)
    expect(window.location.pathname).toBe(encodedDisplayRoutePath(routePath))
    expect(search.has('path')).toBe(false)
    expect(search.get('component')).toBe('viewer.diagram')
    await expect
      .element(page.getByText('History path: /route child.md'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Object key: docs/hello'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Object type: spacewave/document'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Component: viewer.diagram'))
      .toBeInTheDocument()

    const viewerArgs = latestObjectViewerArgs()
    expect(viewerArgs.preferredComponentID).toBe('viewer.diagram')
    expect(viewerArgs.stateNamespace).toEqual([
      'display',
      'space/git',
      'docs/hello',
      'viewer.diagram',
    ])
    await cleanup()
  })

  it('retargets a mounted display to a different object through display navigation', async () => {
    setupDisplayRoute({
      path: 'docs/hello/-/child file.md',
      component: 'viewer.markdown',
      objects: [
        { objectKey: 'docs/hello', objectType: 'spacewave/document' },
        { objectKey: 'dashboards/status', objectType: 'spacewave/dashboard' },
      ],
    })

    await render(<DisplayBrowserSurface />)

    await expect
      .element(page.getByText('Object key: docs/hello'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Object type: spacewave/document'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('History path: /child file.md'))
      .toBeInTheDocument()

    const initialArgs = latestObjectViewerArgs()
    expect(initialArgs.objectInfo?.info?.case).toBe('worldObjectInfo')
    expect(initialArgs.objectInfo?.info?.value?.objectKey).toBe('docs/hello')
    expect(initialArgs.objectInfo?.info?.value?.objectType).toBe(
      'spacewave/document',
    )
    expect(initialArgs.stateNamespace).toEqual([
      'display',
      'space/git',
      'docs/hello',
      'viewer.markdown',
    ])

    await page
      .getByRole('button', { name: 'Retarget status dashboard' })
      .click()

    await expect
      .element(page.getByText('Object key: dashboards/status'))
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Object type: spacewave/dashboard'))
      .toBeInTheDocument()
    await expect.element(page.getByText('History path: /')).toBeInTheDocument()

    expect(window.location.pathname).toBe('/display')
    const nextSearch = new URLSearchParams(window.location.search)
    expect(nextSearch.get('path')).toBe('dashboards/status')
    expect(nextSearch.get('component')).toBe('viewer.markdown')

    const retargetedArgs = latestObjectViewerArgs()
    expect(retargetedArgs.objectInfo?.info?.case).toBe('worldObjectInfo')
    expect(retargetedArgs.objectInfo?.info?.value?.objectKey).toBe(
      'dashboards/status',
    )
    expect(retargetedArgs.objectInfo?.info?.value?.objectType).toBe(
      'spacewave/dashboard',
    )
    expect(retargetedArgs.stateNamespace).toEqual([
      'display',
      'space/git',
      'dashboards/status',
      'viewer.markdown',
    ])

    await cleanup()
  })

  it('renders the explicit browser not-found state for an unmatched display component id', async () => {
    setupDisplayRoute({
      path: 'docs/hello',
      component: 'viewer.missing',
    })

    await render(<DisplayBrowserSurface />)

    await expect
      .element(page.getByText('Display component not found'))
      .toBeInTheDocument()
    await expect
      .element(
        page.getByText(
          'viewer.missing is not installed for docs/hello (spacewave/document).',
        ),
      )
      .toBeInTheDocument()
    await expect
      .element(page.getByText('Component: viewer.fallback'))
      .not.toBeInTheDocument()
    expect(h.objectViewerContent).not.toHaveBeenCalled()

    await capture('component-not-found')
    await cleanup()
  })
})
