import { CreateCameraOp } from '@s4wave/sdk/camera/camera.pb.js'
import type { Device, DeviceCapability } from '@s4wave/sdk/device/device.pb.js'
import {
  DeviceCapabilityKindCamera,
  isDeviceCapabilitySelectable,
} from '@s4wave/sdk/device/device.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { buildObjectKey } from '../space/create-op-builders.js'

// CameraOpData is an encoded CreateCameraOp and the key it creates.
export interface CameraOpData {
  objectKey: string
  opData: Uint8Array
}

// findOpenableCameraCapabilities returns the Device's camera capabilities a
// Camera can open on now.
export function findOpenableCameraCapabilities(
  device?: Device,
): DeviceCapability[] {
  return (device?.capabilities ?? []).filter(isOpenableCameraCapability)
}

// isOpenableCameraCapability reports whether capability is a camera that is
// available or active, the states a Camera can open in.
export function isOpenableCameraCapability(
  capability: Pick<DeviceCapability, 'kind' | 'state'>,
): boolean {
  return (
    capability.kind === DeviceCapabilityKindCamera &&
    isDeviceCapabilitySelectable(capability)
  )
}

// buildCreateCameraOpData encodes one CreateCameraOp for capability on the
// Device at deviceObjectKey, at a free key read from world. The Camera names
// the Device object and the capability, so it follows the Device's peer.
export async function buildCreateCameraOpData({
  device,
  deviceObjectKey,
  capability,
  world,
}: {
  device: Device
  deviceObjectKey: string
  capability: DeviceCapability
  world: IWorldState
}): Promise<CameraOpData | undefined> {
  if (!deviceObjectKey || !capability.id) return undefined
  const name = capability.label || `${device.label || 'Device'} Camera`
  const objectKey = await buildObjectKey(world, 'camera/', name)
  return {
    objectKey,
    opData: CreateCameraOp.toBinary({
      objectKey,
      name,
      deviceObjectKey,
      capabilityId: capability.id,
      timestamp: new Date(),
    }),
  }
}
