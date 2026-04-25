import { AsyncDisposableStack } from '@aptre/bldr-sdk/defer.js'
import { Root } from '@s4wave/sdk/root'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { ObjectLayoutTypeID } from '@s4wave/web/object/LayoutObjectViewer.js'
import {
  OBJECT_LAYOUT_OBJECT_KEY,
  INIT_OBJECT_LAYOUT_OP_ID,
} from '@s4wave/core/space/world/ops/init-object-layout.js'
import { InitObjectLayoutOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { createQuickstartSetupFromSession } from '@s4wave/app/quickstart/create.js'
import { LayoutHostClient } from '@s4wave/sdk/layout/layout_srpc.pb.js'

// testTypedObject tests the typed object system with ObjectLayout.
export async function testTypedObject(
  rootResource: Root,
  abortSignal: AbortSignal,
) {
  await using stack = new AsyncDisposableStack()

  // create a local provider account
  using localProviderHandle = await rootResource.lookupProvider('local')
  const lp = new LocalProvider(localProviderHandle.resourceRef)
  const localProviderAccountResp = await lp.createAccount(abortSignal)

  // mount the session
  const localSession = await rootResource.mountSession(
    { sessionRef: localProviderAccountResp.sessionListEntry?.sessionRef },
    abortSignal,
  )
  stack.defer(() => localSession[Symbol.dispose]())

  // create the space
  const createSpaceResponse = await localSession.createSpace(
    { spaceName: 'Typed Object Test Space' },
    abortSignal,
  )

  // mount the space and access world state
  const setup = await createQuickstartSetupFromSession({
    session: localSession,
    spaceResp: createSpaceResponse,
    abortSignal,
    cleanup: (r) => {
      if (r) stack.defer(() => r[Symbol.dispose]())
      return r
    },
  })

  // Access the Engine to create transactions
  const engine = await setup.space.accessWorld(abortSignal)
  stack.defer(() => engine[Symbol.dispose]())

  // Create a write transaction
  using tx = await engine.newTransaction(true, abortSignal)

  // Create the ObjectLayout demo object using the world op
  const op = InitObjectLayoutOp.create({
    objectKey: OBJECT_LAYOUT_OBJECT_KEY,
    timestamp: new Date(),
  })
  const { sysErr } = await tx.applyWorldOp(
    INIT_OBJECT_LAYOUT_OP_ID,
    InitObjectLayoutOp.toBinary(op),
    '',
    abortSignal,
  )
  if (sysErr) {
    throw new Error('applyWorldOp returned system error')
  }

  // Commit the transaction
  await tx.commit(abortSignal)

  // Test 1: Access typed object via Tx (WorldStateResource)
  console.log('testing accessTypedObject via Tx (WorldStateResource)...')
  {
    using readTx = await engine.newTransaction(false, abortSignal)

    const typedAccess = await readTx.accessTypedObject(
      OBJECT_LAYOUT_OBJECT_KEY,
      abortSignal,
    )

    if (typedAccess.typeId !== ObjectLayoutTypeID) {
      throw new Error(
        `expected typeId ${ObjectLayoutTypeID}, got ${typedAccess.typeId}`,
      )
    }

    if (!typedAccess.resourceId) {
      throw new Error('expected non-zero resourceId from Tx')
    }

    // Verify we can use getResourceRef to create a LayoutHostClient
    const resourceRef = readTx.getResourceRef()
    const ref = resourceRef.createRef(typedAccess.resourceId)
    const _layoutHostClient = new LayoutHostClient(ref.client)
    ref.release()

    console.log('Tx accessTypedObject test passed')
  }

  // Test 2: Access typed object via EngineWorldState
  console.log('testing accessTypedObject via EngineWorldState...')
  {
    // Get EngineWorldState from space (this is what SpaceContainer uses)
    const engineWorldState = await setup.space.accessWorldState(
      false,
      abortSignal,
    )
    stack.defer(() => engineWorldState[Symbol.dispose]())

    const typedAccess = await engineWorldState.accessTypedObject(
      OBJECT_LAYOUT_OBJECT_KEY,
      abortSignal,
    )

    if (typedAccess.typeId !== ObjectLayoutTypeID) {
      throw new Error(
        `expected typeId ${ObjectLayoutTypeID}, got ${typedAccess.typeId}`,
      )
    }

    if (!typedAccess.resourceId) {
      throw new Error('expected non-zero resourceId from EngineWorldState')
    }

    // Verify we can use getResourceRef to create a LayoutHostClient
    const resourceRef = engineWorldState.getResourceRef()
    const ref = resourceRef.createRef(typedAccess.resourceId)
    const _layoutHostClient = new LayoutHostClient(ref.client)
    ref.release()

    console.log('EngineWorldState accessTypedObject test passed')
  }

  // Test 3: Verify IWorldState interface works generically
  console.log('testing IWorldState interface generically...')
  {
    // This function accepts any IWorldState implementation
    async function testIWorldState(worldState: {
      accessTypedObject: (
        key: string,
        signal?: AbortSignal,
      ) => Promise<{ resourceId: number; typeId: string }>
      getResourceRef: () => {
        createRef: (id: number) => { client: unknown; release: () => void }
      }
    }) {
      const access = await worldState.accessTypedObject(
        OBJECT_LAYOUT_OBJECT_KEY,
        abortSignal,
      )
      if (access.typeId !== ObjectLayoutTypeID) {
        throw new Error(`wrong typeId: ${access.typeId}`)
      }
      const ref = worldState.getResourceRef().createRef(access.resourceId)
      ref.release()
    }

    // Test with EngineWorldState
    const engineWorldState = await setup.space.accessWorldState(
      false,
      abortSignal,
    )
    await testIWorldState(engineWorldState)
    engineWorldState[Symbol.dispose]()

    // Test with Tx (WorldStateResource)
    using readTx = await engine.newTransaction(false, abortSignal)
    await testIWorldState(readTx)

    console.log('IWorldState interface test passed')
  }

  console.log('typed object test completed successfully')
}
