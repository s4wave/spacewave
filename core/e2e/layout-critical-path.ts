import { AsyncDisposableStack } from '@aptre/bldr-sdk/defer.js'

import { initObjectLayout, initUnixFS } from '@s4wave/app/quickstart/create.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import { OBJECT_LAYOUT_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-object-layout.js'
import type {
  LayoutModel,
  WatchLayoutModelRequest,
} from '@s4wave/sdk/layout/layout.pb.js'
import { LayoutHostHandle } from '@s4wave/sdk/layout/layout-host.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { Root } from '@s4wave/sdk/root/root.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { ObjectLayoutTypeID } from '@s4wave/web/object/LayoutObjectViewer.js'

const layoutPaths = ['/test', '/test/a', '/test/b', '/test/c', '/test/final']

type LayoutTabState = {
  tabId: string
  path: string
}

function registerCleanup(stack: AsyncDisposableStack) {
  return <R extends Disposable | null | undefined>(resource: R): R => {
    if (resource) {
      stack.defer(() => resource[Symbol.dispose]())
    }
    return resource
  }
}

async function* idleLayoutRequests(
  abortSignal: AbortSignal,
): AsyncGenerator<WatchLayoutModelRequest, void, void> {
  if (abortSignal.aborted) {
    return
  }
  yield {}

  await new Promise<void>((resolve) => {
    abortSignal.addEventListener('abort', () => resolve(), { once: true })
  })
}

async function nextLayoutModel(
  iter: AsyncIterator<LayoutModel>,
): Promise<LayoutModel> {
  const next = await iter.next()
  if (next.done || !next.value) {
    throw new Error('layout stream ended unexpectedly')
  }
  return next.value
}

function getMainFilesTabState(model: LayoutModel): LayoutTabState {
  const firstChild = model.layout?.children?.[0]
  const tabSet =
    firstChild?.node?.case === 'tabSet' ? firstChild.node.value : undefined
  if (!tabSet) {
    throw new Error('expected main tabset in layout model')
  }

  const tab = tabSet.children?.[0]
  if (!tab) {
    throw new Error('expected Files tab in main tabset')
  }

  const tabId = tab.id ?? ''
  if (!tabId) {
    throw new Error('expected Files tab id')
  }

  const tabData = ObjectLayoutTab.fromBinary(tab.data ?? new Uint8Array())
  return {
    tabId,
    path: tabData.path ?? '',
  }
}

// testLayoutCriticalPath exercises repeated ObjectLayout NavigateTab ops through
// the real local-provider shared-object path and verifies each persisted path
// update through WatchLayoutModel.
export async function testLayoutCriticalPath(
  rootResource: Root,
  abortSignal: AbortSignal,
): Promise<void> {
  await using stack = new AsyncDisposableStack()
  const cleanup = registerCleanup(stack)

  using localProvider = await rootResource.lookupProvider('local')
  const lp = new LocalProvider(localProvider.resourceRef)
  const accountResp = await lp.createAccount(abortSignal)

  const session = cleanup(
    await rootResource.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortSignal,
    ),
  )

  const spaceResp = await session.createSpace(
    { spaceName: 'Layout Critical Path Trace' },
    abortSignal,
  )
  const space = await mountSpace({ session, spaceResp, abortSignal, cleanup })
  const engine = cleanup(await space.accessWorld(abortSignal))
  const writeWorld = new EngineWorldState(engine, true)
  const readWorld = new EngineWorldState(engine, false)

  await initUnixFS(writeWorld, abortSignal)
  await initObjectLayout(writeWorld, abortSignal)

  const typedAccess = await readWorld.accessTypedObject(
    OBJECT_LAYOUT_OBJECT_KEY,
    abortSignal,
  )
  if (typedAccess.typeId !== ObjectLayoutTypeID) {
    throw new Error(
      `expected typeId ${ObjectLayoutTypeID}, got ${typedAccess.typeId}`,
    )
  }
  if (!typedAccess.resourceId) {
    throw new Error('expected non-zero layout resource id')
  }

  using layoutHost = readWorld
    .getResourceRef()
    .createResource(typedAccess.resourceId, LayoutHostHandle)

  const stream = layoutHost.WatchLayoutModel(
    idleLayoutRequests(abortSignal),
    abortSignal,
  )
  const iter = stream[Symbol.asyncIterator]()

  const initialTab = getMainFilesTabState(await nextLayoutModel(iter))
  if (initialTab.tabId !== 'files') {
    throw new Error(`expected files tab id, got ${initialTab.tabId}`)
  }
  if (initialTab.path !== '') {
    throw new Error(`expected empty initial path, got ${initialTab.path}`)
  }

  for (const path of layoutPaths) {
    await layoutHost.NavigateTab({ tabId: initialTab.tabId, path }, abortSignal)

    const nextTab = getMainFilesTabState(await nextLayoutModel(iter))
    if (nextTab.tabId !== initialTab.tabId) {
      throw new Error(
        `expected tab id ${initialTab.tabId}, got ${nextTab.tabId}`,
      )
    }
    if (nextTab.path !== path) {
      throw new Error(`expected path ${path}, got ${nextTab.path}`)
    }
  }
}
