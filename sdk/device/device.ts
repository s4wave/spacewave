import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { DeviceResourceServiceClient } from './device_srpc.pb.js'
import {
  DeviceCapabilityState,
  DeviceCheckoutRootAccess,
  DeviceCapabilityGrantState,
  DeviceCapabilityLocalState,
  DeviceSetupState,
  type Device,
  type DeviceAvailability,
  type AccessCheckoutRootRequest,
  type AccessCheckoutRootResponse,
  type DeviceCapability,
  type DeviceCheckoutRootCapability,
  type WatchDeviceStateResponse,
} from './device.pb.js'
import { FSHandle } from '../unixfs/handle.js'

// DeviceTypeID is the type identifier for Spacewave-managed Device objects.
export const DeviceTypeID = 'spacewave/device'

export const DeviceCapabilityKindFilesystem = 'filesystem'
export const DeviceCapabilityKindForgeWorker = 'forge-worker'
export const DeviceCapabilityKindRemoteShell = 'remote-shell'
export const DeviceCapabilityKindCamera = 'camera'

/** DeviceCapabilityKindDisplay identifies a display track with input. */
export const DeviceCapabilityKindDisplay = 'display'

// isDeviceSelectable reports whether a Device can be presented as a target by
// Forge or a workflow builder.
export function isDeviceSelectable(device: Device | null | undefined): boolean {
  return (
    (device?.peerId ?? '').trim() !== '' &&
    (device?.label ?? '').trim() !== '' &&
    device?.setupState === DeviceSetupState.DEVICE_SESSION_READY
  )
}

export function findDeviceCapabilityByKind(
  device: Device | null | undefined,
  kind: string,
): DeviceCapability | undefined {
  return (device?.capabilities ?? []).find(
    (capability) => (capability.kind ?? '').trim() === kind,
  )
}

export function isDeviceCapabilitySelectable(
  capability: DeviceCapability | null | undefined,
): boolean {
  return (
    capability?.state === DeviceCapabilityState.AVAILABLE ||
    capability?.state === DeviceCapabilityState.ACTIVE
  )
}

export function isDeviceCapabilityPolicyWriteAllowed(
  capability: DeviceCapability | null | undefined,
): boolean {
  return (
    capability?.policy?.localState === DeviceCapabilityLocalState.ENABLED &&
    capability?.policy?.grantState === DeviceCapabilityGrantState.ALLOWED
  )
}

export function hasSelectableDeviceCapabilityKind(
  device: Device | null | undefined,
  kind: string,
): boolean {
  return (device?.capabilities ?? []).some(
    (capability) =>
      (capability.kind ?? '').trim() === kind &&
      isDeviceCapabilitySelectable(capability),
  )
}

export function findSelectableDeviceCheckoutRoot(
  device: Device | null | undefined,
  name?: string,
): DeviceCapability | undefined {
  const selector = name?.trim() ?? ''
  return (device?.capabilities ?? []).find((capability) => {
    const checkoutRoot = capability.checkoutRoot
    if (!checkoutRoot) return false
    if ((capability.kind ?? '').trim() !== DeviceCapabilityKindFilesystem) {
      return false
    }
    if (!isDeviceCapabilitySelectable(capability)) return false
    return !selector || (checkoutRoot.name ?? '').trim() === selector
  })
}

export function deviceCheckoutRootCanRead(
  checkoutRoot: DeviceCheckoutRootCapability | null | undefined,
): boolean {
  return (
    !!checkoutRoot?.readAvailable &&
    (checkoutRoot.access === DeviceCheckoutRootAccess.READ_ONLY ||
      checkoutRoot.access === DeviceCheckoutRootAccess.READ_WRITE)
  )
}

export function deviceCheckoutRootCanWrite(
  checkoutRoot: DeviceCheckoutRootCapability | null | undefined,
): boolean {
  return (
    checkoutRoot?.access === DeviceCheckoutRootAccess.READ_WRITE &&
    !!checkoutRoot.readAvailable &&
    !!checkoutRoot.writeAvailable
  )
}

export function deviceCapabilityCanWriteCheckoutRoot(
  capability: DeviceCapability | null | undefined,
): boolean {
  return (
    deviceCheckoutRootCanWrite(capability?.checkoutRoot) &&
    isDeviceCapabilityPolicyWriteAllowed(capability)
  )
}

