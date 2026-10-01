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
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { buildObjectKey } from '../space/create-op-builders.js'

// TerminalOpData is an encoded CreateTerminalOp and the key it creates.
export interface TerminalOpData {
  objectKey: string
  opData: Uint8Array
}

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
// target, named with a free key read from world; labelFallback names the
// target kind when the source has no label.
async function buildTerminalOpData({
  label,
  labelFallback,
  targetKind,
  devicePeerId,
  sshHostObjectKey,
  deviceObjectKey,
  command,
  world,
}: {
  label: string
  labelFallback: string
  targetKind: (typeof TerminalTargetKind)[keyof typeof TerminalTargetKind]
  devicePeerId?: string
  sshHostObjectKey?: string
  deviceObjectKey?: string
  command?: string
  world: IWorldState
}): Promise<TerminalOpData> {
  const name = `${label || labelFallback} Terminal`
  const objectKey = await buildObjectKey(world, 'terminal/', name)
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

export async function buildCreateTerminalOpData({
  device,
  deviceObjectKey,
  world,
}: {
  device: Device
  deviceObjectKey: string
  world: IWorldState
}): Promise<TerminalOpData | undefined> {
  if (!device.peerId) return undefined
  return buildTerminalOpData({
    label: device.label ?? '',
    labelFallback: 'Device',
    targetKind: TerminalTargetKind.DEVICE,
    devicePeerId: device.peerId,
    deviceObjectKey,
    world,
  })
}

export async function buildCreateSshHostTerminalOpData({
  host,
  hostObjectKey,
  world,
  command,
}: {
  host: SshHost
  hostObjectKey: string
  world: IWorldState
  command?: string
}): Promise<TerminalOpData | undefined> {
  if (!hostObjectKey) return undefined
  return buildTerminalOpData({
    label: host.label ?? '',
    labelFallback: 'SSH Host',
    targetKind: TerminalTargetKind.SSH_HOST,
    sshHostObjectKey: hostObjectKey,
    command,
    world,
  })
}
