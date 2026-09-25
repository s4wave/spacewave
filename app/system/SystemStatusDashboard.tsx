import {
  useEffect,
  useRef,
  type ComponentProps,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import {
  LuArrowDown,
  LuArrowLeft,
  LuArrowRight,
  LuArrowUp,
  LuBox,
  LuCircleAlert,
  LuCpu,
  LuFolderOpen,
  LuHardDrive,
  LuLayers,
  LuRadar,
  LuRefreshCw,
  LuRocket,
  LuTriangleAlert,
  LuX,
} from 'react-icons/lu'

import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import {
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useTrackedResources } from '@s4wave/web/devtools/index.js'
import { useStateInspectorEntryMap } from '@s4wave/web/devtools/useStateInspectorEntries.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { NetworkInspector } from './NetworkInspector.js'
import { ReleaseInspector } from './ReleaseInspector.js'
import { RuntimeInspector, type RuntimeView } from './RuntimeInspector.js'
import { SpacesInspector } from './SpacesInspector.js'
import { StorageInspector } from './StorageInspector.js'
import { StorageMeter } from './StorageMeter.js'
import { SyncInspector } from './SyncInspector.js'
import { SystemTile } from './SystemTile.js'
import { UnderTheHoodInspector } from './UnderTheHoodInspector.js'
import {
  formatBytes,
  formatCount,
  plural,
  protectionLabel,
  shortId,
} from './format.js'
import { toneDotClass, toneTextClass } from './tone.js'
import {
  spaceEngineId,
  updatePhaseLabel,
  useSystemModel,
  type AttentionItem,
  type SubsystemId,
  type SystemModel,
  type SystemTone,
} from './useSystemModel.js'

// InspectorId names a full-size inspector: one per subsystem tile plus the
// two devtools trees.
type InspectorId = SubsystemId | 'resources' | 'atoms'

// TileRef registers the control that opens an inspector, so focus can return
// to it when the inspector closes.
type TileRef = (id: InspectorId) => (node: HTMLButtonElement | null) => void

// inspectorMeta titles and describes every inspector in navigation order.
const inspectorMeta: Record<
  InspectorId,
  { title: string; icon: ReactNode; description: string }
> = {
  sync: {
    title: 'Sync',
    icon: <LuRefreshCw />,
    description: 'How this session moves blocks to and from your other copies.',
  },
  storage: {
    title: 'Storage',
    icon: <LuHardDrive />,
    description: 'Where your data lives on this device and how safe it is.',
  },
  network: {
    title: 'Network',
    icon: <LuRadar />,
    description: 'The encrypted peer links this device holds right now.',
  },
  runtime: {
    title: 'Runtime',
    icon: <LuCpu />,
    description:
      'Plugins, controllers, and directives running on the session bus.',
  },
  release: {
    title: 'Release',
    icon: <LuRocket />,
    description: 'This build, its updates, and how it booted.',
  },
  spaces: {
    title: 'Spaces & Accounts',
    icon: <LuFolderOpen />,
    description: 'Every account on this device and the Spaces it holds.',
  },
  resources: {
    title: 'Resources',
    icon: <LuLayers />,
    description: 'SDK resources this app holds open, as a live tree.',
  },
  atoms: {
    title: 'State atoms',
    icon: <LuBox />,
    description: 'Persisted UI state, as a live tree.',
  },
}

const inspectorOrder = Object.keys(inspectorMeta) as InspectorId[]

interface SystemStatusDashboardProps {
  onClose?: () => void
}

// SystemStatusDashboard is the system mission control overlay. Every
// subsystem shows as a live tile under a one-line verdict; a tile opens its
// full inspector in place, and Escape returns to the tiles.
export function SystemStatusDashboard({ onClose }: SystemStatusDashboardProps) {
  const ns = useStateNamespace(['system-status-dashboard'])
  const [inspector, setInspector] = useStateAtom<InspectorId | ''>(
    ns,
    'inspector',
    '',
  )
  const [runtimeView, setRuntimeView] = useStateAtom<RuntimeView>(
    ns,
    'runtime-view',
    'plugins',
  )
  const model = useSystemModel()
  const navigateSession = useSessionNavigate()

  const tileRefs = useRef<Partial<Record<InspectorId, HTMLButtonElement>>>({})
  const inspectorRef = useRef<HTMLDivElement>(null)
  const lastInspectorRef = useRef<InspectorId | ''>('')

  // Move focus into an opened inspector, and back to its tile on return.
  useEffect(() => {
    if (inspector) {
      lastInspectorRef.current = inspector
      inspectorRef.current?.focus({ preventScroll: true })
      return
    }
    const last = lastInspectorRef.current
    if (last) {
      tileRefs.current[last]?.focus()
    }
  }, [inspector])

  const close = () => onClose?.()

  // Escape leaves an open inspector before it reaches the overlay frame.
  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.key !== 'Escape' || !inspector) {
      return
    }
    event.preventDefault()
    event.stopPropagation()
    setInspector('')
  }

  const tileRef: TileRef = (id) => (node) => {
    if (node) {
      tileRefs.current[id] = node
    } else {
      delete tileRefs.current[id]
    }
  }

  return (
    <div
      role="region"
      aria-label="System status"
      className="bg-background @container flex h-full w-full flex-col"
      onKeyDown={handleKeyDown}
    >
      <VerdictBar model={model} onClose={close} />

      {inspector ? (
        <>
          <InspectorNav
            current={inspector}
            tones={model.tones}
            onSelect={setInspector}
            onBack={() => setInspector('')}
          />
          <div className="flex min-h-0 flex-1 flex-col overflow-auto">
            <div
              key={inspector}
              ref={inspectorRef}
              tabIndex={-1}
              aria-labelledby="system-inspector-title"
              className="animate-in fade-in-0 zoom-in-95 flex min-h-0 flex-1 flex-col gap-4 p-4 duration-200 ease-out outline-none motion-reduce:animate-none"
            >
              <div>
                <h2
                  id="system-inspector-title"
                  className="text-foreground flex items-center gap-2 text-lg font-semibold tracking-tight [&>svg]:size-4"
                >
                  {inspectorMeta[inspector].icon}
                  {inspectorMeta[inspector].title}
                </h2>
                <p className="text-foreground-alt/60 mt-0.5 text-xs">
                  {inspectorMeta[inspector].description}
                </p>
              </div>

              {inspector === 'sync' && <SyncInspector sync={model.sync} />}
              {inspector === 'storage' && (
                <StorageInspector
                  storage={model.storage}
                  onOpenSettings={() => {
                    navigateSession({ path: 'settings/storage' })
                    close()
                  }}
                />
              )}
              {inspector === 'network' && (
                <NetworkInspector network={model.network} />
              )}
              {inspector === 'runtime' && (
                <RuntimeInspector
                  plugins={model.plugins}
                  controllers={model.controllers}
                  directives={model.directives}
                  directiveCount={model.directiveCount}
                  view={runtimeView}
                  onViewChange={setRuntimeView}
                />
              )}
              {inspector === 'release' && (
                <ReleaseInspector
                  build={model.build}
                  recovery={model.recovery}
                />
              )}
              {inspector === 'spaces' && (
                <SpacesInspector
                  sessions={model.sessions}
                  spaces={model.spaces}
                  onClose={close}
                />
              )}
              {(inspector === 'resources' || inspector === 'atoms') && (
                <UnderTheHoodInspector kind={inspector} />
              )}
            </div>
          </div>
        </>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <AttentionList items={model.verdict.items} onOpen={setInspector} />
          <SystemGrid model={model} tileRef={tileRef} onOpen={setInspector} />
          <UnderTheHood tileRef={tileRef} onOpen={setInspector} />
        </div>
      )}
    </div>
  )
}

