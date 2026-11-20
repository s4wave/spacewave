import { useCallback, useState } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  deviceCapabilityCanWriteCheckoutRoot,
  type DeviceHandle,
} from '@s4wave/sdk/device/device.js'
import {
  DeviceAvailabilityState,
  type DeviceAvailability,
  type DeviceCapability,
} from '@s4wave/sdk/device/device.pb.js'

import { DeviceDriveView, type DeviceDriveBody } from './DeviceDriveView.js'
import { isBrowsableCheckoutRoot } from './checkout-root.js'

interface DeviceDriveProps {
  handle: Resource<DeviceHandle>
  device: Resource<DeviceAvailability>
  selection: DeviceCapability
  back: () => void
}

type DeviceDriveSource =
  | { kind: 'failed'; retry: () => void }
  | { kind: 'opening' }
  | { kind: 'ready'; value: DeviceAvailability }

/** driveSource follows the Device handle and its availability watch. */
function driveSource(
  handle: Resource<DeviceHandle>,
  device: Resource<DeviceAvailability>,
): DeviceDriveSource {
  if (handle.error || (!handle.loading && !handle.value)) {
    return { kind: 'failed', retry: handle.retry }
  }
  if (device.error || (!device.loading && !device.value?.device)) {
    return { kind: 'failed', retry: device.retry }
  }
  if (!handle.value || !device.value?.device) return { kind: 'opening' }
  return { kind: 'ready', value: device.value }
}

/** DeviceDrive follows the Device of the selected checkout root. */
export function DeviceDrive({
  handle,
  device,
  selection,
  back,
}: DeviceDriveProps) {
  // Retain the selected folder's name through handle failures and reopenings.
  const name = selection.checkoutRoot?.name || selection.label || 'Folder'
  const id = selection.id ?? name
  const source = driveSource(handle, device)
  if (source.kind === 'failed') {
    return (
      <DeviceDriveView
        name={name}
        id={id}
        body={{ kind: 'handle-failed', retry: source.retry }}
        back={back}
      />
    )
  }
  if (source.kind === 'opening') {
    return <DeviceDriveView name={name} id={id} body={source} back={back} />
  }

  // Use the latest declaration, including a changed path or access policy.
  const capability = source.value.device?.capabilities?.find(
    (cap) => cap.id === selection.id,
  )
  const link = capability?.link ?? selection.link
  const target = link?.protocolId || link?.objectKey || ''
  const online =
    (!link?.protocolId ||
      source.value.state === DeviceAvailabilityState.ONLINE) &&
    !device.loading
  return (
    <ConnectedDeviceDrive
      key={target}
      name={name}
      id={id}
      target={target}
      capability={capability}
      handle={handle}
      online={online}
      retryDevice={device.retry}
      back={back}
    />
  )
}

interface ConnectedDeviceDriveProps {
  name: string
  id: string
  target: string
  capability?: DeviceCapability
  handle: Resource<DeviceHandle>
  online: boolean
  retryDevice: () => void
  back: () => void
}

/** ConnectedDeviceDrive retains editing confirmation across Device disconnects. */
function ConnectedDeviceDrive({
  name,
  id,
  target,
  capability,
  handle,
  online,
  retryDevice,
  back,
}: ConnectedDeviceDriveProps) {
  // A connection uses the Device's access. Editing confirmation controls the viewer.
  const canWrite = deviceCapabilityCanWriteCheckoutRoot(capability)
  const canOpen = isBrowsableCheckoutRoot(capability)
  const root = useResource(
    handle,
    async (device, signal, cleanup) => {
      const result = await device.accessCheckoutRoot({
        name,
        write: canWrite,
        abortSignal: signal,
      })
      return cleanup(result.handle)
    },
    [name, target, canWrite],
    { enabled: online && canOpen },
  )
  const [confirmed, setConfirmed] = useState(false)
  const enableEditing = useCallback(() => setConfirmed(true), [])
  const editing = canWrite && confirmed

  // Project the current connection into the viewer's one visible state.
  let body: DeviceDriveBody
  if (!online) {
    body = { kind: 'offline' }
  } else if (!canOpen) {
    body = { kind: 'root-failed', retry: retryDevice }
  } else if (root.error) {
    body = {
      kind: 'root-failed',
      detail: root.error.message,
      retry: root.retry,
    }
  } else if (!root.value) {
    body = { kind: 'connecting' }
  } else {
    body = {
      kind: 'files',
      root,
      editing,
      enableEditing: canWrite && !confirmed ? enableEditing : undefined,
    }
  }
  return <DeviceDriveView name={name} id={id} body={body} back={back} />
}
