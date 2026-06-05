import { initObjectLayout } from '@s4wave/app/quickstart/create.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import { OBJECT_LAYOUT_OBJECT_KEY } from '@s4wave/core/space/world/ops/init-object-layout.js'
import type {
  LayoutModel,
  WatchLayoutModelRequest,
} from '@s4wave/sdk/layout/layout.pb.js'
import { LayoutHostHandle } from '@s4wave/sdk/layout/layout-host.js'
import {
  layoutModelToJsonModel,
  type TabDataMap,
} from '@s4wave/sdk/layout/layout.js'
import {
  ObjectLayoutTab,
  type ObjectLayoutTab as ObjectLayoutTabType,
} from '@s4wave/sdk/layout/world/world.pb.js'
import {
  createObjectLayoutReplaceTabRequest,
  ObjectLayoutTypeID,
} from '@s4wave/sdk/layout/world/object-layout.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { BASE_MODEL } from '@s4wave/web/layout/layout.js'

import { withTimeout } from './test-utils.js'

interface RouteTarget {
  sessionIdx: number
  sharedObjectId: string
}

interface LayoutTabSummary {
  id: string
  name: string
  path: string
  infoCase: string
  objectKey: string
  objectType: string
  unixfsId: string
  unixfsPath: string
}

interface LayoutRuntimeResult {
  action: string
  objectHash: string
  tabCount: number
  tabs: LayoutTabSummary[]
  created?: boolean
  sameMountTabCount?: number
  sameMountTabs?: LayoutTabSummary[]
  navigatedPath?: string
  replacedName?: string
}

interface MountedWorlds {
  readWorld: EngineWorldState
  writeWorld: EngineWorldState
}

type LayoutRuntimeArgs = {
  action: 'prepare' | 'inspect' | 'seed-model' | 'typed-mutate'
  deadlineMs?: number
}

type ResourceLike = { release?: () => void }

function currentRouteTarget(): RouteTarget {
  const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
  if (!match) {
    throw new Error('expected current /u/{idx}/so/{id} route')
  }
  return {
    sessionIdx: Number(match[1]),
    sharedObjectId: decodeURIComponent(match[2] ?? ''),
  }
}

function objectHash(target: RouteTarget): string {
  return `#/u/${target.sessionIdx}/so/${encodeURIComponent(target.sharedObjectId)}/-/${OBJECT_LAYOUT_OBJECT_KEY}`
}

function cleanupTracker() {
  const stack: ResourceLike[] = []
  const cleanup = <T extends ResourceLike>(resource: T): T => {
    stack.push(resource)
    return resource
  }
  return {
    cleanup,
    releaseAll() {
      for (;;) {
        const resource = stack.pop()
        if (!resource) {
          return
        }
        resource.release?.()
      }
    },
  }
}

async function withMountedSpace<T>(
  signal: AbortSignal,
  fn: (worlds: MountedWorlds) => Promise<T>,
): Promise<T> {
  const debug = globalThis.__s4wave_debug
  const root = debug?.root
  if (!root) {
    throw new Error('missing __s4wave_debug.root')
  }
  const target = currentRouteTarget()
  const tracker = cleanupTracker()
  try {
    const mounted = await root.mountSessionByIdx(
      { sessionIdx: target.sessionIdx },
      signal,
    )
    const session = tracker.cleanup(mounted?.session)
    if (!session) {
      throw new Error('mountSessionByIdx returned no session')
    }
    const space = tracker.cleanup(
      await mountSpace({
        session,
        spaceResp: {
          sharedObjectRef: {
            providerResourceRef: { id: target.sharedObjectId },
          },
        },
        abortSignal: signal,
        cleanup: tracker.cleanup,
      }),
    )
    const writeWorld = tracker.cleanup(
      await space.accessWorldState(true, signal),
    )
    const readWorld = new EngineWorldState(writeWorld.getEngine(), false)
    return await fn({ readWorld, writeWorld })
  } finally {
    tracker.releaseAll()
  }
}

async function accessLayoutHost(
  world: EngineWorldState,
  signal: AbortSignal,
): Promise<LayoutHostHandle> {
  const access = await world.accessTypedObject(OBJECT_LAYOUT_OBJECT_KEY, signal)
  if (access.typeId !== ObjectLayoutTypeID) {
    throw new Error(
      `expected ObjectLayout type ${ObjectLayoutTypeID}, got ${access.typeId}`,
    )
  }
  if (!access.resourceId) {
    throw new Error('ObjectLayout access returned no resource id')
  }
  return world
    .getResourceRef()
    .createResource(access.resourceId, LayoutHostHandle)
}

