import { useCallback, useMemo, type ReactNode } from 'react'
import {
  LuHardDrive,
  LuMonitor,
  LuPlus,
  LuServer,
  LuTerminal,
} from 'react-icons/lu'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { DeviceHandle, DeviceTypeID } from '@s4wave/sdk/device/device.js'
import type { Device } from '@s4wave/sdk/device/device.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'

import { buildWizardObjectKey } from '../space/create-op-builders.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from './add-device-wizard.js'
import { ComputersDashboardTypeID } from './computers.js'
import {
  buildCreateTerminalOpData,
  findOpenableTerminalCapability,
} from './terminal-action.js'

export { ComputersDashboardTypeID }

export function ComputersDashboardViewer({
  worldState,
}: ObjectViewerComponentProps) {
  const { navigateToObjects, spaceState, spaceWorld } =
    SpaceContainerContext.useContext()
  const visibleWizardTypeSet = useVisibleObjectWizardTypeSet()
  const rawObjects = spaceState.worldContents?.objects
  const objects = useMemo(() => rawObjects ?? [], [rawObjects])
  const devices = useMemo(
    () => objects.filter((obj) => obj.objectType === DeviceTypeID),
    [objects],
  )
  const hosts = useMemo(
    () =>
      objects.filter((obj) => {
        const typeID = obj.objectType ?? ''
        return typeID === 'ssh/host' || typeID === 'spacewave/host'
      }),
    [objects],
  )
  const existingObjectKeys = useMemo(
    () => objects.map((obj) => obj.objectKey ?? ''),
    [objects],
  )
  const canAddDevice = visibleWizardTypeSet.has(DeviceTypeID)

  const handleAddDevice = useCallback(async () => {
    if (!canAddDevice) return
    const wizardKey = buildWizardObjectKey(
      AddDeviceDefaultName,
      existingObjectKeys,
    )
    const opData = CreateWizardObjectOp.toBinary({
      objectKey: wizardKey,
      wizardTypeId: AddDeviceWizardTypeID,
      targetTypeId: DeviceTypeID,
      targetKeyPrefix: AddDeviceWizardTargetKeyPrefix,
      name: AddDeviceDefaultName,
      timestamp: new Date(),
    })
    try {
      await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
      navigateToObjects([wizardKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open Add Device',
      )
    }
  }, [canAddDevice, existingObjectKeys, navigateToObjects, spaceWorld])

  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <span className="text-foreground flex min-w-0 items-center gap-2 text-sm font-semibold select-none">
          <LuMonitor className="size-4 shrink-0" />
          Computers
        </span>
        {canAddDevice && (
          <DashboardButton
            icon={<LuPlus className="size-3.5" />}
            onClick={() => void handleAddDevice()}
          >
            Add Device
          </DashboardButton>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-4">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
          <section className="grid gap-3 sm:grid-cols-2">
            <SummaryTile label="Devices" value={devices.length} />
            <SummaryTile label="Hosts" value={hosts.length} />
          </section>

          <section className="grid gap-4 lg:grid-cols-2">
            <InventoryPanel
              title="Devices"
              icon={<LuHardDrive className="size-3.5" />}
              entries={devices.map((obj) => obj.objectKey ?? '')}
              empty="No Device objects yet"
              onOpen={(objectKey) => navigateToObjects([objectKey])}
              renderAction={(objectKey) => (
                <DeviceTerminalAction
                  deviceObjectKey={objectKey}
                  existingObjectKeys={existingObjectKeys}
                  navigateToObjects={navigateToObjects}
                  spaceWorld={spaceWorld}
                  worldState={worldState}
                />
              )}
            />
            <InventoryPanel
              title="Hosts"
              icon={<LuServer className="size-3.5" />}
              entries={hosts.map((obj) => obj.objectKey ?? '')}
              empty="No host entries yet"
              onOpen={(objectKey) => navigateToObjects([objectKey])}
            />
          </section>
        </div>
      </div>
    </div>
  )
}

function SummaryTile({ label, value }: { label: string; value: number }) {
  return (
    <div className="border-foreground/10 bg-background-secondary rounded-md border p-3">
      <div className="text-muted-foreground text-xs font-medium uppercase">
        {label}
      </div>
      <div className="text-foreground mt-2 text-xl font-semibold">{value}</div>
    </div>
  )
}

function InventoryPanel({
  title,
  icon,
  entries,
  empty,
  onOpen,
  renderAction,
}: {
  title: string
  icon: ReactNode
  entries: string[]
  empty: string
  onOpen: (objectKey: string) => void
  renderAction?: (objectKey: string) => ReactNode
}) {
  return (
    <div className="border-foreground/10 bg-background-secondary overflow-hidden rounded-md border">
      <div className="border-foreground/8 flex h-9 items-center gap-2 border-b px-3">
        <span className="text-muted-foreground">{icon}</span>
        <span className="text-foreground text-sm font-medium">{title}</span>
      </div>
      {entries.length === 0 ? (
        <div className="text-muted-foreground px-3 py-6 text-sm">{empty}</div>
      ) : (
        <div className="divide-foreground/8 divide-y">
          {entries.map((objectKey) => (
            <div
              key={objectKey}
              className="hover:bg-foreground/5 flex min-w-0 items-center gap-2 px-3 transition-colors"
            >
              <button
                type="button"
                onClick={() => onOpen(objectKey)}
                className="flex min-h-9 min-w-0 flex-1 items-center justify-between gap-3 py-2 text-left"
              >
                <span className="text-foreground min-w-0 truncate text-sm font-medium">
                  {objectKey}
                </span>
                <span className="text-muted-foreground shrink-0 text-xs">
                  Open
                </span>
              </button>
              {renderAction?.(objectKey)}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function DeviceTerminalAction({
  deviceObjectKey,
  existingObjectKeys,
  navigateToObjects,
  spaceWorld,
  worldState,
}: {
  deviceObjectKey: string
  existingObjectKeys: Iterable<string | undefined>
  navigateToObjects: (objectKeys: string[]) => void
  spaceWorld: {
    applyWorldOp(
      opTypeId: string,
      opData: Uint8Array,
      opSender: string,
    ): Promise<unknown>
  }
  worldState: ObjectViewerComponentProps['worldState']
}) {
  const handle = useAccessTypedHandle(
    worldState,
    deviceObjectKey,
    DeviceHandle,
    DeviceTypeID,
  )
  const streamFactory = useCallback(
    (h: DeviceHandle, signal: AbortSignal) => h.watchDeviceState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Device | undefined = stateResource.value ?? undefined
  const terminalCapability = findOpenableTerminalCapability(state)

  const handleOpenTerminal = async () => {
    if (!state || !terminalCapability) return
    const terminalOp = buildCreateTerminalOpData({
      device: state,
      deviceObjectKey,
      existingObjectKeys,
    })
    if (!terminalOp) return
    try {
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

  if (!terminalCapability || !state?.peerId) return null
  return (
    <button
      type="button"
      title="Open Terminal"
      aria-label={`Open terminal for ${state.label || deviceObjectKey}`}
      onClick={() => void handleOpenTerminal()}
      className="text-muted-foreground hover:text-foreground hover:bg-foreground/8 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors"
    >
      <LuTerminal className="size-3.5" />
    </button>
  )
}
