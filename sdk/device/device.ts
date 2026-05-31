import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { DeviceResourceServiceClient } from './device_srpc.pb.js'
import type { Device, WatchDeviceStateResponse } from './device.pb.js'

// DeviceTypeID is the type identifier for Spacewave-managed Device objects.
export const DeviceTypeID = 'spacewave/device'

// IDeviceHandle contains the DeviceHandle interface.
export interface IDeviceHandle {
  // watchDeviceState streams Device state changes.
  watchDeviceState(abortSignal?: AbortSignal): AsyncIterable<Device | undefined>

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

  // watchDeviceState streams Device state changes.
  public async *watchDeviceState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Device | undefined> {
    const stream = this.service.WatchDeviceState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchDeviceStateResponse>) {
      yield resp.state
    }
  }
}
