import { mountSpace } from '@s4wave/app/space/space.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import {
  Device,
  DeviceLiveness,
  DeviceSetupState,
  DeviceUpdateState,
} from '@s4wave/sdk/device/device.pb.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'

import { withTimeout } from './test-utils.js'

interface RouteTarget {
  sessionIdx: number
  sharedObjectId: string
}

type ResourceLike = { release?: () => void }

// This helper is presentation-only. Lifecycle proofs must use real enrollment.
type ActionArgs = {
  action: 'seed-device'
  objectKey?: string
  label?: string
  peerId?: string
  deadlineMs?: number
}

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

function cleanupTracker() {
  const stack: ResourceLike[] = []
  const cleanup = <T extends ResourceLike | undefined | null>(
    resource: T,
  ): T => {
    if (resource?.release) {
      stack.push(resource)
    }
    return resource
  }
  return {
    cleanup,
    releaseAll() {
      for (;;) {
        const resource = stack.pop()
        if (!resource) return
        resource.release?.()
      }
    },
  }
}

async function withMountedWriteWorld<T>(
  signal: AbortSignal,
  fn: (world: EngineWorldState) => Promise<T>,
): Promise<T> {
  const root = globalThis.__s4wave_debug?.root
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
    return await fn(writeWorld)
  } finally {
    tracker.releaseAll()
  }
}

async function seedDevice(args: ActionArgs, signal: AbortSignal) {
  const objectKey = args.objectKey ?? 'devices/e2e-build-host'
  const label = args.label ?? 'E2E Build Host'
  const peerId = args.peerId ?? '12D3KooWE2EDevice'
  return await withMountedWriteWorld(signal, async (world) => {
    const existing = await world.getObject(objectKey, signal)
    if (existing) {
      existing.release()
      return {
        action: 'seed-device',
        created: false,
        objectKey,
        typeId: DeviceTypeID,
        label,
        peerId,
      }
    }

    const now = new Date()
    const data = Device.toBinary({
      peerId,
      label,
      platform: { os: 'linux', arch: 'arm64' },
      daemonVersion: 'e2e-device-viewer',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      updateState: DeviceUpdateState.IDLE,
      lastStatus: {
        liveness: DeviceLiveness.ONLINE,
        message: 'seeded by browser e2e',
        observedAt: now,
      },
      capabilities: [],
      createdAt: now,
      updatedAt: now,
    })

    const tx = await world.getEngine().newTransaction(true, signal)
    try {
      const objState = await tx.createObject(objectKey, {}, signal)
      try {
        const cursor = await objState.accessWorldState(undefined, signal)
        try {
          await cursor.putBlock(
            {
              data,
              markDirty: true,
              blockType: 'github.com/s4wave/spacewave/sdk/device.Device',
            },
            signal,
          )
        } finally {
          cursor.release()
        }
      } finally {
        objState.release()
      }
      await setObjectType(tx, objectKey, DeviceTypeID, signal)
      await tx.commit(signal)
    } finally {
      await tx.discard(signal).catch(() => undefined)
      tx.release()
    }

    return {
      action: 'seed-device',
      created: true,
      objectKey,
      typeId: DeviceTypeID,
      label,
      peerId,
    }
  })
}

export default async function deviceQuickstartHelper(args: ActionArgs) {
  const deadlineMs = args.deadlineMs ?? 120000
  return await withTimeout(deadlineMs, async (signal) => {
    switch (args.action) {
      case 'seed-device':
        return await seedDevice(args, signal)
      default:
        throw new Error(`unsupported action ${(args as { action?: string }).action}`)
    }
  })
}