async function* idleLayoutRequests(
  signal: AbortSignal,
): AsyncGenerator<WatchLayoutModelRequest, void, void> {
  yield {}
  await new Promise<void>((resolve) => {
    signal.addEventListener('abort', () => resolve(), { once: true })
  })
}

async function nextModel(
  iter: AsyncIterator<LayoutModel>,
  signal: AbortSignal,
  label: string,
): Promise<LayoutModel> {
  const next = await Promise.race([
    iter.next(),
    new Promise<never>((_resolve, reject) => {
      const tid = setTimeout(
        () => reject(new Error(label + ' timed out')),
        15000,
      )
      signal.addEventListener(
        'abort',
        () => {
          clearTimeout(tid)
          reject(new Error(label + ' aborted'))
        },
        { once: true },
      )
    }),
  ])
  if (next.done || !next.value) {
    throw new Error(label + ' stream ended')
  }
  return next.value
}

async function waitForModel(
  iter: AsyncIterator<LayoutModel>,
  signal: AbortSignal,
  label: string,
  pred: (model: LayoutModel) => boolean,
): Promise<LayoutModel> {
  const deadline = performance.now() + 15000
  while (performance.now() < deadline) {
    const model = await nextModel(iter, signal, label)
    if (pred(model)) {
      return model
    }
  }
  throw new Error(label + ' did not reach expected model')
}

function allTabs(model: LayoutModel): LayoutTabSummary[] {
  const tabDataMap: TabDataMap = {}
  const jsonModel = layoutModelToJsonModel(BASE_MODEL, tabDataMap, model)
  const tabs: LayoutTabSummary[] = []
  const visitNode = (node: unknown) => {
    if (!node || typeof node !== 'object') return
    const typed = node as {
      type?: string
      id?: string
      name?: string
      children?: unknown[]
      config?: Uint8Array
    }
    if (typed.type === 'tab') {
      tabs.push(
        summarizeTab({
          id: typed.id,
          name: typed.name,
          data:
            (typed.id ? tabDataMap[typed.id] : undefined) ??
            (typed.config instanceof Uint8Array ? typed.config : undefined),
        }),
      )
      return
    }
    for (const child of typed.children ?? []) {
      visitNode(child)
    }
  }
  visitNode(jsonModel.layout)
  for (const border of jsonModel.borders ?? []) {
    for (const child of border.children ?? []) {
      visitNode(child)
    }
  }
  if (tabs.length === 0) {
    const visitProtoRow = (
      row: NonNullable<LayoutModel['layout']> | undefined,
    ) => {
      for (const child of row?.children ?? []) {
        const node = child.node
        if (node?.case === 'row') {
          visitProtoRow(node.value)
          continue
        }
        if (node?.case === 'tabSet') {
          for (const tab of node.value.children ?? []) {
            tabs.push(summarizeTab(tab))
          }
        }
      }
    }
    visitProtoRow(model.layout)
  }
  return tabs
}

function summarizeTab(tab: {
  id?: string
  name?: string
  data?: Uint8Array
}): LayoutTabSummary {
  const decoded = decodeTab(tab.data)
  const info = decoded?.objectInfo?.info
  const worldInfo = info?.case === 'worldObjectInfo' ? info.value : null
  const unixfsInfo = info?.case === 'unixfsObjectInfo' ? info.value : null
  return {
    id: tab.id ?? '',
    name: tab.name ?? '',
    path: decoded?.path ?? '',
    infoCase: info?.case ?? '',
    objectKey: worldInfo?.objectKey ?? '',
    objectType: worldInfo?.objectType ?? '',
    unixfsId: unixfsInfo?.unixfsId ?? '',
    unixfsPath: unixfsInfo?.path ?? '',
  }
}

function decodeTab(data: Uint8Array | undefined): ObjectLayoutTabType | null {
  if (!data || data.length === 0) return null
  return ObjectLayoutTab.fromBinary(data)
}

async function inspectLayout(
  signal: AbortSignal,
): Promise<LayoutRuntimeResult> {
  const target = currentRouteTarget()
  return await withMountedSpace(signal, async ({ readWorld }) => {
    const layoutHost = await accessLayoutHost(readWorld, signal)
    try {
      const stream = layoutHost.WatchLayoutModel(
        idleLayoutRequests(signal),
        signal,
      )
      const iter = stream[Symbol.asyncIterator]()
      const model = await nextModel(iter, signal, 'inspect layout model')
      const tabs = allTabs(model)
      return {
        action: 'inspect',
        objectHash: objectHash(target),
        tabCount: tabs.length,
        tabs,
      }
    } finally {
      layoutHost.release()
    }
  })
}

