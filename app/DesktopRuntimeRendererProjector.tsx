import { useBldrContext, useAbortSignalEffect } from '@aptre/bldr-react'
import { isDesktop, type WebDocument as BldrWebDocument } from '@aptre/bldr'
import { Client as SRPCClient } from 'starpc'
import {
  Client as ResourceClient,
  ResourceServiceClient,
} from '@aptre/bldr-sdk/resource/index.js'

import {
  DesktopRuntimeHealth,
  DesktopRuntimeLifecycle,
  DesktopRuntimeReachability,
  type DesktopRuntimeNavigationItem,
  type DesktopRuntimeState,
} from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime.pb.js'
import { DesktopRuntimeResourceServiceClient } from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime_srpc.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type {
  SessionListEntry,
  SessionMetadata,
} from '@s4wave/core/session/session.pb.js'
import type { WatchResourcesListResponse } from '@s4wave/sdk/session/session.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'

interface DesktopRuntimeRendererProjectorProps {
  root: Root
}

declare global {
  interface Window {
    __spacewaveDesktopProjection?: DesktopRuntimeProjectionDebugState
  }
}

interface DesktopRuntimeProjectionDebugState {
  mounted: boolean
  isDesktop: boolean
  hasDocument: boolean
  status: string
  sessions: number
  spaces: number
  error?: string
}

// DesktopRuntimeRendererProjector publishes visible app state into Electron main.
export function DesktopRuntimeRendererProjector({
  root,
}: DesktopRuntimeRendererProjectorProps) {
  const ctx = useBldrContext()
  const doc = ctx?.webDocument

  useAbortSignalEffect(
    (signal) => {
      if (!isDesktop || !doc) {
        setProjectionDebug({
          mounted: true,
          isDesktop,
          hasDocument: Boolean(doc),
          status: 'skipped',
          sessions: 0,
          spaces: 0,
        })
        return
      }
      setProjectionDebug({
        mounted: true,
        isDesktop,
        hasDocument: true,
        status: 'connecting',
        sessions: 0,
        spaces: 0,
      })
      void projectDesktopRuntimeFromRenderer(root, doc, signal).catch(
        (err: unknown) => {
          if (signal.aborted) return
          setProjectionDebug({
            mounted: true,
            isDesktop,
            hasDocument: true,
            status: 'error',
            sessions: 0,
            spaces: 0,
            error: String(err),
          })
          console.error('desktop runtime projection failed', err)
        },
      )
    },
    [root, doc],
  )

  return null
}

async function projectDesktopRuntimeFromRenderer(
  root: Root,
  doc: BldrWebDocument,
  signal: AbortSignal,
): Promise<void> {
  const rpc = new SRPCClient(() => doc.openWebDocumentHostStream())
  const resource = new ResourceClient(new ResourceServiceClient(rpc), signal)
  setProjectionDebug({
    mounted: true,
    isDesktop,
    hasDocument: true,
    status: 'accessing-desktop-root',
    sessions: 0,
    spaces: 0,
  })
  const desktopRoot = await resource.accessRootResource()
  const desktop = new DesktopRuntimeResourceServiceClient(desktopRoot.client)
  setProjectionDebug({
    mounted: true,
    isDesktop,
    hasDocument: true,
    status: 'watching-sessions',
    sessions: 0,
    spaces: 0,
  })
  const sessions = root.watchSessions({}, signal)[Symbol.asyncIterator]()
  let current = await sessions.next()
  try {
    while (!signal.aborted && !current.done) {
      const projection = await buildProjection(
        root,
        current.value.sessions ?? [],
        signal,
      )
      try {
        await desktop.SetDesktopState({ state: projection.state }, signal)
        setProjectionDebug({
          mounted: true,
          isDesktop,
          hasDocument: true,
          status: 'published',
          sessions: projection.state.sessions?.length ?? 0,
          spaces: projection.state.spaces?.length ?? 0,
        })
        current = await Promise.race([
          sessions.next(),
          projection.changed.then(() => current),
        ])
      } finally {
        projection.release()
      }
    }
  } finally {
    await sessions.return?.()
    desktopRoot.release()
    resource.dispose()
  }
}