// VerdictBar is the one-line system verdict with the build identity.
function VerdictBar({
  model,
  onClose,
}: {
  model: SystemModel
  onClose: () => void
}) {
  const { build, verdict } = model
  const platform =
    build.goos && build.goarch ? `${build.goos}/${build.goarch}` : ''

  return (
    <header className="border-foreground/8 flex h-12 shrink-0 items-center gap-3 border-b px-4">
      <span
        className={cn(
          'size-2.5 shrink-0 rounded-full',
          toneDotClass[verdict.tone],
        )}
        aria-hidden="true"
      />
      <h1
        className="text-foreground min-w-0 truncate text-sm font-semibold tracking-tight"
        role="status"
      >
        {verdict.headline}
      </h1>
      <p className="text-foreground-alt/50 hidden min-w-0 truncate text-xs @lg:block">
        <span className="font-mono">{build.version || 'dev'}</span>
        {build.runtimeLabel && <> · {build.runtimeLabel}</>}
        {platform && (
          <>
            {' · '}
            <span className="font-mono">{platform}</span>
          </>
        )}
      </p>
      <button
        type="button"
        onClick={onClose}
        aria-label="Close system status"
        className="text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5 focus-visible:ring-brand/50 ml-auto flex size-7 shrink-0 items-center justify-center rounded-md transition-colors focus-visible:ring-2 focus-visible:outline-none"
      >
        <LuX className="size-4" aria-hidden="true" />
      </button>
    </header>
  )
}

