import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { ViewerBar } from '@s4wave/web/object/ViewerBar.js'
import {
  resourceStatus,
  ViewerStatus,
  type ViewerStatusProps,
} from '@s4wave/web/object/ViewerStatus.js'
import { ViewerRailGlyph, ViewerSide } from '@s4wave/web/object/ViewerSide.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'
import { useCallback } from 'react'
import { LuServer, LuTerminal, LuTriangleAlert } from 'react-icons/lu'

import {
  type Device,
  DeviceAvailabilityState,
  DeviceSetupState,
  DeviceUpdateState,
} from '@s4wave/sdk/device/device.pb.js'
import { DeviceHandle, DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'

import { DeviceCapabilityTable } from './DeviceCapabilityTable.js'
import {
  buildCreateTerminalOpData,
  findOpenableTerminalCapability,
} from './terminal-action.js'

export { DeviceTypeID }

export function DeviceViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the Device through the current Space's typed resource grant.
  const deviceObjectKey = getObjectKey(objectInfo)
  const { navigateToObjects, spaceWorld } = SpaceContainerContext.useContext()
  const handle = useAccessTypedHandle(
    worldState,
    deviceObjectKey,
    DeviceHandle,
    DeviceTypeID,
  )

  // Observe transport reachability and the durable record through one projection.
  const streamFactory = useCallback(
    (h: DeviceHandle, signal: AbortSignal) => h.watchDeviceAvailability(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state = stateResource.value?.device
  const capabilities = state?.capabilities ?? []
  const terminalCapability = findOpenableTerminalCapability(state)

  // Open a Terminal from the Device's declared capability.
  const handleOpenTerminal = async () => {
    if (!state || !terminalCapability) return
    try {
      const terminalOp = await buildCreateTerminalOpData({
        device: state,
        deviceObjectKey,
        world: spaceWorld,
      })
      if (!terminalOp) return
      await spaceWorld.applyWorldOp(
        CREATE_TERMINAL_OP_ID,
        terminalOp.opData,
        '',
      )
      navigateToObjects([terminalOp.objectKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open terminal',
      )
    }
  }

  // The device's facts, listed by the side column and the Details popover.
  const platform = [state?.platform?.os, state?.platform?.arch]
    .filter(Boolean)
    .join(', ')
  const updateFailed = state?.updateState === DeviceUpdateState.FAILED
  const statusError = state?.lastStatus?.error
  const facts = state ? (
    <DeviceFacts
      device={state}
      liveness={stateResource.value?.state}
      platform={platform}
    />
  ) : null

  // Until the device arrives, show what is read or why it failed; a stream
  // that ends without a device offers to read it again.
  const status: ViewerStatusProps | null = state
    ? null
    : (resourceStatus('the device', stateResource) ?? {
        state: 'failed',
        message: 'The Device returned no details.',
        retry: stateResource.retry,
      })

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <ViewerBar
        icon={<LuServer />}
        title={state?.label || 'Device'}
        description={platform || undefined}
        details={facts ? { label: 'Device', facts, narrow: true } : undefined}
        actions={
          terminalCapability
            ? [
                {
                  id: 'terminal',
                  label: 'Open Terminal',
                  icon: <LuTerminal />,
                  onClick: () => void handleOpenTerminal(),
                },
              ]
            : []
        }
      />
      {statusError ? (
        <div
          role="alert"
          className="border-foreground/8 text-destructive flex shrink-0 items-center gap-2 border-b px-2 py-1 text-xs [&_svg]:size-3.5"
        >
          <LuTriangleAlert aria-hidden className="shrink-0" />
          <span className="min-w-0">{statusError}</span>
        </div>
      ) : null}
      {status ? <ViewerStatus {...status} /> : null}
      {state && (
        <ViewerSide
          label="Device"
          facts={facts}
          rail={
            updateFailed ? (
              <ViewerRailGlyph
                label="Update failed"
                icon={<LuTriangleAlert className="text-destructive" />}
              />
            ) : null
          }
        >
          <DeviceCapabilityTable
            capabilities={capabilities}
            onOpenTerminal={
              terminalCapability ? () => void handleOpenTerminal() : undefined
            }
          />
        </ViewerSide>
      )}
    </div>
  )
}

// DeviceFacts lists a device's facts as FactRows, for the side column and
// the Details popover. A missing value shows as a text placeholder.
function DeviceFacts({
  device,
  liveness,
  platform,
}: {
  device: Device
  liveness?: DeviceAvailabilityState
  platform: string
}) {
  return (
    <>
      <FactRow
        label="Liveness"
        value={formatLiveness(liveness)}
        kind="text"
        control={null}
      />
      <FactRow
        label="Setup"
        value={formatSetupState(device.setupState)}
        kind="text"
        control={null}
      />
      <FactRow
        label="Update"
        value={formatUpdateState(device.updateState)}
        kind="text"
        control={null}
      />
      {device.peerId ? (
        <FactRow label="Peer ID" value={device.peerId} />
      ) : (
        <FactRow label="Peer ID" value="Unknown" kind="text" control={null} />
      )}
      <FactRow
        label="Platform"
        value={platform || 'Unknown'}
        kind="text"
        control={null}
      />
      {device.daemonVersion ? (
        <FactRow label="Version" value={device.daemonVersion} kind="text" />
      ) : null}
      <FactRow
        label="Status"
        value={device.lastStatus?.message || 'No status reported'}
        kind="text"
        control={null}
      />
    </>
  )
}

function formatSetupState(state?: DeviceSetupState): string {
  switch (state) {
    case DeviceSetupState.WAITING_FOR_COMPLETION:
      return 'Waiting'
    case DeviceSetupState.COMPLETION_IMPORTED:
      return 'Imported'
    case DeviceSetupState.DEVICE_SESSION_READY:
      return 'Ready'
    case DeviceSetupState.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function formatUpdateState(state?: DeviceUpdateState): string {
  switch (state) {
    case DeviceUpdateState.IDLE:
      return 'Idle'
    case DeviceUpdateState.READY:
      return 'Ready'
    case DeviceUpdateState.STAGING:
      return 'Staging'
    case DeviceUpdateState.APPLYING:
      return 'Applying'
    case DeviceUpdateState.UPDATED:
      return 'Updated'
    case DeviceUpdateState.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function formatLiveness(state?: DeviceAvailabilityState): string {
  switch (state) {
    case DeviceAvailabilityState.ONLINE:
      return 'Online'
    case DeviceAvailabilityState.OFFLINE:
      return 'Offline'
    default:
      return 'Unknown'
  }
}
