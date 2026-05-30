import {
  DeviceCapabilityState,
  type Device,
  type DeviceCapability,
} from '@s4wave/sdk/device/device.pb.js'
import type { SshHost } from '@s4wave/sdk/sshhost/sshhost.pb.js'
import {
  CreateTerminalOp,
  TerminalTargetKind,
} from '@s4wave/sdk/terminal/terminal.pb.js'

import { buildObjectKey } from '../space/create-op-builders.js'

export function findOpenableTerminalCapability(
  device?: Device,
): DeviceCapability | undefined {
  return (device?.capabilities ?? []).find(isOpenableTerminalCapability)
}

export function isOpenableTerminalCapability(
  capability: Pick<DeviceCapability, 'id' | 'kind' | 'state'>,
): boolean {
  if (capability.kind !== 'terminal' && capability.id !== 'terminal') {
    return false
  }
  return (
    capability.state === DeviceCapabilityState.AVAILABLE ||
    capability.state === DeviceCapabilityState.ACTIVE
  )
}

export function buildCreateTerminalOpData({
  device,
  deviceObjectKey,
  existingObjectKeys,
}: {
  device: Device
  deviceObjectKey: string
  existingObjectKeys?: Iterable<string | undefined>
}): { objectKey: string; opData: Uint8Array } | undefined {
  if (!device.peerId) return undefined
  const name = `${device.label || 'Device'} Terminal`
  const objectKey = buildObjectKey('terminal/', name, existingObjectKeys)
  return {
    objectKey,
    opData: CreateTerminalOp.toBinary({
      objectKey,
      name,
      deviceObjectKey,
      devicePeerId: device.peerId,
      targetKind: TerminalTargetKind.DEVICE,
      cols: 80,
      rows: 24,
      timestamp: new Date(),
    }),
  }
}

export function buildCreateSshHostTerminalOpData({
  host,
  hostObjectKey,
  existingObjectKeys,
  command,
}: {
  host: SshHost
  hostObjectKey: string
  existingObjectKeys?: Iterable<string | undefined>
  command?: string
}): { objectKey: string; opData: Uint8Array } | undefined {
  if (!hostObjectKey) return undefined
  const name = `${host.label || 'SSH Host'} Terminal`
  const objectKey = buildObjectKey('terminal/', name, existingObjectKeys)
  return {
    objectKey,
    opData: CreateTerminalOp.toBinary({
      objectKey,
      name,
      sshHostObjectKey: hostObjectKey,
      targetKind: TerminalTargetKind.SSH_HOST,
      command,
      cols: 80,
      rows: 24,
      timestamp: new Date(),
    }),
  }
}
