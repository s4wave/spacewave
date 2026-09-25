import { useMemo } from 'react'

import { useAppBuildInfo, type AppBuildInfo } from '@s4wave/app/build-info.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import {
  useSessionSyncStatus,
  type SessionSyncStatusView,
} from '@s4wave/app/session/SessionSyncStatusContext.js'
import {
  useStorageHealth,
  type StorageHealthView,
} from '@s4wave/app/session/storage/useStorageHealth.js'
import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import type {
  ControllerInfo,
  NativePackageRecoveryStatus,
  RecoveryStatus,
  WatchControllersResponse,
  WatchDirectivesResponse,
  WatchNetworkStatsResponse,
  WatchPluginsResponse,
} from '@s4wave/sdk/status/status.pb.js'
import type { PluginManifestRecoveryStatus } from '@go/github.com/s4wave/spacewave/bldr/plugin/plugin.pb.js'

import {
  useWatchControllers,
  useWatchDirectives,
  useWatchNetworkStats,
  useWatchPlugins,
  useWatchRecoveryStatus,
  useWatchSpacesList,
} from './useSystemStatus.js'

// SubsystemId names one tile of the system dashboard.
export type SubsystemId =
  | 'sync'
  | 'storage'
  | 'network'
  | 'runtime'
  | 'release'
  | 'spaces'

// SystemTone is the health reading of a subsystem or of the whole system.
// Pending means the live watch has not delivered its first snapshot.
export type SystemTone = 'nominal' | 'active' | 'warning' | 'error' | 'pending'

// AttentionItem is one live fact that needs the user's attention, attributed
// to the subsystem whose inspector explains it.
export interface AttentionItem {
  key: string
  subsystem: SubsystemId
  tone: 'warning' | 'error'
  title: string
  detail: string
}

// SystemVerdict summarizes the whole system in one headline.
export interface SystemVerdict {
  tone: SystemTone
  headline: string
  items: AttentionItem[]
}

// PluginView joins a plugin host instance with the Space it serves and, for
// root host plugins, its retained manifest recovery and native package facts.
export interface PluginView {
  key: string
  id: string
  instanceKey: string
  state: string
  // spaceId is the engine ID of the Space the instance serves, empty for
  // system plugins.
  spaceId: string
  // host names the Space the instance serves, or the system.
  host: string
  manifest?: PluginManifestRecoveryStatus
  nativePackage?: NativePackageRecoveryStatus
}

// DirectiveGroup is every active directive instance of one directive type.
export interface DirectiveGroup {
  name: string
  idents: string[]
}

// SystemModel is the complete live picture of the running system. Nullable
// fields are null until their watch delivers its first snapshot.
export interface SystemModel {
  build: AppBuildInfo
  sync: SessionSyncStatusView
  storage: StorageHealthView
  network: WatchNetworkStatsResponse | null
  controllers: ControllerInfo[] | null
  directives: DirectiveGroup[] | null
  directiveCount: number
  plugins: PluginView[] | null
  recovery: RecoveryStatus | null
  sessions: SessionListEntry[] | null
  spaces: SpaceSoListEntry[] | null
  tones: Record<SubsystemId, SystemTone>
  verdict: SystemVerdict
}

// SystemModelInputs are the raw snapshots composed into a SystemModel.
export interface SystemModelInputs {
  build: AppBuildInfo
  sync: SessionSyncStatusView
  storage: StorageHealthView
  network: WatchNetworkStatsResponse | null
  controllers: WatchControllersResponse | null
  directives: WatchDirectivesResponse | null
  plugins: WatchPluginsResponse | null
  recovery: RecoveryStatus | null
  sessions: SessionListEntry[] | null
  spaces: ReadonlyArray<SpaceSoListEntry> | null
}

// storageQuotaWarningRatio is the browser quota fraction above which new
// writes are at risk of failing.
const storageQuotaWarningRatio = 0.9

// useSystemModel composes every live system watch in the current session into
// one SystemModel. Each source keeps its own subscription lifetime; the model
// updates whenever any source pushes a new snapshot.
export function useSystemModel(): SystemModel {
  const build = useAppBuildInfo()
  const sync = useSessionSyncStatus()
  const storage = useStorageHealth()
  const network = useWatchNetworkStats()
  const controllers = useWatchControllers()
  const directives = useWatchDirectives()
  const plugins = useWatchPlugins()
  const recovery = useWatchRecoveryStatus()
  const sessionList = useSessionList()
  const spaces = useWatchSpacesList()
  const sessions = sessionList.value?.sessions ?? null

  return useMemo(
    () =>
      buildSystemModel({
        build,
        sync,
        storage,
        network,
        controllers,
        directives,
        plugins,
        recovery,
        sessions,
        spaces,
      }),
    [
      build,
      sync,
      storage,
      network,
      controllers,
      directives,
      plugins,
      recovery,
      sessions,
      spaces,
    ],
  )
}