// AttentionList lists every live fault or risk, each linked to the inspector
// that explains it.
function AttentionList({
  items,
  onOpen,
}: {
  items: AttentionItem[]
  onOpen: (id: InspectorId) => void
}) {
  if (!items.length) {
    return null
  }

  return (
    <ul className="space-y-2 px-4 pt-4" aria-label="Needs attention">
      {items.map((item) => (
        <li key={item.key}>
          <button
            type="button"
            onClick={() => onOpen(item.subsystem)}
            className={cn(
              'group focus-visible:ring-brand/50 flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none',
              item.tone === 'error'
                ? 'border-destructive/25 bg-destructive/5 hover:bg-destructive/10'
                : 'border-warning/20 bg-warning/5 hover:bg-warning/10',
            )}
          >
            {item.tone === 'error' ? (
              <LuCircleAlert
                className="text-destructive mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
            ) : (
              <LuTriangleAlert
                className="text-warning mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
            )}
            <span className="min-w-0 flex-1">
              <span className="text-foreground block text-xs font-medium">
                {item.title}
              </span>
              <span className="text-foreground-alt/70 mt-0.5 block text-xs break-words">
                {item.detail}
              </span>
            </span>
            <span className="text-foreground-alt/60 group-hover:text-foreground flex shrink-0 items-center gap-1 text-xs transition-colors">
              Inspect {inspectorMeta[item.subsystem].title}
              <LuArrowRight className="size-3.5" aria-hidden="true" />
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

// TileBase is the identity and open action every subsystem tile shares.
type TileBase = Pick<
  ComponentProps<typeof SystemTile>,
  'title' | 'icon' | 'tone' | 'buttonRef' | 'onOpen'
>

// SystemGrid lays out the six live subsystem tiles.
function SystemGrid({
  model,
  tileRef,
  onOpen,
}: {
  model: SystemModel
  tileRef: TileRef
  onOpen: (id: InspectorId) => void
}) {
  const tile = (id: SubsystemId): TileBase => ({
    title: inspectorMeta[id].title,
    icon: inspectorMeta[id].icon,
    tone: model.tones[id],
    buttonRef: tileRef(id),
    onOpen: () => onOpen(id),
  })

  return (
    <div className="grid grid-cols-1 gap-3 p-4 @2xl:grid-cols-6">
      <SyncTile base={tile('sync')} sync={model.sync} />
      <StorageTile base={tile('storage')} storage={model.storage} />
      <NetworkTile base={tile('network')} network={model.network} />
      <RuntimeTile base={tile('runtime')} model={model} />
      <ReleaseTile base={tile('release')} model={model} />
      <SpacesTile
        base={tile('spaces')}
        sessions={model.sessions}
        spaces={model.spaces}
      />
    </div>
  )
}

// SyncTile shows the sync summary, live transfer rates, and transport.
function SyncTile({
  base,
  sync,
}: {
  base: TileBase
  sync: SystemModel['sync']
}) {
  return (
    <SystemTile
      {...base}
      className="@2xl:col-span-2"
      headline={sync.summaryLabel}
    >
      <div className="flex items-center gap-4 font-mono tabular-nums">
        <span className="flex items-center gap-1">
          <LuArrowUp className="size-3" aria-label="Upload" />
          {sync.uploadRateLabel}
        </span>
        <span className="flex items-center gap-1">
          <LuArrowDown className="size-3" aria-label="Download" />
          {sync.downloadRateLabel}
        </span>
      </div>
      <p className="mt-1 truncate">{sync.transportLabel}</p>
    </SystemTile>
  )
}

// StorageTile shows the local store size, browser quota, and protection.
function StorageTile({
  base,
  storage,
}: {
  base: TileBase
  storage: SystemModel['storage']
}) {
  let headline = 'No reading'
  if (storage.providerLoading) {
    headline = 'Reading…'
  } else if (storage.providerSupported) {
    headline = formatBytes(storage.providerBytes)
  }

  return (
    <SystemTile {...base} className="@2xl:col-span-2" headline={headline}>
      <p className="font-mono tabular-nums">
        {formatCount(storage.blockCount)} block entries
      </p>
      <div className="my-2">
        <StorageMeter
          usage={storage.originUsageBytes}
          quota={storage.originQuotaBytes}
          compact
        />
      </div>
      <p>Cleanup protection: {protectionLabel(storage.protectionState)}</p>
    </SystemTile>
  )
}

// NetworkTile shows peers, links, transport state, and this device's peer.
function NetworkTile({
  base,
  network,
}: {
  base: TileBase
  network: SystemModel['network']
}) {
  if (!network) {
    return (
      <SystemTile {...base} className="@2xl:col-span-2" headline="Reading…" />
    )
  }

  return (
    <SystemTile
      {...base}
      className="@2xl:col-span-2"
      headline={plural(network.peerCount ?? 0, 'peer')}
    >
      <p>
        {plural(network.linkCount ?? 0, 'link')} · Transport{' '}
        <span className={cn(!network.transportRunning && 'text-warning')}>
          {network.transportRunning ? 'running' : 'stopped'}
        </span>
      </p>
      {network.localPeerId && (
        <p className="mt-1 truncate">
          This device{' '}
          <span className="font-mono">{shortId(network.localPeerId)}</span>
        </p>
      )}
    </SystemTile>
  )
}

// runtimeHeadline summarizes the plugins running across every plugin host.
function runtimeHeadline(plugins: SystemModel['plugins'], running: number) {
  if (!plugins) return 'Reading…'
  if (plugins.length === 0) return 'No plugins running'
  const headline = `${formatCount(running)} of ${plural(plugins.length, 'plugin')} running`
  const spaces = new Set(
    plugins.filter((plugin) => plugin.spaceId).map((plugin) => plugin.spaceId),
  ).size
  return spaces ? `${headline} in ${plural(spaces, 'Space')}` : headline
}

// RuntimeTile shows plugin health and the busiest directive types.
function RuntimeTile({ base, model }: { base: TileBase; model: SystemModel }) {
  const plugins = model.plugins ?? []
  const running = plugins.filter((plugin) => plugin.state === 'running').length
  const busiest = (model.directives ?? []).slice(0, 3)

  return (
    <SystemTile
      {...base}
      className="@2xl:col-span-4"
      headline={runtimeHeadline(model.plugins, running)}
    >
      <p className="font-mono tabular-nums">
        {plural(model.controllers?.length ?? 0, 'controller')} ·{' '}
        {plural(model.directiveCount, 'directive')}
      </p>
      {busiest.length > 0 && (
        <ol className="mt-2 space-y-1" aria-label="Busiest directive types">
          {busiest.map((group) => (
            <li key={group.name} className="flex items-center gap-3">
              <span className="min-w-0 flex-1 truncate font-mono">
                {group.name}
              </span>
              <span
                className="bg-foreground/8 h-1 w-20 shrink-0 overflow-hidden rounded-full"
                aria-hidden="true"
              >
                <span
                  className="bg-foreground-alt/50 progress-width block h-full rounded-full"
                  style={{
                    '--progress-width': `${(group.idents.length / busiest[0].idents.length) * 100}%`,
                  }}
                />
              </span>
              <span className="w-10 shrink-0 text-right font-mono tabular-nums">
                {formatCount(group.idents.length)}
              </span>
            </li>
          ))}
        </ol>
      )}
    </SystemTile>
  )
}

// ReleaseTile shows this version, the update phase, and the runtime load.
function ReleaseTile({ base, model }: { base: TileBase; model: SystemModel }) {
  const launcher = model.recovery?.launcher
  const asset = model.recovery?.runtimeAsset
  let update = 'No launcher in this runtime'
  if (!model.recovery) {
    update = 'Reading…'
  } else if (launcher) {
    update = updatePhaseLabel(launcher.updatePhase)
  }

  return (
    <SystemTile
      {...base}
      className="@2xl:col-span-2"
      headline={
        <span className="font-mono">{model.build.version || 'dev'}</span>
      }
    >
      <p
        className={cn(launcher?.updatePhase === 'error' && 'text-destructive')}
      >
        {update}
      </p>
      {asset?.status === 'reported' && (
        <p className={cn('mt-1', !asset.ok && 'text-destructive')}>
          {asset.ok ? 'Runtime loaded cleanly' : 'Runtime loaded with errors'}
        </p>
      )}
    </SystemTile>
  )
}

// SpacesTile shows every account session and the first Spaces it holds.
function SpacesTile({
  base,
  sessions,
  spaces,
}: {
  base: TileBase
  sessions: SystemModel['sessions']
  spaces: SystemModel['spaces']
}) {
  if (!sessions || !spaces) {
    return (
      <SystemTile {...base} className="@2xl:col-span-6" headline="Reading…" />
    )
  }

  const shown = spaces.slice(0, 8)
  return (
    <SystemTile
      {...base}
      className="@2xl:col-span-6"
      headline={`${plural(spaces.length, 'Space')} · ${plural(sessions.length, 'account')}`}
    >
      <ul className="flex flex-wrap gap-1.5">
        {sessions.map((session) => (
          <SessionChip
            key={session.sessionIndex}
            sessionIndex={session.sessionIndex ?? 0}
          />
        ))}
        {shown.map((space) => (
          <li
            key={spaceEngineId(space)}
            className="border-foreground/8 max-w-48 truncate rounded-md border px-2 py-1"
          >
            {space.spaceMeta?.name || 'Untitled Space'}
          </li>
        ))}
        {spaces.length > shown.length && (
          <li className="px-2 py-1">
            +{formatCount(spaces.length - shown.length)} more
          </li>
        )}
      </ul>
    </SystemTile>
  )
}

// SessionChip names one account session, marking the current one.
function SessionChip({ sessionIndex }: { sessionIndex: number }) {
  const metadata = useSessionMetadata(sessionIndex)
  const current = useSessionIndex() === sessionIndex

  return (
    <li
      className={cn(
        'flex max-w-56 items-center gap-1.5 rounded-md border px-2 py-1',
        current
          ? 'border-brand/30 bg-brand/10 text-foreground'
          : 'border-foreground/8',
      )}
    >
      <span className="truncate">
        {metadata?.displayName || `Session ${sessionIndex}`}
      </span>
      <span className="text-foreground-alt/50 font-mono">
        /u/{sessionIndex}
      </span>
    </li>
  )
}

// UnderTheHood opens the devtools trees for SDK resources and state atoms.
function UnderTheHood({
  tileRef,
  onOpen,
}: {
  tileRef: TileRef
  onOpen: (id: InspectorId) => void
}) {
  const resourceCount = useTrackedResources().size
  const atomCount = useStateInspectorEntryMap().size

  return (
    <div className="flex flex-wrap items-center gap-2 px-4 pb-4">
      <h2 className="text-foreground-alt/60 mr-1 text-xs">Under the hood</h2>
      <DashboardButton
        ref={tileRef('resources')}
        icon={<LuLayers className="size-3.5" />}
        onClick={() => onOpen('resources')}
      >
        Resources
        <span className="text-foreground-alt/50 font-mono tabular-nums">
          {formatCount(resourceCount)}
        </span>
      </DashboardButton>
      <DashboardButton
        ref={tileRef('atoms')}
        icon={<LuBox className="size-3.5" />}
        onClick={() => onOpen('atoms')}
      >
        State atoms
        <span className="text-foreground-alt/50 font-mono tabular-nums">
          {formatCount(atomCount)}
        </span>
      </DashboardButton>
    </div>
  )
}

// InspectorNav returns to the tiles or hops straight to another inspector.
function InspectorNav({
  current,
  tones,
  onSelect,
  onBack,
}: {
  current: InspectorId
  tones: Record<SubsystemId, SystemTone>
  onSelect: (id: InspectorId) => void
  onBack: () => void
}) {
  return (
    <nav
      aria-label="System inspectors"
      className="border-foreground/8 flex h-10 shrink-0 items-center gap-1 overflow-x-auto border-b px-2"
    >
      <button
        type="button"
        onClick={onBack}
        className="text-foreground-alt/70 hover:text-foreground hover:bg-foreground/5 focus-visible:ring-brand/50 flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors focus-visible:ring-2 focus-visible:outline-none"
      >
        <LuArrowLeft className="size-3.5" aria-hidden="true" />
        All systems
      </button>
      <span
        className="bg-foreground/10 mx-1 h-4 w-px shrink-0"
        aria-hidden="true"
      />
      {inspectorOrder.map((id) => {
        const tone = id in tones ? tones[id as SubsystemId] : null
        return (
          <button
            key={id}
            type="button"
            aria-current={id === current ? 'page' : undefined}
            onClick={() => onSelect(id)}
            className={cn(
              'focus-visible:ring-brand/50 flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors focus-visible:ring-2 focus-visible:outline-none',
              id === current
                ? 'bg-foreground/8 text-foreground'
                : 'text-foreground-alt/60 hover:text-foreground',
            )}
          >
            {tone && (
              <span
                className={cn('size-1.5 rounded-full', toneDotClass[tone])}
                aria-hidden="true"
              />
            )}
            <span className={cn(tone === 'error' && toneTextClass.error)}>
              {inspectorMeta[id].title}
            </span>
          </button>
        )
      })}
      <span className="text-foreground-alt/45 ml-auto hidden shrink-0 px-2 text-xs @xl:block">
        <kbd className="font-sans">Esc</kbd> to go back
      </span>
    </nav>
  )
}
