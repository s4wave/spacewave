import {
  type DeviceCapability,
  DeviceCapabilityState,
} from '@s4wave/sdk/device/device.pb.js'
import { useParams } from '@s4wave/web/router/router.js'

import { DeviceCapabilityTable } from '@s4wave/app/device/DeviceCapabilityTable.js'

// declared are a workstation's capabilities in each state the table names,
// with details long enough to shed.
const declared: DeviceCapability[] = [
  {
    id: 'terminal',
    label: 'Terminal',
    state: DeviceCapabilityState.ACTIVE,
    detail: 'One session open',
  },
  {
    id: 'forge-worker',
    label: 'Forge worker',
    state: DeviceCapabilityState.AVAILABLE,
    detail: '8 cores, 16 GB declared',
  },
  {
    id: 'vm-v86',
    label: 'VM (v86)',
    state: DeviceCapabilityState.DECLARED,
  },
  {
    id: 'files',
    label: 'Files',
    state: DeviceCapabilityState.GRANT_BLOCKED,
    detail: 'blocked by Space grant',
  },
  {
    id: 'gpu',
    label: 'GPU compute',
    state: DeviceCapabilityState.DISABLED,
    detail: 'disabled on this device',
  },
]

/**
 * DeviceCapabilityDebug renders the Device viewer's capability table in one
 * fixture state, with the viewport as the pane, for the viewer sweep: list
 * (five capabilities) and empty (none declared). An unknown or missing state
 * shows the list.
 */
export function DeviceCapabilityDebug() {
  // Read the fixture state from the route.
  const { state } = useParams()
  const capabilities = state === 'empty' ? [] : declared

  // The table sits where the Device viewer puts it, beside its side column.
  return (
    <div className="bg-background-primary viewer-pane flex h-full w-full flex-col">
      <DeviceCapabilityTable capabilities={capabilities} />
    </div>
  )
}