// buildSystemModel derives the dashboard model, attention items, and verdict
// from raw snapshots. It never invents a reading: a missing snapshot yields a
// pending tone, not a healthy one.
export function buildSystemModel(inputs: SystemModelInputs): SystemModel {
  // Project the runtime snapshots into display order.
  const pluginViews = inputs.plugins
    ? joinPlugins(inputs.plugins, inputs.recovery, inputs.spaces)
    : null
  const directiveGroups = inputs.directives
    ? groupDirectives(inputs.directives)
    : null
  const controllers = inputs.controllers
    ? [...(inputs.controllers.controllers ?? [])].sort((a, b) =>
        (a.id ?? '').localeCompare(b.id ?? ''),
      )
    : null

  // Collect the facts that need attention and fold them into tones.
  const items = collectAttention(inputs, pluginViews)
  const tones = buildTones(inputs, items)

  return {
    build: inputs.build,
    sync: inputs.sync,
    storage: inputs.storage,
    network: inputs.network,
    controllers,
    directives: directiveGroups,
    directiveCount: inputs.directives?.directiveCount ?? 0,
    plugins: pluginViews,
    recovery: inputs.recovery,
    sessions: inputs.sessions,
    spaces: inputs.spaces ? [...inputs.spaces] : null,
    tones,
    verdict: buildVerdict(tones, items),
  }
}

// updatePhaseLabel describes a launcher update phase in plain language.
export function updatePhaseLabel(phase: string | undefined): string {
  switch (phase) {
    case 'idle':
      return 'No update pending'
    case 'downloading':
      return 'Downloading update'
    case 'staged':
      return 'Update ready'
    case 'applying':
      return 'Applying update'
    case 'error':
      return 'Update failed'
    default:
      return 'Update state not reported'
  }
}

// groupDirectives groups directive instances by type, largest group first.
function groupDirectives(resp: WatchDirectivesResponse): DirectiveGroup[] {
  const groups = new Map<string, string[]>()
  for (const directive of resp.directives ?? []) {
    const name = directive.name || 'unknown'
    const idents = groups.get(name)
    if (idents) {
      idents.push(directive.ident ?? '')
    } else {
      groups.set(name, [directive.ident ?? ''])
    }
  }
  return Array.from(groups, ([name, idents]) => ({ name, idents })).sort(
    (a, b) => b.idents.length - a.idents.length || a.name.localeCompare(b.name),
  )
}

// joinPlugins names the Space each plugin instance serves and attaches the
// root host's manifest recovery and native package facts. Plugins are ordered
// with system plugins first, then by Space name.
function joinPlugins(
  resp: WatchPluginsResponse,
  recovery: RecoveryStatus | null,
  spaces: ReadonlyArray<SpaceSoListEntry> | null,
): PluginView[] {
  const manifests = new Map(
    (recovery?.plugins ?? []).map((m) => [
      `${m.pluginId ?? ''}:${m.instanceKey ?? ''}`,
      m,
    ]),
  )
  const nativePackages = new Map(
    (recovery?.nativePackages ?? []).map((p) => [p.pluginId ?? '', p]),
  )
  const spaceNames = new Map(
    (spaces ?? []).map((space) => [
      spaceEngineId(space),
      space.spaceMeta?.name || 'Untitled Space',
    ]),
  )

  return (resp.plugins ?? [])
    .map((plugin) => {
      const id = plugin.id ?? ''
      const instanceKey = plugin.instanceKey ?? ''
      const spaceId = plugin.spaceId ?? ''
      const rootKey = `${id}:${instanceKey}`
      return {
        key: `${spaceId}:${rootKey}`,
        id,
        instanceKey,
        state: plugin.state || 'unknown',
        spaceId,
        host: spaceId
          ? (spaceNames.get(spaceId) ?? 'Unlisted Space')
          : 'System',
        // Recovery rows come from the root host, which keys its Space
        // instances by Space engine ID.
        manifest:
          !spaceId || instanceKey === spaceId
            ? manifests.get(rootKey)
            : undefined,
        nativePackage: spaceId ? undefined : nativePackages.get(id),
      }
    })
    .sort(
      (a, b) =>
        Number(!!a.spaceId) - Number(!!b.spaceId) ||
        a.host.localeCompare(b.host) ||
        a.key.localeCompare(b.key),
    )
}

// spaceEngineId builds the world engine ID a Space runtime uses as its plugin
// scheduler instance key. It mirrors SpaceEngineId in core/space and is
// unique per Space.
export function spaceEngineId(space: SpaceSoListEntry): string {
  const ref = space.entry?.ref?.providerResourceRef
  return [
    'space',
    ref?.providerId ?? '',
    ref?.providerAccountId ?? '',
    ref?.id ?? '',
  ].join('/')
}

