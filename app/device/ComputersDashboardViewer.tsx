import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { LuHardDrive, LuMonitor, LuPlus, LuServer } from 'react-icons/lu'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { cn } from '@s4wave/web/style/utils.js'
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

type InventoryFilter = 'all' | 'devices' | 'hosts'

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
  const [opening, setOpening] = useState(false)
  const [openingError, setOpeningError] = useState('')
  const [filter, setFilter] = useState<InventoryFilter>('all')

  const handleAddDevice = useCallback(async () => {
    if (opening) return
    setOpeningError('')
    setOpening(true)
    if (seededAddDeviceWizardKey) {
      navigateToObjects([seededAddDeviceWizardKey])
      setOpening(false)
      return
    }
    if (!canCreateAddDeviceWizard) {
      setOpening(false)
      return
    }
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
    } catch {
      const message = 'Add Device could not be opened. Try again.'
      setOpeningError(message)
      toast.error(message)
    } finally {
      setOpening(false)
    }
  }, [
    canCreateAddDeviceWizard,
    existingObjectKeys,
    navigateToObjects,
    opening,
    seededAddDeviceWizardKey,
    spaceWorld,
  ])

  const inventory = useMemo(() => {
    const entries = [
      ...devices.map((obj) => ({
        objectKey: obj.objectKey ?? '',
        kind: 'Managed Device',
        icon: <LuHardDrive className="size-3.5" />,
      })),
      ...hosts.map((obj) => ({
        objectKey: obj.objectKey ?? '',
        kind: 'SSH Host',
        icon: <LuServer className="size-3.5" />,
      })),
    ]
    if (filter === 'devices')
      return entries.filter((entry) => entry.kind === 'Managed Device')
    if (filter === 'hosts')
      return entries.filter((entry) => entry.kind === 'SSH Host')
    return entries
  }, [devices, filter, hosts])

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
            disabled={opening}
          >
            {opening ? 'Opening Add Device…' : 'Add Device'}
          </DashboardButton>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
          {openingError && (
            <div
              className="border-destructive/15 bg-destructive/5 text-destructive rounded-lg border p-3 text-xs leading-relaxed"
              role="alert"
            >
              {openingError}
            </div>
          )}
          <section className="grid gap-3 sm:grid-cols-2">
            <SummaryTile
              icon={<LuHardDrive className="size-3.5" />}
              label="Managed Devices"
              value={devices.length}
            />
            <SummaryTile
              icon={<LuServer className="size-3.5" />}
              label="SSH Hosts"
              value={hosts.length}
            />
          </section>

          {objects.length === 0 ? (
            <InfoCard
              icon={<LuMonitor className="text-foreground-alt/60 size-3.5" />}
              title="No computers added"
            >
              <div className="flex flex-col items-start gap-3">
                <p className="text-foreground-alt/70 text-xs leading-relaxed">
                  Add a managed Device for persistent capabilities, or add an
                  SSH Host for on-demand terminal access.
                </p>
              </div>
            </InfoCard>
          ) : (
            <section className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium">
                  <LuMonitor className="size-3.5" />
                  Inventory
                </h3>
                <div className="flex items-center gap-1" role="tablist">
                  <FilterButton
                    active={filter === 'all'}
                    onClick={() => setFilter('all')}
                  >
                    All
                  </FilterButton>
                  <FilterButton
                    active={filter === 'devices'}
                    onClick={() => setFilter('devices')}
                  >
                    Devices
                  </FilterButton>
                  <FilterButton
                    active={filter === 'hosts'}
                    onClick={() => setFilter('hosts')}
                  >
                    SSH Hosts
                  </FilterButton>
                </div>
              </div>
              <InventoryPanel
                entries={inventory}
                onOpen={(objectKey) => navigateToObjects([objectKey])}
              />
            </section>
          )}
        </div>
      </div>
    </div>
  )
}

function SummaryTile({
  icon,
  label,
  value,
}: {
  icon: ReactNode
  label: string
  value: number
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
      <div className="text-foreground-alt/70 flex items-center gap-1.5 text-xs font-medium">
        {icon}
        {label}
      </div>
      <div className="text-foreground mt-2 text-xl font-semibold">{value}</div>
    </div>
  )
}

function FilterButton({
  active,
  children,
  onClick,
}: {
  active: boolean
  children: ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={cn(
        'h-7 rounded-md border px-2 text-xs font-medium transition-colors',
        active
          ? 'border-brand/30 bg-brand/10 text-foreground'
          : 'border-foreground/10 text-foreground-alt/70 hover:border-foreground/20 hover:bg-foreground/5',
      )}
    >
      {children}
    </button>
  )
}

function InventoryPanel({
  entries,
  onOpen,
}: {
  entries: Array<{
    objectKey: string
    kind: string
    icon: ReactNode
  }>
  onOpen: (objectKey: string) => void
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border">
      {entries.length === 0 ? (
        <div className="text-foreground-alt/70 px-3.5 py-4 text-xs">
          No computers match this filter.
        </div>
      ) : (
        <div className="divide-foreground/8 divide-y">
          {entries.map((entry) => (
            <button
              key={entry.objectKey}
              type="button"
              onClick={() => onOpen(entry.objectKey)}
              className="hover:bg-foreground/5 flex w-full min-w-0 items-center justify-between gap-3 px-3.5 py-3 text-left transition-colors"
            >
              <span className="flex min-w-0 items-center gap-2">
                <span className="text-foreground-alt/60 shrink-0">
                  {entry.icon}
                </span>
                <span className="min-w-0">
                  <span className="text-foreground block truncate text-sm font-medium">
                    {entry.kind}
                  </span>
                  <span className="text-foreground-alt/50 mt-0.5 block truncate font-mono text-[0.6rem]">
                    {entry.objectKey}
                  </span>
                </span>
              </span>
              <span className="text-foreground-alt/70 shrink-0 text-xs font-medium">
                Open
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
