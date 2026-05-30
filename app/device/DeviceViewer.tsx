import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { useCallback } from 'react'

import {
  DeviceCapabilityState,
  DeviceLiveness,
  DeviceSetupState,
  DeviceUpdateState,
  type Device,
} from '@s4wave/sdk/device/device.pb.js'
import { DeviceHandle, DeviceTypeID } from '@s4wave/sdk/device/device.js'

export { DeviceTypeID }

export function DeviceViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    DeviceHandle,
    DeviceTypeID,
  )

  const streamFactory = useCallback(
    (h: DeviceHandle, signal: AbortSignal) => h.watchDeviceState(signal),
    [],
  )

  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Device | undefined = stateResource.value ?? undefined
  const capabilities = state?.capabilities ?? []

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center border-b px-4">
        <span className="text-foreground text-sm font-semibold select-none">
          Device
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-4">
        {stateResource.loading && !state && (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading device',
              detail: 'Reading the device state stream.',
            }}
          />
        )}
        {state && (
          <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
            <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <DeviceField label="Name" value={state.label || 'Device'} />
              <DeviceField
                label="Liveness"
                value={formatLiveness(state.lastStatus?.liveness)}
              />
              <DeviceField
                label="Setup"
                value={formatSetupState(state.setupState)}
              />
              <DeviceField
                label="Update"
                value={formatUpdateState(state.updateState)}
              />
            </section>

            <section className="grid gap-3 lg:grid-cols-2">
              <div className="border-foreground/10 bg-background-secondary rounded-md border p-3">
                <div className="text-muted-foreground text-xs font-medium uppercase">
                  Identity
                </div>
                <div className="text-foreground mt-2 font-mono text-xs break-all">
                  {state.peerId || 'unknown'}
                </div>
                <div className="text-muted-foreground mt-3 text-sm">
                  {[state.platform?.os, state.platform?.arch]
                    .filter(Boolean)
                    .join(' / ') || 'unknown platform'}
                </div>
                {state.daemonVersion && (
                  <div className="text-muted-foreground mt-1 text-sm">
                    {state.daemonVersion}
                  </div>
                )}
              </div>

              <div className="border-foreground/10 bg-background-secondary rounded-md border p-3">
                <div className="text-muted-foreground text-xs font-medium uppercase">
                  Last Status
                </div>
                <div className="text-foreground mt-2 text-sm">
                  {state.lastStatus?.message || 'No status reported'}
                </div>
                {state.lastStatus?.error && (
                  <div className="text-destructive mt-2 text-sm">
                    {state.lastStatus.error}
                  </div>
                )}
              </div>
            </section>

            <section>
              <div className="text-muted-foreground mb-2 text-xs font-medium uppercase">
                Capabilities
              </div>
              {capabilities.length === 0 ? (
                <div className="border-foreground/10 text-muted-foreground rounded-md border px-3 py-2 text-sm">
                  No capabilities declared
                </div>
              ) : (
                <div className="divide-foreground/8 border-foreground/10 overflow-hidden rounded-md border">
                  {capabilities.map((capability) => (
                    <div
                      key={capability.id}
                      className="bg-background-secondary grid gap-2 p-3 sm:grid-cols-[1fr_auto]"
                    >
                      <div>
                        <div className="text-foreground text-sm font-medium">
                          {capability.label || capability.id}
                        </div>
                        <div className="text-muted-foreground mt-1 text-xs">
                          {capability.kind || capability.id}
                        </div>
                      </div>
                      <div className="text-muted-foreground text-sm">
                        {formatCapabilityState(capability.state)}
                      </div>
                      {capability.detail && (
                        <div className="text-muted-foreground text-sm sm:col-span-2">
                          {capability.detail}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>
        )}
      </div>
    </div>
  )
}

function DeviceField({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-foreground/10 bg-background-secondary rounded-md border p-3">
      <div className="text-muted-foreground text-xs font-medium uppercase">
        {label}
      </div>
      <div className="text-foreground mt-2 text-sm font-medium">{value}</div>
    </div>
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

function formatLiveness(state?: DeviceLiveness): string {
  switch (state) {
    case DeviceLiveness.ONLINE:
      return 'Online'
    case DeviceLiveness.DEGRADED:
      return 'Degraded'
    case DeviceLiveness.OFFLINE:
      return 'Offline'
    default:
      return 'Unknown'
  }
}

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
    default:
      return 'Unknown'
  }
}
