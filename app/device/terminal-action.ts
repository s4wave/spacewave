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

// buildTerminalOpData encodes one CreateTerminalOp for a resolved terminal
// target; labelFallback names the target kind when the source has no label.
function buildTerminalOpData({
  label,
  labelFallback,
  targetKind,
  devicePeerId,
  sshHostObjectKey,
  deviceObjectKey,
  command,
  existingObjectKeys,
}: {
  label: string
  labelFallback: string
  targetKind: (typeof TerminalTargetKind)[keyof typeof TerminalTargetKind]
  devicePeerId?: string
  sshHostObjectKey?: string
  deviceObjectKey?: string
  command?: string
  existingObjectKeys?: Iterable<string | undefined>
}): { objectKey: string; opData: Uint8Array } {
  const name = `${label || labelFallback} Terminal`
  const objectKey = buildObjectKey('terminal/', name, existingObjectKeys)
  return {
    objectKey,
    opData: CreateTerminalOp.toBinary({
      objectKey,
      name,
      devicePeerId,
      sshHostObjectKey,
      deviceObjectKey,
      targetKind,
      command,
      cols: 80,
      rows: 24,
      timestamp: new Date(),
    }),
  }
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
  return buildTerminalOpData({
    label: device.label ?? '',
    labelFallback: 'Device',
    targetKind: TerminalTargetKind.DEVICE,
    devicePeerId: device.peerId,
    deviceObjectKey,
    existingObjectKeys,
  })
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
  return buildTerminalOpData({
    label: host.label ?? '',
    labelFallback: 'SSH Host',
    targetKind: TerminalTargetKind.SSH_HOST,
    sshHostObjectKey: hostObjectKey,
    command,
    existingObjectKeys,
  })
}