// collectAttention lists every live fault or risk, most severe first.
function collectAttention(
  inputs: SystemModelInputs,
  plugins: PluginView[] | null,
): AttentionItem[] {
  const items: AttentionItem[] = []

  // Sync reports its own failure.
  if (inputs.sync.error) {
    items.push({
      key: 'sync-error',
      subsystem: 'sync',
      tone: 'error',
      title: 'Sync stopped on an error',
      detail: inputs.sync.lastError || inputs.sync.detailLabel,
    })
  }

  // The browser quota is close to full.
  const { originUsageBytes, originQuotaBytes } = inputs.storage
  if (
    originUsageBytes != null &&
    originQuotaBytes != null &&
    originQuotaBytes > 0 &&
    originUsageBytes / originQuotaBytes >= storageQuotaWarningRatio
  ) {
    items.push({
      key: 'storage-quota',
      subsystem: 'storage',
      tone: 'warning',
      title: 'Browser storage is almost full',
      detail: 'New saves can fail once the browser quota is reached.',
    })
  }

  // The session transport is not running.
  if (inputs.network && !inputs.network.transportRunning) {
    items.push({
      key: 'network-stopped',
      subsystem: 'network',
      tone: 'warning',
      title: 'Peer transport is not running',
      detail: 'This device cannot reach peers until the transport starts.',
    })
  }

  // The launcher failed to fetch or apply a release.
  const launcher = inputs.recovery?.launcher
  if (launcher?.updatePhase === 'error') {
    items.push({
      key: 'release-update',
      subsystem: 'release',
      tone: 'error',
      title: 'The last update failed',
      detail: launcher.updateError || 'The launcher reported an update error.',
    })
  }

  // The runtime entry asset loaded with an error.
  const asset = inputs.recovery?.runtimeAsset
  if (asset?.status === 'reported' && !asset.ok) {
    items.push({
      key: 'release-asset',
      subsystem: 'release',
      tone: 'error',
      title: 'The runtime loaded from a fallback',
      detail:
        asset.runtimeError ||
        asset.classification ||
        `The runtime asset returned status ${asset.statusCode ?? 0}.`,
    })
  }

  // Plugins with quarantined manifests or failed native packages.
  for (const plugin of plugins ?? []) {
    if (plugin.manifest?.quarantinedCandidateCount) {
      items.push({
        key: `plugin-quarantine:${plugin.key}`,
        subsystem: 'runtime',
        tone: 'warning',
        title: `${plugin.id} skipped a quarantined manifest`,
        detail:
          plugin.manifest.quarantinedCandidateSummary ||
          'A plugin manifest candidate was quarantined.',
      })
    }
    if (plugin.nativePackage?.lastError) {
      items.push({
        key: `plugin-native:${plugin.key}`,
        subsystem: 'runtime',
        tone: 'warning',
        title: `${plugin.id} native package failed`,
        detail: plugin.nativePackage.lastError,
      })
    }
  }

  return items.sort((a, b) =>
    a.tone === b.tone ? 0 : a.tone === 'error' ? -1 : 1,
  )
}

// buildTones reads each subsystem's tone from its attention items, then from
// its own activity and loading state.
function buildTones(
  inputs: SystemModelInputs,
  items: AttentionItem[],
): Record<SubsystemId, SystemTone> {
  const worst = (subsystem: SubsystemId): SystemTone | null => {
    const tones = items.filter((item) => item.subsystem === subsystem)
    if (tones.some((item) => item.tone === 'error')) {
      return 'error'
    }
    return tones.length ? 'warning' : null
  }

  return {
    sync:
      worst('sync') ??
      (inputs.sync.loading
        ? 'pending'
        : inputs.sync.active
          ? 'active'
          : 'nominal'),
    storage:
      worst('storage') ??
      (inputs.storage.providerLoading ? 'pending' : 'nominal'),
    network: worst('network') ?? (inputs.network ? 'nominal' : 'pending'),
    runtime:
      worst('runtime') ??
      (inputs.plugins && inputs.controllers && inputs.directives
        ? 'nominal'
        : 'pending'),
    release:
      worst('release') ??
      (!inputs.recovery
        ? 'pending'
        : inputs.recovery.launcher?.updatePhase === 'downloading' ||
            inputs.recovery.launcher?.updatePhase === 'applying'
          ? 'active'
          : 'nominal'),
    spaces:
      worst('spaces') ??
      (inputs.sessions && inputs.spaces ? 'nominal' : 'pending'),
  }
}

// buildVerdict folds the subsystem tones into one headline.
function buildVerdict(
  tones: Record<SubsystemId, SystemTone>,
  items: AttentionItem[],
): SystemVerdict {
  const values = Object.values(tones)
  if (items.some((item) => item.tone === 'error')) {
    return { tone: 'error', headline: attentionHeadline(items), items }
  }
  if (items.length) {
    return { tone: 'warning', headline: attentionHeadline(items), items }
  }
  if (values.includes('pending')) {
    return { tone: 'pending', headline: 'Reading system state', items }
  }
  if (values.includes('active')) {
    return { tone: 'active', headline: 'All systems nominal', items }
  }
  return { tone: 'nominal', headline: 'All systems nominal', items }
}

// attentionHeadline counts the items that need attention.
function attentionHeadline(items: AttentionItem[]): string {
  return items.length === 1
    ? '1 thing needs attention'
    : `${items.length} things need attention`
}