async function prepareLayout(
  signal: AbortSignal,
): Promise<LayoutRuntimeResult> {
  const target = currentRouteTarget()
  const created = await withMountedSpace(
    signal,
    async ({ readWorld, writeWorld }) => {
      const existing = await readWorld.getObject(
        OBJECT_LAYOUT_OBJECT_KEY,
        signal,
      )
      if (existing) {
        existing.release()
        return false
      }
      await initObjectLayout(writeWorld, signal)
      return true
    },
  )
  const result = await inspectLayout(signal)
  return {
    ...result,
    action: 'prepare',
    objectHash: objectHash(target),
    created,
  }
}

async function seedModel(signal: AbortSignal): Promise<LayoutRuntimeResult> {
  const target = currentRouteTarget()
  const seed = await withMountedSpace(
    signal,
    async ({ readWorld, writeWorld }) => {
      const existing = await readWorld.getObject(
        OBJECT_LAYOUT_OBJECT_KEY,
        signal,
      )
      if (existing) {
        existing.release()
      }
      const created = !existing
      if (created) {
        await initObjectLayout(writeWorld, signal)
      }

      const layoutHost = await accessLayoutHost(readWorld, signal)
      try {
        const stream = layoutHost.WatchLayoutModel(
          idleLayoutRequests(signal),
          signal,
        )
        const iter = stream[Symbol.asyncIterator]()
        const model = await nextModel(
          iter,
          signal,
          'same-mount seed layout model',
        )
        return {
          created,
          sameMountTabs: allTabs(model),
        }
      } finally {
        layoutHost.release()
      }
    },
  )

  const remounted = await inspectLayout(signal)
  return {
    ...remounted,
    action: 'seed-model',
    objectHash: objectHash(target),
    created: seed.created,
    sameMountTabCount: seed.sameMountTabs.length,
    sameMountTabs: seed.sameMountTabs,
  }
}

async function mutateLayout(signal: AbortSignal): Promise<LayoutRuntimeResult> {
  const target = currentRouteTarget()
  return await withMountedSpace(signal, async ({ readWorld }) => {
    const layoutHost = await accessLayoutHost(readWorld, signal)
    try {
      const stream = layoutHost.WatchLayoutModel(
        idleLayoutRequests(signal),
        signal,
      )
      const iter = stream[Symbol.asyncIterator]()
      const initial = await nextModel(
        iter,
        signal,
        'initial typed layout model',
      )
      const filesTab = allTabs(initial).find((tab) => tab.id === 'files')
      if (!filesTab) {
        throw new Error('expected files tab before typed mutation')
      }

      await layoutHost.NavigateTab(
        { tabId: filesTab.id, path: '/getting-started.md' },
        signal,
      )
      const navigated = await waitForModel(
        iter,
        signal,
        'navigate typed layout model',
        (model) =>
          allTabs(model).some(
            (tab) =>
              tab.id === filesTab.id && tab.path === '/getting-started.md',
          ),
      )

      await layoutHost.ReplaceTab(
        createObjectLayoutReplaceTabRequest({
          tabId: filesTab.id,
          name: 'Files Replaced',
          objectKey: 'files',
          objectType: '',
          path: '',
        }),
        signal,
      )
      const replaced = await waitForModel(
        iter,
        signal,
        'replace typed layout model',
        (model) =>
          allTabs(model).some(
            (tab) => tab.id === filesTab.id && tab.name === 'Files Replaced',
          ),
      )
      const replacedTabs = allTabs(replaced)
      return {
        action: 'typed-mutate',
        objectHash: objectHash(target),
        tabCount: replacedTabs.length,
        tabs: replacedTabs,
        navigatedPath:
          allTabs(navigated).find((tab) => tab.id === filesTab.id)?.path ?? '',
        replacedName:
          replacedTabs.find((tab) => tab.id === filesTab.id)?.name ?? '',
      }
    } finally {
      layoutHost.release()
    }
  })
}

export default async function (
  args: LayoutRuntimeArgs,
): Promise<LayoutRuntimeResult> {
  const deadlineMs = args.deadlineMs ?? 120000
  return await withTimeout(deadlineMs, async (signal) => {
    switch (args.action) {
      case 'prepare':
        return await prepareLayout(signal)
      case 'inspect':
        return await inspectLayout(signal)
      case 'seed-model':
        return await seedModel(signal)
      case 'typed-mutate':
        return await mutateLayout(signal)
    }
  })
}
