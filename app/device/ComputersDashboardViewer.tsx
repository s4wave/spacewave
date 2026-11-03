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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'inline-flex',
    alignItems: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-emerald-400']} 8%, transparent)`,
    color: themeColors['--color-emerald-300'],
  },
  rule3: {
    borderColor: 'color-mix(in oklab, var(--color-amber-400) 20%, transparent)',
    backgroundColor:
      'color-mix(in oklab, var(--color-amber-400) 8%, transparent)',
    color: 'var(--color-amber-300)',
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 8%, transparent)`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule7: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule11: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule12: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule13: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule14: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule15: {
    containerType: 'inline-size',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule16: {
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule17: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'minmax(0, 1fr) auto',
      '@container (min-width: 42rem)':
        'minmax(0, 1.4fr) 6rem minmax(0, 1fr) 6.5rem 6.5rem',
    },
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule18: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule19: {
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
  },
  rule20: {
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    textAlign: 'right',
  },
  rule21: {
    width: '100%',
    minWidth: 0,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule22: { minWidth: 0 },
  rule23: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule24: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule25: {
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule26: {
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>
          <LuMonitor sx={stylexStyles.rule1} />
          Computers
        </span>
        {canAddDevice && (
          <DashboardButton
            icon={<LuPlus sx={stylexStyles.rule2} />}
            onClick={() => void handleAddDevice()}
            disabled={opening}
          >
            {opening ? 'Opening Add Device…' : 'Add Device'}
          </DashboardButton>
        )}
      </div>
      <div sx={stylexStyles2.rule4}>
        <div sx={stylexStyles2.rule5}>
          {openingError && (
            <div sx={stylexStyles2.rule6} role="alert">
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
              icon={<LuMonitor sx={stylexStyles.rule3} />}
              title="No computers added"
            >
              <div sx={stylexStyles2.rule7}>
                <p sx={stylexStyles2.rule8}>
                  Add a managed Device for persistent capabilities, or add an
                  SSH Host for on-demand terminal access.
                </p>
              </div>
            </InfoCard>
          ) : (
            <section sx={false}>
              <div sx={[nativeChildSpacing.s2, stylexStyles2.rule9]}>
                <h3 sx={stylexStyles2.rule10}>Inventory</h3>
                <div sx={stylexStyles2.rule11} role="tablist">
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
                sx={nativeChildSpacing.s2}
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
      sx={[
        stylexStyles2.rule12,
        active ? stylexStyles2.rule13 : stylexStyles2.rule14,
      ]}
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

const chipTone: Record<ChipTone, stylex.StyleXStyles> = {
  idle: stylexStyles3.rule1,
  success: stylexStyles3.rule2,
  warning: stylexStyles3.rule3,
  error: stylexStyles3.rule4,
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

function InventoryPanel({
  sx,
  entries,
  worldState,
  onOnline,
  onOpen,
}: {
  entries: InventoryEntry[]
  worldState: Resource<IWorldState>
  onOnline: (objectKey: string, online: boolean) => void
  onOpen: (objectKey: string) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule15, sx]}>
      {entries.length === 0 ? (
        <div sx={stylexStyles2.rule16}>No computers match this filter.</div>
      ) : (
        <div>
          <div aria-hidden sx={[stylexStyles2.rule17, stylexStyles2.rule18]}>
            <span>Name</span>
            <span sx={stylexStyles2.rule19}>Kind</span>
            <span sx={stylexStyles2.rule19}>Platform</span>
            <span>State</span>
            <span sx={stylexStyles2.rule20}>Capabilities</span>
          </div>
          <div>
            {entries.map((entry) =>
              entry.kind === 'device' ? (
                <DeviceRow
                  sx={nativeDividers.t8}
                  key={entry.objectKey}
                  objectKey={entry.objectKey}
                  worldState={worldState}
                  onOnline={onOnline}
                  onOpen={onOpen}
                />
              ) : (
                <HostRow
                  sx={nativeDividers.t8}
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

function DeviceRow({
  sx,
  objectKey,
  worldState,
  onOnline,
  onOpen,
}: RowProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
      sx={sx}
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

function HostRow({
  sx,
  objectKey,
  worldState,
  onOnline,
  onOpen,
}: RowProps & { sx?: stylex.StyleXStyles; className?: never }) {
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
      sx={sx}
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
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const { online } = view
  useEffect(() => {
    onOnline(objectKey, online)
  }, [objectKey, online, onOnline])
  useEffect(() => () => onOnline(objectKey, false), [objectKey, onOnline])

  return (
    <button
      type="button"
      onClick={() => onOpen(objectKey)}
      sx={[[stylexStyles2.rule17, stylexStyles2.rule21], sx]}
    >
      <span sx={stylexStyles2.rule22}>
        <span sx={stylexStyles2.rule23}>{view.name}</span>
        <span sx={stylexStyles2.rule24}>{objectKey}</span>
      </span>
      <span sx={stylexStyles2.rule25}>{kind}</span>
      <span sx={stylexStyles2.rule25}>{view.platform}</span>
      <span>
        <span sx={[[stylexStyles4.rule1], [chipTone[view.tone]]]}>
          {view.state}
        </span>
      </span>
      <span sx={stylexStyles2.rule26}>{view.capabilities ?? ''}</span>
    </button>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeDividers applies the list border only to its nonfinal physical rows.
const nativeDividers = stylex.create({
  t8: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 8%, transparent)',
    },
  },
})
