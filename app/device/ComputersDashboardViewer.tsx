import { useCallback, useMemo, type ReactNode } from 'react'
import { LuHardDrive, LuMonitor, LuPlus, LuServer } from 'react-icons/lu'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { SshHostTypeID } from '@s4wave/sdk/sshhost/sshhost.js'

import { buildWizardObjectKey } from '../space/create-op-builders.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from './add-device-wizard.js'
import { ComputersDashboardTypeID } from './computers.js'

export { ComputersDashboardTypeID }

export function ComputersDashboardViewer(_props: ObjectViewerComponentProps) {
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
        return typeID === SshHostTypeID
      }),
    [objects],
  )
  const existingObjectKeys = useMemo(
    () => objects.map((obj) => obj.objectKey ?? ''),
    [objects],
  )
  const seededAddDeviceWizardKey = useMemo(
    () =>
      objects.find((obj) => obj.objectType === AddDeviceWizardTypeID)
        ?.objectKey ?? '',
    [objects],
  )
  const canCreateAddDeviceWizard = visibleWizardTypeSet.has(DeviceTypeID)
  const canAddDevice = !!seededAddDeviceWizardKey || canCreateAddDeviceWizard

  const handleAddDevice = useCallback(async () => {
    if (seededAddDeviceWizardKey) {
      navigateToObjects([seededAddDeviceWizardKey])
      return
    }
    if (!canCreateAddDeviceWizard) return
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
  }, [
    canCreateAddDeviceWizard,
    existingObjectKeys,
    navigateToObjects,
    seededAddDeviceWizardKey,
    spaceWorld,
  ])

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
              emptyAction={
                canAddDevice ? (
                  <DashboardButton
                    icon={<LuPlus className="size-3.5" />}
                    onClick={() => void handleAddDevice()}
                  >
                    Add Device
                  </DashboardButton>
                ) : undefined
              }
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
  emptyAction,
  onOpen,
}: {
  title: string
  icon: ReactNode
  entries: string[]
  empty: string
  emptyAction?: ReactNode
  onOpen: (objectKey: string) => void
}) {
  return (
    <div className="border-foreground/10 bg-background-secondary overflow-hidden rounded-md border">
      <div className="border-foreground/8 flex h-9 items-center gap-2 border-b px-3">
        <span className="text-muted-foreground">{icon}</span>
        <span className="text-foreground text-sm font-medium">{title}</span>
      </div>
      {entries.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-start gap-3 px-3 py-6 text-sm">
          <span>{empty}</span>
          {emptyAction}
        </div>
      ) : (
        <div className="divide-foreground/8 divide-y">
          {entries.map((objectKey) => (
            <button
              key={objectKey}
              type="button"
              onClick={() => onOpen(objectKey)}
              className="hover:bg-foreground/5 flex w-full min-w-0 items-center justify-between gap-3 px-3 py-2 text-left transition-colors"
            >
              <span className="text-foreground min-w-0 truncate text-sm font-medium">
                {objectKey}
              </span>
              <span className="text-muted-foreground shrink-0 text-xs">
                Open
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
