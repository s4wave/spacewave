import { CreateCameraOpTypeID } from '@s4wave/sdk/camera/camera.js'
import { CreateCameraOp } from '@s4wave/sdk/camera/camera.pb.js'
import type { Device, DeviceCapability } from '@s4wave/sdk/device/device.pb.js'
import {
  DeviceCapabilityKindCamera,
  DeviceCapabilityKindDisplay,
  isDeviceCapabilitySelectable,
} from '@s4wave/sdk/device/device.js'
import { CreateDisplayOp } from '@s4wave/sdk/display/display.pb.js'
import { CreateDisplayOpTypeID } from '@s4wave/sdk/display/display.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { buildObjectKey } from '../space/create-op-builders.js'

/** frameActions maps frame capabilities to the object they open. */
const frameActions = {
  [DeviceCapabilityKindCamera]: {
    label: 'Camera',
    typename: CreateCameraOpTypeID,
    prefix: 'camera/',
    codec: CreateCameraOp,
  },
  [DeviceCapabilityKindDisplay]: {
    label: 'Display',
    typename: CreateDisplayOpTypeID,
    prefix: 'display/',
    codec: CreateDisplayOp,
  },
}

/** getFrameAction returns the object a selectable frame capability opens. */
export function getFrameAction(capability: DeviceCapability) {
  if (!isDeviceCapabilitySelectable(capability)) return undefined
  switch (capability.kind) {
    case DeviceCapabilityKindCamera:
    case DeviceCapabilityKindDisplay:
      return frameActions[capability.kind]
  }
}

/** buildCreateFrameOpData names the Device object and its served capability. */
export async function buildCreateFrameOpData({
  device,
  deviceObjectKey,
  capability,
  world,
}: {
  device: Device
  deviceObjectKey: string
  capability: DeviceCapability
  world: IWorldState
}) {
  // Require an action on a capability with an identity on this Device.
  const action = getFrameAction(capability)
  if (!action || !deviceObjectKey || !capability.id) return undefined

  // Encode the create operation at a free key in the current Space.
  const name = capability.label || `${device.label || 'Device'} ${action.label}`
  const objectKey = await buildObjectKey(world, action.prefix, name)
  return {
    typename: action.typename,
    objectKey,
    opData: action.codec.toBinary({
      objectKey,
      name,
      deviceObjectKey,
      capabilityId: capability.id,
      timestamp: new Date(),
    }),
  }
}
