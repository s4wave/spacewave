import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => {
  const state: {
    bldrContext: unknown
    rootResource: unknown
    lastRootClient: unknown
    lastResourcesClient: unknown
  } = {
    bldrContext: null,
    rootResource: {
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    },
    lastRootClient: 'never-set',
    lastResourcesClient: 'never-set',
  }
  const resourceClientInstances: unknown[] = []
  const rootAtoms: unknown[] = []
  return {
    getBldrContext: () => state.bldrContext,
    setBldrContext: (value: unknown) => {
      state.bldrContext = value
    },
    getRootResource: () => state.rootResource,
    setRootResource: (value: unknown) => {
      state.rootResource = value
    },
    getLastRootClient: () => state.lastRootClient,
    setLastRootClient: (value: unknown) => {
      state.lastRootClient = value
    },
    getLastResourcesClient: () => state.lastResourcesClient,
    setLastResourcesClient: (value: unknown) => {
      state.lastResourcesClient = value
    },
    rootAtoms,
    resourceClientDispose: vi.fn(),
    resourceClientInstances,
  }
})

vi.mock('@aptre/bldr-react', () => ({
  useBldrContext: () => mocks.getBldrContext(),
}))

vi.mock('@aptre/bldr-sdk/hooks/ResourcesContext.js', () => ({
  ResourcesProvider: ({
    client,
    children,
  }: {
    client: unknown
    children: React.ReactNode
  }) => {
    mocks.setLastResourcesClient(client)
    return <>{children}</>
  },
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
  useRootResourceWithClient: (client: unknown) => {
    mocks.setLastRootClient(client)
    return mocks.getRootResource()
  },
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
  // Fresh object per call mirrors the real atom() factory identity behavior.
  atom: () => ({}),
  StateNamespaceProvider: ({
    rootAtom,
    children,
  }: {
    rootAtom?: unknown
    children: React.ReactNode
  }) => {
    mocks.rootAtoms.push(rootAtom)
    return <>{children}</>
  },
}))

vi.mock('@s4wave/web/command/index.js', () => ({
  CommandProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}))

