import { useCallback, useMemo } from 'react'
import { LuFolderOpen } from 'react-icons/lu'

import {
  type DeviceCapability,
  DeviceCapabilityState,
} from '@s4wave/sdk/device/device.pb.js'
import { type ListItem, useListState } from '@s4wave/web/ui/list/index.js'
import { Table, type TableColumn } from '@s4wave/web/ui/table/Table.js'

import { isBrowsableCheckoutRoot } from './checkout-root.js'

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
// reads from its name to its state across a wide pane. Opening rows calls
// onOpenCapability with the first opened capability, and the caller opens
// what that capability's kind serves. Rows the caller cannot open only select.
export function DeviceCapabilityTable({
  capabilities,
  onOpenCapability,
  onBrowse,
}: {
  capabilities: DeviceCapability[]
  onOpenCapability?: (capability: DeviceCapability) => void
  onBrowse?: (capability: DeviceCapability) => void
}) {
  // Keep the table's selection and sort with the viewer.
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

  // Keep the table cells stable while capability states change.
  const columns = useMemo(() => browseColumns(onBrowse), [onBrowse])

  // Opening rows hands the first opened capability to the caller.
  const handleOpen = useCallback(
    (opened: ListItem<DeviceCapability>[]) => {
      const capability = opened.find((row) => row.data)?.data
      if (capability) onOpenCapability?.(capability)
    },
    [onOpenCapability],
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
        columns={columns}
        state={state}
        onOpen={handleOpen}
        label="Capabilities"
        className="min-h-0 max-w-120 flex-1"
        placeholder={<span className="text-xs">No capabilities declared</span>}
      />
    </>
  )
}

/** browseColumns adds the Browse press to a readable checkout root's name. */
function browseColumns(
  onBrowse?: (capability: DeviceCapability) => void,
): TableColumn<DeviceCapability>[] {
  const [first, ...rest] = capabilityColumns
  return [
    {
      ...first,
      cell: (capability: DeviceCapability) => (
        <>
          <span
            className="min-w-0 flex-1 truncate"
            title={first.text(capability)}
          >
            {first.text(capability)}
          </span>
          {onBrowse && isBrowsableCheckoutRoot(capability) && (
            <button
              type="button"
              className="focus-ring h-viewer-press w-viewer-press hover:bg-foreground/5 @pane-md:w-auto flex shrink-0 items-center justify-center gap-1 rounded px-1 text-xs pointer-coarse:min-h-11 pointer-coarse:min-w-11"
              aria-label={`Browse ${capability.checkoutRoot?.name || capability.label || 'folder'}`}
              title="Browse"
              onClick={(event) => {
                event.stopPropagation()
                onBrowse(capability)
              }}
            >
              <LuFolderOpen aria-hidden className="size-3.5" />
              <span aria-hidden className="@pane-md:inline hidden">
                Browse
              </span>
            </button>
          )}
        </>
      ),
    },
    ...rest,
  ]
}

// formatCapabilityState formats a capability state as the table shows it.
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
