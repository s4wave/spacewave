import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  type Device,
  type DeviceAvailability,
  DeviceAvailabilityState,
} from '@s4wave/sdk/device/device.pb.js'
import type { ViewerStatusProps } from '@s4wave/web/object/ViewerStatus.js'

/** FrameObjectState is the handle and Device state that selects the viewer body. */
export type FrameObjectState =
  | { body: 'status'; status: ViewerStatusProps }
  | { body: 'offline' | 'picture'; name?: string; device?: Device }

/** frameObjectState selects the body from the handle and Device watches. */
export function frameObjectState<H, W>(
  handle: Resource<H>,
  watch: Resource<W>,
  kind: string,
  object: { name?: string } | undefined,
  availability: DeviceAvailability | undefined,
): FrameObjectState {
  // Show operation failures and their retry actions while the handle opens.
  if (handle.error) {
    return {
      body: 'status',
      status: {
        state: 'failed',
        message: `Could not open the ${kind}: ${handle.error.message}`,
        retry: handle.retry,
      },
    }
  }
  if (watch.error) {
    return {
      body: 'status',
      status: {
        state: 'failed',
        message: `Could not read the ${kind}: ${watch.error.message}`,
        retry: watch.retry,
      },
    }
  }
  if (!handle.value || !object) {
    return {
      body: 'status',
      status: { state: 'waiting', what: `the ${kind}`, rows: 0 },
    }
  }

  // Mount a frame track only while the Device is online.
  const body =
    availability?.state === DeviceAvailabilityState.ONLINE
      ? 'picture'
      : 'offline'
  return { body, name: object.name, device: availability?.device }
}
