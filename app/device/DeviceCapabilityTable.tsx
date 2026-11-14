import { useCallback, useMemo } from 'react'

import {
  type DeviceCapability,
  DeviceCapabilityState,
} from '@s4wave/sdk/device/device.pb.js'
import { type ListItem, useListState } from '@s4wave/web/ui/list'
import { Table, type TableColumn } from '@s4wave/web/ui/table/Table.js'

import { isOpenableTerminalCapability } from './terminal-action.js'

// capabilityColumns are the capability table's columns: the capability, its
// state, which shows from 16rem, and its detail, which sheds first.
const capabilityColumns: TableColumn<DeviceCapability>[] = [
  {
    key: 'capability',
    label: 'Capability',
    text: (capability) => capability.label || capability.id || '',
  },
  {
    key: 'state',
    label: 'State',
    minWidth: 5.5,
    text: (capability) => formatCapabilityState(capability.state),
  },
  {
    key: 'detail',
    label: 'Detail',
    minWidth: 8,
    text: (capability) => capability.detail ?? '',
  },
]

// DeviceCapabilityTable lists a device's declared capabilities under a
// section row with their count, as a table that stops at 30rem so a row
// reads from its name to its state across a wide pane. Opening an openable
// terminal capability's row calls onOpenTerminal; other rows only select.
export function DeviceCapabilityTable({
  capabilities,
  onOpenTerminal,
}: {
  capabilities: DeviceCapability[]
  onOpenTerminal?: () => void
}) {
  // The table's selection and sort, kept with the viewer.
  const state = useListState(null, 'deviceCapabilities', {
    sortKey: 'capability',
    sortDirection: 'asc',
  })

  // Each capability is a row keyed by its stable id.
  const rows = useMemo<ListItem<DeviceCapability>[]>(
    () =>
      capabilities.map((capability, index) => ({
        id: capability.id || String(index),
        data: capability,
      })),
    [capabilities],
  )

  // Opening rows opens the terminal when one of them can open it.
  const handleOpen = useCallback(
    (opened: ListItem<DeviceCapability>[]) => {
      if (
        opened.some((row) => row.data && isOpenableTerminalCapability(row.data))
      ) {
        onOpenTerminal?.()
      }
    },
    [onOpenTerminal],
  )

  // The section row takes the table header's 14px inset, so its label
  // starts on the same edge as the Capability header below it.
  return (
    <>
      <div className="h-viewer-row text-foreground-alt flex shrink-0 items-center gap-2 px-3.5 text-xs font-medium select-none">
        Capabilities
        <span className="text-foreground-alt/70 font-normal">
          {capabilities.length}
        </span>
      </div>
      <Table
        rows={rows}
        columns={capabilityColumns}
        state={state}
        onOpen={handleOpen}
        label="Capabilities"
        className="min-h-0 max-w-120 flex-1"
        placeholder={<span className="text-xs">No capabilities declared</span>}
      />
    </>
  )
}

// formatCapabilityState names a capability state as the table shows it.
function formatCapabilityState(state?: DeviceCapabilityState): string {
  switch (state) {
    case DeviceCapabilityState.DECLARED:
      return 'Declared'
    case DeviceCapabilityState.DISABLED:
      return 'Disabled'
    case DeviceCapabilityState.GRANT_BLOCKED:
      return 'Grant blocked'
    case DeviceCapabilityState.AVAILABLE:
      return 'Available'
    case DeviceCapabilityState.ACTIVE:
      return 'Active'
    case DeviceCapabilityState.REJECTED:
      return 'Rejected'
    default:
      return 'Unknown'
  }
}