async function buildProjection(
  root: Root,
  entries: SessionListEntry[],
  signal: AbortSignal,
): Promise<{
  state: DesktopRuntimeState
  changed: Promise<void>
  release: () => void
}> {
  const sessions: DesktopRuntimeNavigationItem[] = []
  const spaces: DesktopRuntimeNavigationItem[] = []
  const waits: Array<Promise<void>> = []
  const releases: Array<() => void> = []

  for (const entry of entries) {
    const idx = entry.sessionIndex ?? 0
    if (!idx) continue
    const metadata = await root.getSessionMetadata(idx, signal)
    const meta = metadata.metadata
    sessions.push(buildSessionItem(entry, meta))

    const mounted = await root.mountSessionByIdx({ sessionIdx: idx }, signal)
    if (!mounted) continue
    releases.push(() => mounted.session.release())
    const resources: AsyncIterable<WatchResourcesListResponse> =
      mounted.session.watchResourcesList({}, signal)
    const stream = resources[Symbol.asyncIterator]()
    releases.push(() => {
      void stream.return?.()
    })
    const first = await stream.next()
    if (!first.done) {
      for (const space of first.value.spacesList ?? []) {
        spaces.push(buildSpaceItem(idx, meta, space))
      }
    }
    waits.push(
      stream.next().then(
        () => {},
        () => {},
      ),
    )
  }

  return {
    state: {
      statusText: 'Running',
      health: DesktopRuntimeHealth.HEALTHY,
      lifecycle: DesktopRuntimeLifecycle.RUNNING,
      listener: {
        reachability: DesktopRuntimeReachability.REACHABLE,
        label: 'Runtime',
        detail: 'Ready',
      },
      sessions,
      spaces,
      activity: [],
      update: {},
      attentionItems: [],
      actions: [],
    },
    changed: waits.length ? Promise.race(waits) : new Promise(() => {}),
    release: () => {
      while (releases.length) {
        releases.pop()?.()
      }
    },
  }
}

function buildSessionItem(
  entry: SessionListEntry,
  meta: SessionMetadata | undefined,
): DesktopRuntimeNavigationItem {
  const idx = entry.sessionIndex ?? 0
  return {
    id: `session-${idx}`,
    label: meta?.displayName || `Session ${idx}`,
    detail: meta?.providerDisplayName || providerLabel(entry),
    route: `/u/${idx}/`,
    active: true,
    statusText: 'Ready',
  }
}

function buildSpaceItem(
  sessionIndex: number,
  meta: SessionMetadata | undefined,
  space: SpaceSoListEntry,
): DesktopRuntimeNavigationItem {
  const id = space.entry?.ref?.providerResourceRef?.id ?? ''
  return {
    id: `space-${sessionIndex}-${id}`,
    label: space.spaceMeta?.name || id || 'Untitled space',
    detail: meta?.displayName || meta?.providerDisplayName || '',
    route: `/u/${sessionIndex}/so/${id}`,
    active: true,
    statusText: spaceStatusText(space),
  }
}

function providerLabel(entry: SessionListEntry): string {
  const providerId = entry.sessionRef?.providerResourceRef?.providerId ?? ''
  if (providerId === 'spacewave') return 'Cloud'
  if (providerId === 'local') return 'Local'
  return providerId
}

function spaceStatusText(space: SpaceSoListEntry): string {
  if (space.entry?.source === 'created') return 'Created'
  if (space.entry?.source === 'shared') return 'Shared'
  return 'Available'
}

function setProjectionDebug(state: DesktopRuntimeProjectionDebugState): void {
  window.__spacewaveDesktopProjection = state
}
