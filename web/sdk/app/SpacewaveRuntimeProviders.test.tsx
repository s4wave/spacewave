import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => {
  const state: {
    bldrContext: unknown
    rootResource: unknown
  } = {
    bldrContext: null,
    rootResource: {
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    },
  }
  const resourceClientInstances: unknown[] = []
  return {
    getBldrContext: () => state.bldrContext,
    setBldrContext: (value: unknown) => {
      state.bldrContext = value
    },
    getRootResource: () => state.rootResource,
    setRootResource: (value: unknown) => {
      state.rootResource = value
    },
    resourceClientDispose: vi.fn(),
    resourceClientInstances,
  }
})

vi.mock('@aptre/bldr-react', () => ({
  useBldrContext: () => mocks.getBldrContext(),
}))

vi.mock('@aptre/bldr-sdk/hooks/ResourcesContext.js', () => ({
  ResourcesProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@aptre/bldr-sdk/resource/index.js', () => ({
  Client: class ResourceClient {
    constructor() {
      mocks.resourceClientInstances.push(this)
    }
    dispose() {
      mocks.resourceClientDispose()
    }
  },
}))

vi.mock('@aptre/bldr-sdk/resource/resource_srpc.pb.js', () => ({
  ResourceServiceClient: class ResourceServiceClient {},
  ResourceServiceServiceName: 'resource.ResourceService',
}))

vi.mock('starpc', () => ({
  Client: class SRPCClient {},
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResourceWithClient: () => mocks.getRootResource(),
}))

vi.mock('@s4wave/web/hooks/useViewerRegistry.js', () => ({
  ViewerRegistryProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/configtype/ConfigTypeRegistryContext.js', () => ({
  ConfigTypeRegistryProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/ui/FloatingWindow.js', () => ({
  FloatingWindowManagerProvider: ({
    children,
  }: {
    children: React.ReactNode
  }) => <>{children}</>,
}))

vi.mock('@s4wave/web/devtools/index.js', () => ({
  ResourceDevToolsProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  StateDevToolsProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  RootContext: {
    Provider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  },
}))

vi.mock('@s4wave/web/state/index.js', () => ({
  StateNamespaceProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/command/index.js', () => ({
  CommandProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/ui/ErrorState.js', () => ({
  ErrorState: ({ title, message }: { title: string; message?: string }) => (
    <div>
      <h1>{title}</h1>
      <p>{message}</p>
    </div>
  ),
}))

vi.mock('@s4wave/web/ui/loading/LoadingCard.js', () => ({
  LoadingCard: ({ view }: { view: { title: string; detail?: string } }) => (
    <div>
      <h1>{view.title}</h1>
      <p>{view.detail}</p>
    </div>
  ),
}))

import { SpacewaveRuntimeProviders } from './SpacewaveRuntimeProviders.js'

describe('SpacewaveRuntimeProviders', () => {
  beforeEach(() => {
    mocks.setBldrContext(null)
    mocks.setRootResource({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
    mocks.resourceClientDispose.mockClear()
    mocks.resourceClientInstances.length = 0
  })

  afterEach(() => {
    cleanup()
  })

  it('renders the runtime loading state before the Resource client is ready', () => {
    render(
      <SpacewaveRuntimeProviders staticViewers={[]} staticConfigTypes={[]}>
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(screen.getByText('Initializing')).toBeDefined()
    expect(screen.getByText('Preparing the Spacewave runtime.')).toBeDefined()
    expect(screen.queryByText('ready')).toBeNull()
  })

  it('renders the root error state', () => {
    mocks.setRootResource({
      value: null,
      loading: false,
      error: new Error('root failed'),
      retry: vi.fn(),
    })

    render(
      <SpacewaveRuntimeProviders staticViewers={[]} staticConfigTypes={[]}>
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(screen.getByText('Failed to load')).toBeDefined()
    expect(screen.getByText('root failed')).toBeDefined()
  })

  it('passes the explicit runtime context to render-function children', async () => {
    mocks.setRootResource({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    mocks.setBldrContext({
      webView: {
        getUuid: () => 'web-view-1',
      },
      webDocument: {
        buildWebViewHostOpenStream: () => ({}),
      },
    })

    render(
      <SpacewaveRuntimeProviders staticViewers={[]} staticConfigTypes={[]}>
        {({ rootResource, resourceClient }) => (
          <div>
            {rootResource.value && resourceClient ? 'ready' : 'missing'}
          </div>
        )}
      </SpacewaveRuntimeProviders>,
    )

    expect(await screen.findByText('ready')).toBeDefined()
  })
})