/** hasCheckoutRootLink reports whether the root names a Device protocol or a World object. */
function hasCheckoutRootLink(
  capability: DeviceCapability | undefined,
): boolean {
  const link = capability?.link
  return (
    !!link?.protocolId?.trim() ||
    (!!link?.objectKey?.trim() && !!link?.typeId?.trim())
  )
}

/** findReadableDeviceCheckoutRoot returns a selectable root linked to a filesystem. */
export function findReadableDeviceCheckoutRoot(
  device: Device | null | undefined,
  name?: string,
): DeviceCapability | undefined {
  const capability = findSelectableDeviceCheckoutRoot(device, name)
  if (
    deviceCheckoutRootCanRead(capability?.checkoutRoot) &&
    hasCheckoutRootLink(capability)
  )
    return capability
}

/** findWritableDeviceCheckoutRoot returns a selectable root whose policy allows writes. */
export function findWritableDeviceCheckoutRoot(
  device: Device | null | undefined,
  name?: string,
): DeviceCapability | undefined {
  const capability = findSelectableDeviceCheckoutRoot(device, name)
  if (
    deviceCapabilityCanWriteCheckoutRoot(capability) &&
    hasCheckoutRootLink(capability)
  )
    return capability
}

export function findSelectableDeviceForgeWorker(
  device: Device | null | undefined,
): DeviceCapability | undefined {
  return (device?.capabilities ?? []).find((capability) => {
    if ((capability.kind ?? '').trim() !== DeviceCapabilityKindForgeWorker) {
      return false
    }
    if (!isDeviceCapabilitySelectable(capability)) return false
    const objectKey = capability.link?.objectKey?.trim() ?? ''
    const typeId = capability.link?.typeId?.trim() ?? ''
    return !!objectKey && !!typeId
  })
}

/** AccessCheckoutRootOptions selects the root and the requested access mode. */
export interface AccessCheckoutRootOptions {
  name?: string
  write?: boolean
  abortSignal?: AbortSignal
}

// IDeviceHandle contains the DeviceHandle interface.
export interface IDeviceHandle {
  /** watchDeviceAvailability streams durable Device and live transport changes. */
  watchDeviceAvailability(
    abortSignal?: AbortSignal,
  ): AsyncIterable<DeviceAvailability>

  // watchDeviceState streams Device state changes.
  watchDeviceState(abortSignal?: AbortSignal): AsyncIterable<Device | undefined>

  // accessCheckoutRoot opens a selected checkout root as a filesystem handle.
  accessCheckoutRoot(
    nameOrOptions?: string | AccessCheckoutRootOptions,
    abortSignal?: AbortSignal,
  ): Promise<{ handle: FSHandle; response: AccessCheckoutRootResponse }>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// DeviceHandle represents a handle to a Device resource.
export class DeviceHandle extends Resource implements IDeviceHandle {
  private service: DeviceResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new DeviceResourceServiceClient(resourceRef.client)
  }

  /** watchDeviceAvailability observes reachability without durable status writes. */
  public watchDeviceAvailability(
    abortSignal?: AbortSignal,
  ): AsyncIterable<DeviceAvailability> {
    return this.service.WatchDeviceAvailability({}, abortSignal)
  }

  // watchDeviceState streams Device state changes.
  public async *watchDeviceState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Device | undefined> {
    const stream = this.service.WatchDeviceState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchDeviceStateResponse>) {
      yield resp.state
    }
  }

  // accessCheckoutRoot opens a selected checkout root as a filesystem handle.
  public async accessCheckoutRoot(
    nameOrOptions?: string | AccessCheckoutRootOptions,
    abortSignal?: AbortSignal,
  ): Promise<{ handle: FSHandle; response: AccessCheckoutRootResponse }> {
    const request: AccessCheckoutRootRequest =
      typeof nameOrOptions === 'object'
        ? {
            name: nameOrOptions.name ?? '',
            write: nameOrOptions.write ?? false,
          }
        : { name: nameOrOptions ?? '' }
    const signal =
      typeof nameOrOptions === 'object'
        ? nameOrOptions.abortSignal
        : abortSignal
    const response = await this.service.AccessCheckoutRoot(request, signal)
    return {
      handle: this.resourceRef.createResource(
        response.resourceId ?? 0,
        FSHandle,
        { path: '' },
      ),
      response,
    }
  }
}