vi.mock('@s4wave/web/ui/ErrorState.js', () => ({
  ErrorState: ({
    title,
    message,
    onRetry,
  }: {
    title: string
    message?: string
    onRetry?: () => void
  }) => (
    <div>
      <h1>{title}</h1>
      <p>{message}</p>
      {onRetry && (
        <button type="button" onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  ),
}))

import type { Client as ResourceClient } from '@aptre/bldr-sdk/resource/index.js'
import type { StateType } from '@s4wave/web/state/index.js'

import { SpacewaveRuntimeProviders } from './SpacewaveRuntimeProviders.js'

// Real atom factory from the unmocked state module for rootAtom instances.
const realState = await vi.importActual<
  typeof import('@s4wave/web/state/index.js')
>('@s4wave/web/state/index.js')

describe('SpacewaveRuntimeProviders', () => {
  beforeEach(() => {
    mocks.setBldrContext(null)
    mocks.setRootResource({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
    mocks.setLastRootClient('never-set')
    mocks.setLastResourcesClient('never-set')
    mocks.rootAtoms.length = 0
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

    expect(
      screen.getByRole('heading', { name: 'Starting Spacewave' }),
    ).toBeDefined()
    expect(screen.getByText('Preparing the Spacewave runtime.')).toBeDefined()
    expect(screen.queryByText('ready')).toBeNull()
  })

  it('shows the root connection error and offers its retry', () => {
    const retry = vi.fn()
    mocks.setRootResource({
      value: null,
      loading: false,
      error: new Error(
        'Failed to initialize client connection: local transport unavailable',
      ),
      retry,
    })

    render(
      <SpacewaveRuntimeProviders staticViewers={[]} staticConfigTypes={[]}>
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(screen.getByText('Failed to load')).toBeDefined()
    expect(
      screen.getByText(
        'Failed to initialize client connection: local transport unavailable',
      ),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
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

    const { unmount } = render(
      <SpacewaveRuntimeProviders staticViewers={[]} staticConfigTypes={[]}>
        {({ rootResource, resourceClient }) => (
          <div>
            {rootResource.value && resourceClient ? 'ready' : 'missing'}
          </div>
        )}
      </SpacewaveRuntimeProviders>,
    )

    expect(await screen.findByText('ready')).toBeDefined()

    // Default mode owns the client it created: unmount disposes it.
    unmount()
    expect(mocks.resourceClientDispose).toHaveBeenCalled()
  })

  it('renders with a supplied client and no Bldr context', () => {
    mocks.setRootResource({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const fakeClient = { dispose: vi.fn() } as unknown as ResourceClient

    render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={fakeClient}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(screen.getByText('ready')).toBeDefined()
    expect(mocks.getLastRootClient()).toBe(fakeClient)
    expect(mocks.getLastResourcesClient()).toBe(fakeClient)
    // The supplied client is passed through, never constructed here.
    expect(mocks.resourceClientInstances).toHaveLength(0)
  })

  it('does not fall back to the Bldr transport when the supplied client is null', () => {
    mocks.setBldrContext({
      webView: {
        getUuid: () => 'web-view-1',
      },
      webDocument: {
        buildWebViewHostOpenStream: () => ({}),
      },
    })

    render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={null}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(
      screen.getByRole('heading', { name: 'Starting Spacewave' }),
    ).toBeDefined()
    expect(screen.queryByText('ready')).toBeNull()
    expect(mocks.getLastRootClient()).toBeNull()
    expect(mocks.getLastResourcesClient()).toBeNull()
    expect(mocks.resourceClientInstances).toHaveLength(0)
    cleanup()

    // A present-but-undefined prop is explicit pending too, not omitted:
    // presence dispatch, not nullness, selects the connection mode.
    const { unmount: unmountPending } = render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={undefined}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(
      screen.getByRole('heading', { name: 'Starting Spacewave' }),
    ).toBeDefined()
    expect(mocks.getLastRootClient()).toBeNull()
    expect(mocks.resourceClientInstances).toHaveLength(0)
    unmountPending()
  })

  it('propagates a replaced client to the root resource and providers', () => {
    mocks.setRootResource({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const clientA = { dispose: vi.fn() } as unknown as ResourceClient
    const clientB = { dispose: vi.fn() } as unknown as ResourceClient

    const { rerender } = render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={clientA}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )
    expect(mocks.getLastRootClient()).toBe(clientA)
    expect(mocks.getLastResourcesClient()).toBe(clientA)

    rerender(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={clientB}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )
    expect(mocks.getLastRootClient()).toBe(clientB)
    expect(mocks.getLastResourcesClient()).toBe(clientB)
    // Caller-owned clients are never disposed by the providers.
    expect(clientA.dispose).not.toHaveBeenCalled()
    expect(clientB.dispose).not.toHaveBeenCalled()
  })

  it('does not dispose a caller-owned client on unmount', () => {
    mocks.setRootResource({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const fakeClient = { dispose: vi.fn() } as unknown as ResourceClient

    const { unmount } = render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={fakeClient}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    unmount()
    expect(fakeClient.dispose).not.toHaveBeenCalled()
    expect(mocks.resourceClientDispose).not.toHaveBeenCalled()
  })

  it('scopes the root atom per explicit provider instance', () => {
    mocks.setRootResource({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    const clientA = { dispose: vi.fn() } as unknown as ResourceClient
    const clientB = { dispose: vi.fn() } as unknown as ResourceClient

    render(
      <>
        <SpacewaveRuntimeProviders
          staticViewers={[]}
          staticConfigTypes={[]}
          resourceClient={clientA}
        >
          <div>a</div>
        </SpacewaveRuntimeProviders>
        <SpacewaveRuntimeProviders
          staticViewers={[]}
          staticConfigTypes={[]}
          resourceClient={clientB}
        >
          <div>b</div>
        </SpacewaveRuntimeProviders>
      </>,
    )

    const unique = [...new Set(mocks.rootAtoms)]
    expect(unique.length).toBe(2)
    expect(unique.every((atom) => atom != null)).toBe(true)
  })

  it('honors a caller-supplied root atom in explicit mode', () => {
    mocks.setRootResource({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const supplied = realState.atom<StateType>({})

    render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        resourceClient={{ dispose: vi.fn() } as unknown as ResourceClient}
        rootAtom={supplied}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(mocks.rootAtoms).toContain(supplied)
  })

  it('keeps the default mode root atom undefined', () => {
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
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(mocks.rootAtoms.length).toBeGreaterThan(0)
    expect(mocks.rootAtoms.every((atom) => atom === undefined)).toBe(true)
  })

  it('honors a caller-supplied root atom in default mode', () => {
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
    const supplied = realState.atom<StateType>({})

    render(
      <SpacewaveRuntimeProviders
        staticViewers={[]}
        staticConfigTypes={[]}
        rootAtom={supplied}
      >
        <div>ready</div>
      </SpacewaveRuntimeProviders>,
    )

    expect(mocks.rootAtoms).toContain(supplied)
  })
})
