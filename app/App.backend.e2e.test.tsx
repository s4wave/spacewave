/**
 * E2E tests for the spacewave Resources SDK with the real spacewave-core plugin backend.
 *
 * These tests verify the Resources SDK works correctly with the production Go backend
 * providing RootResourceService, SessionResourceService, SpaceResourceService, etc.
 *
 * To run these tests:
 * 1. Run: go test -v ./core/e2e/browser/...
 *    This starts the Go backend and runs vitest browser tests.
 */
import { describe, it, expect, beforeAll, afterAll } from 'vitest'

import {
  E2ETestClient,
  createE2EClient,
  getTestServerPort,
} from '@s4wave/web/test/e2e-client.js'
import { ResourceServiceClient } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import {
  Client as ResourceClient,
  type ResourceReleasedEvent,
  type ResourceReleaseReason,
} from '@aptre/bldr-sdk/resource/index.js'
import { Root } from '@s4wave/sdk/root/root.js'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { AsyncDisposableStack } from '@aptre/bldr-sdk/defer.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { Counter, CounterTypeID } from '@s4wave/sdk/plugin/testdata/counter.js'
import {
  ViewerSurface,
  type ViewerRegistration,
} from '@s4wave/sdk/viewer/registry/registry.pb.js'
import { ViewerRegistryResourceServiceClient } from '@s4wave/sdk/viewer/registry/registry_srpc.pb.js'
import { viewerRegistrationToComponent } from '@s4wave/web/hooks/useViewerRegistry.js'
import { render, cleanup } from 'vitest-browser-react'
import { page } from 'vitest/browser'
import { Suspense } from 'react'
import { LayoutHostClient } from '@s4wave/sdk/layout/layout_srpc.pb.js'

import { testLayoutCriticalPath } from '../core/e2e/layout-critical-path.js'

