import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { LuMonitor, LuPlus } from '@s4wave/web/images/icons.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DeviceHandle, DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { DeviceAvailabilityState } from '@s4wave/sdk/device/device.pb.js'
import { SshHostHandle, SshHostTypeID } from '@s4wave/sdk/sshhost/sshhost.js'

import { buildWizardObjectKey } from '../space/create-op-builders.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from './add-device-wizard.js'
import { ComputersDashboardTypeID } from './computers.js'
import { projectSshConnectionStatus } from './SshHostViewer.js'

export { ComputersDashboardTypeID }

type InventoryFilter = 'all' | 'devices' | 'hosts'

const noKeys: string[] = []

export function ComputersDashboardViewer({
  worldState,
}: ObjectViewerComponentProps) {
  const { navigateToObjects, spaceWorld, spaceWorldResource } =
    SpaceContainerContext.useContext()
  const visibleWizardTypeSet = useVisibleObjectWizardTypeSet()
  const computers = useWorldQuery(
    spaceWorldResource,
    async (world, signal) => {
      const [devices, hosts, wizards] = await Promise.all([
        world.listObjectsWithType(DeviceTypeID, signal),
        world.listObjectsWithType(SshHostTypeID, signal),
        world.listObjectsWithType(AddDeviceWizardTypeID, signal),
      ])
      return { devices, hosts, wizards }
    },
    [],
  ).value
  const devices = computers?.devices ?? noKeys
  const hosts = computers?.hosts ?? noKeys
  const seededAddDeviceWizardKey = computers?.wizards[0] ?? ''
  const canCreateAddDeviceWizard = visibleWizardTypeSet.has(DeviceTypeID)
  // Until the World answers, a click could not tell a seeded wizard apart from
  // none and would create a duplicate.
  const canAddDevice =
    !!computers && (!!seededAddDeviceWizardKey || canCreateAddDeviceWizard)
  const [opening, setOpening] = useState(false)
  const [openingError, setOpeningError] = useState('')
  const [filter, setFilter] = useState<InventoryFilter>('all')
  const [onlineKeys, setOnlineKeys] = useState<ReadonlySet<string>>(
    () => new Set(),
  )
  const handleOnline = useCallback((objectKey: string, online: boolean) => {
    setOnlineKeys((prev) => {
      if (prev.has(objectKey) === online) return prev
      const next = new Set(prev)
      if (online) next.add(objectKey)
      else next.delete(objectKey)
      return next
    })
  }, [])

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
    try {
      const wizardKey = await buildWizardObjectKey(
        spaceWorld,
        AddDeviceDefaultName,
      )
      const opData = CreateWizardObjectOp.toBinary({
        objectKey: wizardKey,
        wizardTypeId: AddDeviceWizardTypeID,
        targetTypeId: DeviceTypeID,
        targetKeyPrefix: AddDeviceWizardTargetKeyPrefix,
        name: AddDeviceDefaultName,
        timestamp: new Date(),
      })
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
    navigateToObjects,
    opening,
    seededAddDeviceWizardKey,
    spaceWorld,
  ])

  const inventory = useMemo(() => {
    const entries: InventoryEntry[] = [
      ...devices.map((objectKey) => ({ objectKey, kind: 'device' as const })),
      ...hosts.map((objectKey) => ({ objectKey, kind: 'host' as const })),
    ]
    if (filter === 'devices')
      return entries.filter((entry) => entry.kind === 'device')
    if (filter === 'hosts')
      return entries.filter((entry) => entry.kind === 'host')
    return entries
  }, [devices, filter, hosts])
  const total = devices.length + hosts.length
  const online = [...devices, ...hosts].filter((key) =>
    onlineKeys.has(key),
  ).length

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
          <FigureStrip
            figures={[
              { label: 'Devices', value: devices.length },
              { label: 'SSH hosts', value: hosts.length },
              { label: 'Online', value: online, unit: `of ${total}` },
            ]}
          />

          {computers && devices.length + hosts.length === 0 ? (
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
                <h3 className="text-foreground text-xs font-medium select-none">
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
                worldState={worldState}
                onOnline={handleOnline}
                onOpen={(objectKey) => navigateToObjects([objectKey])}
              />
            </section>
          )}
        </div>
      </div>
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
        'h-7 rounded-md px-2 text-xs font-medium transition-colors',
        active
          ? 'bg-foreground/8 text-foreground'
          : 'text-foreground-alt/70 hover:bg-foreground/5 hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}

interface InventoryEntry {
  objectKey: string
  kind: 'device' | 'host'
}

type ChipTone = 'idle' | 'success' | 'warning' | 'error'

const chipTone: Record<ChipTone, string> = {
  idle: 'border-foreground/15 bg-foreground/5 text-foreground-alt/70',
  success: 'border-emerald-400/20 bg-emerald-400/8 text-emerald-300',
  warning: 'border-amber-400/20 bg-amber-400/8 text-amber-300',
  error: 'border-destructive/30 bg-destructive/8 text-destructive',
}

// RowView is what one inventory row shows once its object state streams in.
interface RowView {
  name: string
  platform: string
  state: string
  tone: ChipTone
  capabilities?: number
  online: boolean
}

const rowGrid =
  'grid grid-cols-inventory-narrow items-center gap-x-4 @2xl:grid-cols-inventory'

