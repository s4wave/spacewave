import type { DeviceCapability } from '@s4wave/sdk/device/device.pb.js'
import {
  DeviceCapabilityKindFilesystem,
  deviceCheckoutRootCanRead,
  isDeviceCapabilitySelectable,
} from '@s4wave/sdk/device/device.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'

/**
 * isBrowsableCheckoutRoot reports whether the capability references a readable
 * folder.
 */
export function isBrowsableCheckoutRoot(
  capability?: DeviceCapability,
): boolean {
  const link = capability?.link
  return (
    capability?.kind === DeviceCapabilityKindFilesystem &&
    isDeviceCapabilitySelectable(capability) &&
    deviceCheckoutRootCanRead(capability.checkoutRoot) &&
    (!!link?.protocolId || (link?.typeId === UnixFSTypeID && !!link.objectKey))
  )
}
