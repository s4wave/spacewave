import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import {
  Device,
  DeviceLiveness,
  DeviceSetupState,
  DeviceUpdateState,
} from '@s4wave/sdk/device/device.pb.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { createWorldObject } from '@s4wave/sdk/world/utils.js'

import { currentSpaceTarget, withMountedWorld } from './mounted-world.js'
import { withTimeout } from './test-utils.js'

// This helper is presentation-only. Lifecycle proofs must use real enrollment.
type ActionArgs = {
  action: 'seed-device'
  objectKey?: string
  label?: string
  peerId?: string
  deadlineMs?: number
}

async function seedDevice(args: ActionArgs, signal: AbortSignal) {
  const objectKey = args.objectKey ?? 'devices/e2e-build-host'
  const label = args.label ?? 'E2E Build Host'
  const peerId = args.peerId ?? '12D3KooWE2EDevice'
  return await withMountedWorld(
    currentSpaceTarget(),
    true,
    signal,
    async (world) => {
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

      using stage = await world.stageWorldState(signal)
      using cursor = await stage.buildStorageCursor(signal)
      const created = await createWorldObject(
        world,
        cursor,
        objectKey,
        (objectCursor) => objectCursor.setBlock({ data, markDirty: true }),
        signal,
      )
      try {
        await setObjectType(world, objectKey, DeviceTypeID, signal)
      } finally {
        created.objectState.release()
      }

      return {
        action: 'seed-device',
        created: true,
        objectKey,
        typeId: DeviceTypeID,
        label,
        peerId,
      }
    },
  )
}

export default async function deviceQuickstartHelper(args: ActionArgs) {
  const deadlineMs = args.deadlineMs ?? 120000
  return await withTimeout(deadlineMs, async (signal) => {
    switch (args.action) {
      case 'seed-device':
        return await seedDevice(args, signal)
      default:
        throw new Error(
          `unsupported action ${(args as { action?: string }).action}`,
        )
    }
  })
}