function InventoryPanel({
  entries,
  worldState,
  onOnline,
  onOpen,
}: {
  entries: InventoryEntry[]
  worldState: Resource<IWorldState>
  onOnline: (objectKey: string, online: boolean) => void
  onOpen: (objectKey: string) => void
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 @container overflow-hidden rounded-lg border">
      {entries.length === 0 ? (
        <div className="text-foreground-alt/70 px-3.5 py-4 text-xs">
          No computers match this filter.
        </div>
      ) : (
        <div>
          <div
            aria-hidden
            className={cn(
              rowGrid,
              'border-foreground/8 text-foreground-alt micro-ten border-b px-3.5 py-2 font-semibold tracking-widest uppercase select-none',
            )}
          >
            <span>Name</span>
            <span className="hidden @2xl:block">Kind</span>
            <span className="hidden @2xl:block">Platform</span>
            <span>State</span>
            <span className="hidden text-right @2xl:block">Capabilities</span>
          </div>
          <div className="divide-foreground/8 divide-y">
            {entries.map((entry) =>
              entry.kind === 'device' ? (
                <DeviceRow
                  key={entry.objectKey}
                  objectKey={entry.objectKey}
                  worldState={worldState}
                  onOnline={onOnline}
                  onOpen={onOpen}
                />
              ) : (
                <HostRow
                  key={entry.objectKey}
                  objectKey={entry.objectKey}
                  worldState={worldState}
                  onOnline={onOnline}
                  onOpen={onOpen}
                />
              ),
            )}
          </div>
        </div>
      )}
    </div>
  )
}

interface RowProps {
  objectKey: string
  worldState: Resource<IWorldState>
  onOnline: (objectKey: string, online: boolean) => void
  onOpen: (objectKey: string) => void
}

const watchDevice = (h: DeviceHandle, signal: AbortSignal) =>
  h.watchDeviceAvailability(signal)

function DeviceRow({ objectKey, worldState, onOnline, onOpen }: RowProps) {
  // Observe the Device record together with its authenticated transport.
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    DeviceHandle,
    DeviceTypeID,
  )
  const availability = useStreamingResource(handle, watchDevice, []).value
  const device = availability?.device
  const liveness = availability?.state ?? DeviceAvailabilityState.UNKNOWN

  // Render reachability separately from the daemon's last status report.
  const view: RowView = {
    name: device?.label || 'Device',
    platform:
      [device?.platform?.os, device?.platform?.arch]
        .filter(Boolean)
        .join(' / ') || '',
    ...(availability
      ? livenessChip(liveness)
      : { state: 'Loading', tone: 'idle' as const }),
    capabilities: device ? (device.capabilities?.length ?? 0) : undefined,
    online: liveness === DeviceAvailabilityState.ONLINE,
  }
  return (
    <InventoryRow
      objectKey={objectKey}
      kind="Device"
      view={view}
      onOnline={onOnline}
      onOpen={onOpen}
    />
  )
}

const watchHost = (h: SshHostHandle, signal: AbortSignal) =>
  h.watchSshHostState(signal)

function HostRow({ objectKey, worldState, onOnline, onOpen }: RowProps) {
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SshHostHandle,
    SshHostTypeID,
  )
  const host = useStreamingResource(handle, watchHost, []).value
  const status = projectSshConnectionStatus(host?.lastStatus)
  const endpoint = host?.endpoint
  const view: RowView = {
    name: host?.label || 'SSH host',
    platform: endpoint?.host
      ? `${endpoint.username ? endpoint.username + '@' : ''}${endpoint.host}`
      : '',
    state: status.label,
    tone:
      status.state === 'ready'
        ? 'success'
        : status.state === 'failed'
          ? 'error'
          : 'idle',
    online: status.state === 'ready',
  }
  return (
    <InventoryRow
      objectKey={objectKey}
      kind="SSH host"
      view={view}
      onOnline={onOnline}
      onOpen={onOpen}
    />
  )
}

function livenessChip(liveness: DeviceAvailabilityState): {
  state: string
  tone: ChipTone
} {
  switch (liveness) {
    case DeviceAvailabilityState.ONLINE:
      return { state: 'Online', tone: 'success' }
    case DeviceAvailabilityState.OFFLINE:
      return { state: 'Offline', tone: 'error' }
    default:
      return { state: 'Unknown', tone: 'idle' }
  }
}

function InventoryRow({
  objectKey,
  kind,
  view,
  onOnline,
  onOpen,
}: {
  objectKey: string
  kind: string
  view: RowView
  onOnline: (objectKey: string, online: boolean) => void
  onOpen: (objectKey: string) => void
}) {
  const { online } = view
  useEffect(() => {
    onOnline(objectKey, online)
  }, [objectKey, online, onOnline])
  useEffect(() => () => onOnline(objectKey, false), [objectKey, onOnline])

  return (
    <button
      type="button"
      onClick={() => onOpen(objectKey)}
      className={cn(
        rowGrid,
        'hover:bg-foreground/5 w-full min-w-0 px-3.5 py-2.5 text-left transition-colors',
      )}
    >
      <span className="min-w-0">
        <span className="text-foreground block truncate text-sm font-medium">
          {view.name}
        </span>
        <span className="text-foreground-alt/50 block truncate font-mono text-xs">
          {objectKey}
        </span>
      </span>
      <span className="text-foreground-alt hidden truncate text-xs @2xl:block">
        {kind}
      </span>
      <span className="text-foreground-alt hidden truncate text-xs @2xl:block">
        {view.platform}
      </span>
      <span>
        <span
          className={cn(
            'micro-ten inline-flex items-center rounded-full border px-2 py-0.5 font-medium tracking-widest uppercase select-none',
            chipTone[view.tone],
          )}
        >
          {view.state}
        </span>
      </span>
      <span className="text-foreground hidden text-right font-mono text-xs tabular-nums @2xl:block">
        {view.capabilities ?? ''}
      </span>
    </button>
  )
}