describe('Resources SDK with Real Backend E2E', () => {
  let client: E2ETestClient
  let resourceClient: ResourceClient
  let abortController: AbortController

  beforeAll(async () => {
    // Get the test server port from environment
    let port: number
    try {
      port = getTestServerPort()
    } catch {
      // For development, skip if no server is running
      console.warn('Skipping E2E tests: no test server available')
      return
    }

    // Connect to the test server
    client = await createE2EClient(port)

    // Create the Resources client using the starpc Client
    abortController = new AbortController()
    const resourceService = new ResourceServiceClient(client.getClient())
    resourceClient = new ResourceClient(resourceService, abortController.signal)
  })

  afterAll(() => {
    if (abortController) {
      abortController.abort()
    }
    if (resourceClient) {
      resourceClient.dispose()
    }
    if (client) {
      client.disconnect()
    }
  })

  it('connects to the backend via WebSocket', () => {
    if (!client) {
      return // Skip if no server
    }

    expect(client.isConnected()).toBe(true)
  })

  it('accesses root resource and creates Root', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    // Access the root resource
    using rootRef = await resourceClient.accessRootResource()
    expect(rootRef).not.toBeNull()

    // Create Root from the resource reference
    const root = new Root(rootRef)
    expect(root).not.toBeNull()
  })

  it('looks up the local provider', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Lookup the local provider
    using localProvider = await root.lookupProvider('local')
    expect(localProvider).not.toBeNull()

    // Get provider info
    const providerInfo = await localProvider.getProviderInfo()
    expect(providerInfo).toBeDefined()
    expect(providerInfo?.providerId).toBe('local')
  })

  it('creates a local provider account', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Create a local provider account
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const accountResp = await lp.createAccount(abortController.signal)
    expect(accountResp).toBeDefined()
    expect(accountResp.sessionListEntry).toBeDefined()
    expect(accountResp.sessionListEntry?.sessionRef).toBeDefined()
  })

  it('mounts a session and gets session info', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    await using stack = new AsyncDisposableStack()

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Create account and mount session
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const accountResp = await lp.createAccount(abortController.signal)
    const session = await root.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortController.signal,
    )
    stack.defer(() => session[Symbol.dispose]())

    // Get session info
    const sessionInfo = await session.getSessionInfo(abortController.signal)
    expect(sessionInfo).toBeDefined()
    expect(sessionInfo.sessionRef).toBeDefined()
    expect(sessionInfo.peerId).toBeTruthy()
  })

  it('creates a space within a session', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    await using stack = new AsyncDisposableStack()

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Create account and mount session
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const accountResp = await lp.createAccount(abortController.signal)
    const session = await root.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortController.signal,
    )
    stack.defer(() => session[Symbol.dispose]())

    // Create a space
    const spaceResp = await session.createSpace(
      { spaceName: 'E2E Test Space' },
      abortController.signal,
    )
    expect(spaceResp).toBeDefined()
    expect(spaceResp.sharedObjectRef).toBeDefined()
    expect(spaceResp.sharedObjectMeta).toBeDefined()
  })

  it('mounts a shared object', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    await using stack = new AsyncDisposableStack()

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Create account, mount session, create space
    console.log('creating local provider account...')
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const accountResp = await lp.createAccount(abortController.signal)
    console.log('mounting session...')
    const session = await root.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortController.signal,
    )
    stack.defer(() => session[Symbol.dispose]())

    console.log('creating space...')
    const spaceResp = await session.createSpace(
      { spaceName: 'Mount SharedObject Test Space' },
      abortController.signal,
    )

    // Mount the shared object
    console.log('mounting shared object...')
    const sharedObjectId = spaceResp.sharedObjectRef?.providerResourceRef?.id
    console.log('sharedObjectId:', sharedObjectId)
    const spaceSo = await session.mountSharedObject(
      { sharedObjectId },
      abortController.signal,
    )
    stack.defer(() => spaceSo[Symbol.dispose]())
    console.log('mounted shared object')

    expect(spaceSo).not.toBeNull()
  })

  it('mounts a shared object body', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    await using stack = new AsyncDisposableStack()

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Create account, mount session, create space
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const accountResp = await lp.createAccount(abortController.signal)
    const session = await root.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortController.signal,
    )
    stack.defer(() => session[Symbol.dispose]())

    const spaceResp = await session.createSpace(
      { spaceName: 'Mount SharedObjectBody Test Space' },
      abortController.signal,
    )

    // Mount the shared object
    const sharedObjectId = spaceResp.sharedObjectRef?.providerResourceRef?.id
    const spaceSo = await session.mountSharedObject(
      { sharedObjectId },
      abortController.signal,
    )
    stack.defer(() => spaceSo[Symbol.dispose]())

    // Mount the shared object body
    console.log('mounting shared object body...')
    const spaceSoBody = await spaceSo.mountSharedObjectBody(
      {},
      abortController.signal,
    )
    stack.defer(() => spaceSoBody[Symbol.dispose]())
    console.log('mounted shared object body')

    expect(spaceSoBody).not.toBeNull()
  })

  it('accesses space world state', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    await using stack = new AsyncDisposableStack()

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Create account, mount session, create space
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const accountResp = await lp.createAccount(abortController.signal)
    const session = await root.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortController.signal,
    )
    stack.defer(() => session[Symbol.dispose]())

    const spaceResp = await session.createSpace(
      { spaceName: 'Access World State Test Space' },
      abortController.signal,
    )

    // Mount the shared object
    const sharedObjectId = spaceResp.sharedObjectRef?.providerResourceRef?.id
    const spaceSo = await session.mountSharedObject(
      { sharedObjectId },
      abortController.signal,
    )
    stack.defer(() => spaceSo[Symbol.dispose]())

    // Mount the shared object body
    const spaceSoBody = await spaceSo.mountSharedObjectBody(
      {},
      abortController.signal,
    )
    stack.defer(() => spaceSoBody[Symbol.dispose]())

    // Create Space from body
    const space = new Space(spaceSoBody.resourceRef.createRef(spaceSoBody.id))
    stack.defer(() => space[Symbol.dispose]())

    // Access world state
    console.log('accessing world state...')
    const worldState = await space.accessWorldState(
      true,
      abortController.signal,
    )
    stack.defer(() => worldState[Symbol.dispose]())
    console.log('accessed world state')

    expect(worldState).not.toBeNull()
  })

  it('runs the TypeScript Counter ObjectType and dynamic viewer journey', async () => {
    if (!client || !resourceClient) return

    await using stack = new AsyncDisposableStack()
    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)
    using localProvider = await root.lookupProvider('local')
    const lp = new LocalProvider(localProvider.resourceRef)
    const account = await lp.createAccount(abortController.signal)
    const session = await root.mountSession(
      { sessionRef: account.sessionListEntry?.sessionRef },
      abortController.signal,
    )
    stack.defer(() => session[Symbol.dispose]())
    const created = await session.createSpace(
      { spaceName: 'TypeScript Counter Journey' },
      abortController.signal,
    )
    const sharedObjectId = created.sharedObjectRef?.providerResourceRef?.id
    const sharedObject = await session.mountSharedObject(
      { sharedObjectId },
      abortController.signal,
    )
    stack.defer(() => sharedObject[Symbol.dispose]())
    const body = await sharedObject.mountSharedObjectBody(
      {},
      abortController.signal,
    )
    stack.defer(() => body[Symbol.dispose]())
    const space = new Space(body.resourceRef.createRef(body.id))
    stack.defer(() => space[Symbol.dispose]())
    const contents = await space.mountSpaceContents(abortController.signal)
    stack.defer(() => contents[Symbol.dispose]())

    const world = await space.accessWorldState(true, abortController.signal)
    stack.defer(() => world[Symbol.dispose]())
    const objectKey = 'counter/main'
    using object = await world.createObject(
      objectKey,
      {},
      abortController.signal,
    )
    await setObjectType(world, objectKey, CounterTypeID, abortController.signal)

    const firstAccess = await world.accessTypedObject(
      objectKey,
      abortController.signal,
    )
    expect(firstAccess.typeId).toBe(CounterTypeID)
    const firstRef = world.getResourceRef().createRef(firstAccess.resourceId)
    const first = new Counter(firstRef)
    stack.defer(() => first[Symbol.dispose]())

    // Discriminator probe: capture the typed facts that distinguish the
    // ERR_RPC_ABORT producers. Fixture-only; removed once the abort source is
    // identified and corrected at its owner.
    const releasedEvents: Array<
      ResourceReleasedEvent & { seq: number }
    > = []
    let connectionLost = false
    let eventSeq = 0
    const offReleased = resourceClient.onResourceReleased((event) => {
      releasedEvents.push({ ...event, seq: ++eventSeq })
    })
    const offConnectionLost = resourceClient.onConnectionLost(() => {
      connectionLost = true
    })
    try {
      await first.initialize(7n, abortController.signal)
    } catch (error) {
      offReleased()
      offConnectionLost()
      const observed = {
        callerAborted: abortController.signal.aborted,
        childReleased: firstRef.released,
        connectionLost,
        releasedEvents,
        error:
          error instanceof Error
            ? `${error.name}: ${error.message}`
            : String(error),
      }
      throw new Error(
        'counter discriminator: tuple not yet identified: '
          + JSON.stringify(observed),
        { cause: error },
      )
    }
    offReleased()
    offConnectionLost()
    await expect(first.getCounter(abortController.signal)).resolves.toBe(7n)

    const secondAccess = await world.accessTypedObject(
      objectKey,
      abortController.signal,
    )
    const second = new Counter(
      world.getResourceRef().createRef(secondAccess.resourceId),
    )
    stack.defer(() => second[Symbol.dispose]())
    await expect(second.getCounter(abortController.signal)).resolves.toBe(7n)

    const registry = new ViewerRegistryResourceServiceClient(root.client)
    const viewerSnapshot = await registry
      .WatchViewers({ surface: ViewerSurface.WEB }, abortController.signal)
      [Symbol.asyncIterator]()
      .next()
    const registration = viewerSnapshot.value?.registrations?.find(
      (viewer: ViewerRegistration) =>
        viewer.componentId === 'example.counter.viewer',
    )
    expect(registration?.scriptPath).toMatch(/^\/b\/pa\//)
    const viewer = registration
      ? viewerRegistrationToComponent(registration)
      : null
    expect(viewer).not.toBeNull()
    const Viewer = viewer!.component
    await render(
      <Suspense fallback={<p>Loading counter viewer</p>}>
        <Viewer
          objectInfo={{
            info: {
              case: 'worldObjectInfo',
              value: { objectKey, objectType: CounterTypeID },
            },
          }}
          worldState={{
            value: world,
            loading: false,
            error: null,
            retry: () => {},
          }}
        />
      </Suspense>,
    )
    await expect
      .element(page.getByLabelText('Counter value'))
      .toHaveTextContent('7')

    const layout = new LayoutHostClient(client.getClient())
    await layout.NavigateTab(
      { tabId: 'stop-sdk-fixture-plugin' },
      abortController.signal,
    )
    await expect(second.getCounter(abortController.signal)).rejects.toThrow()
    await cleanup()
  })

  it('accesses state atom from root', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Access the default state atom store
    console.log('accessing state atom...')
    using stateAtom = await root.accessStateAtom({}, abortController.signal)
    console.log('accessed state atom')
    expect(stateAtom).not.toBeNull()

    // Get initial state (should be empty or default)
    console.log('getting initial state...')
    const initialState = await stateAtom.getState()
    console.log('got initial state')
    expect(initialState).toBeDefined()
  })

  it('runs repeated ObjectLayout NavigateTab ops through the real backend critical path', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    await testLayoutCriticalPath(root, abortController.signal)
  })

  it('computes and validates hashes', async () => {
    if (!client || !resourceClient) {
      return // Skip if no server
    }

    using rootRef = await resourceClient.accessRootResource()
    const root = new Root(rootRef)

    // Compute a hash
    console.log('computing hash...')
    const testData = new TextEncoder().encode('hello world')
    const hash = await root.hashSum(1, testData, abortController.signal) // 1 = SHA256
    console.log('computed hash')
    expect(hash).not.toBeNull()

    // Validate the hash
    console.log('validating hash...')
    const validation = await root.hashValidate(hash, abortController.signal)
    console.log('validated hash')
    expect(validation.valid).toBe(true)

    // Marshal and parse hash
    console.log('marshaling hash...')
    const hashStr = await root.marshalHash(hash, abortController.signal)
    console.log('marshaled hash')
    expect(hashStr).toBeTruthy()

    console.log('parsing hash...')
    const parsedHash = await root.parseHash(hashStr, abortController.signal)
    console.log('parsed hash')
    expect(parsedHash).not.toBeNull()
    expect(parsedHash?.hashType).toBe(hash?.hashType)
  })
})
