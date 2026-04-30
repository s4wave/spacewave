import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  LuArrowLeft,
  LuBox,
  LuChevronDown,
  LuChevronRight,
  LuCpu,
  LuFolderOpen,
  LuLayers,
  LuLock,
  LuLockOpen,
  LuPuzzle,
  LuRadar,
  LuTerminal,
  LuUser,
  LuX,
} from 'react-icons/lu'

import { List as VirtualList, type RowComponentProps } from 'react-window'

import { cn } from '@s4wave/web/style/utils.js'
import { useAppBuildInfo, type AppBuildInfo } from '@s4wave/app/build-info.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import {
  useStateAtom,
  useStateNamespace,
  type StateNamespace,
} from '@s4wave/web/state/index.js'
import { ResourceTreeTab } from '@s4wave/web/devtools/ResourceTreeTab.js'
import { ResourceDetailsPanel } from '@s4wave/web/devtools/ResourceDetailsPanel.js'
import { StateDetailsPanel } from '@s4wave/web/devtools/StateDetailsPanel.js'
import {
  useSelectedResourceId,
  useTrackedResources,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'
import { StateTreeTab } from '@s4wave/web/devtools/StateTreeTab.js'
import { useSelectedStateAtomId } from '@s4wave/web/devtools/StateDevToolsContext.js'
import { useStateInspectorEntryMap } from '@s4wave/web/devtools/useStateInspectorEntries.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import {
  useWatchSpacesList,
  useWatchControllers,
  useWatchDirectives,
  useWatchPlugins,
} from './useSystemStatus.js'
import { useIsMobile } from '@s4wave/web/hooks/useMobile.js'

function formatTimestamp(ms: number): string {
  const d = new Date(ms)
  return d.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

// Group flat directives by name for display.
function groupDirectives(
  directives: ReadonlyArray<{ name?: string; ident?: string }>,
) {
  const groups = new Map<string, string[]>()
  for (const d of directives) {
    const name = d.name || 'unknown'
    const arr = groups.get(name)
    if (arr) arr.push(d.ident || '')
    else groups.set(name, [d.ident || ''])
  }
  return Array.from(groups.entries())
    .map(([name, idents]) => ({ name, idents, count: idents.length }))
    .sort((a, b) => b.count - a.count)
}

function makeOccurrenceKey(id: string | undefined, index: number): string {
  return `${id || 'unknown'}:${index}`
}

const TRANSIENT_FEEDBACK_MS = 1600
const STATS_KEYS = ['acct', 'spc', 'plug', 'ctrl', 'dir'] as const
// SHOW_LOG_PANEL keeps the placeholder logs drawer hidden until a real log
// stream backs the designed tray.
const SHOW_LOG_PANEL = false

type StatsKey = (typeof STATS_KEYS)[number]

function useCountDeltas(
  counts: Record<StatsKey, number>,
): Partial<Record<StatsKey, number>> {
  const prevRef = useRef<Record<StatsKey, number> | null>(null)
  const timeoutRef = useRef<Partial<Record<StatsKey, number>>>({})
  const [deltas, setDeltas] = useState<Partial<Record<StatsKey, number>>>({})

  useEffect(() => {
    const prev = prevRef.current
    prevRef.current = counts
    if (!prev) return

    for (const key of STATS_KEYS) {
      const delta = counts[key] - prev[key]
      if (!delta) continue

      setDeltas((state) => ({ ...state, [key]: delta }))

      const timeoutId = timeoutRef.current[key]
      if (timeoutId != null) {
        window.clearTimeout(timeoutId)
      }

      timeoutRef.current[key] = window.setTimeout(() => {
        setDeltas((state) => {
          if (!(key in state)) return state
          const next = { ...state }
          delete next[key]
          return next
        })
      }, TRANSIENT_FEEDBACK_MS)
    }
  }, [counts])

  useEffect(() => {
    const timeouts = timeoutRef.current
    return () => {
      for (const timeoutId of Object.values(timeouts)) {
        if (timeoutId == null) continue
        window.clearTimeout(timeoutId)
      }
    }
  }, [])

  return deltas
}

function useFreshKeys(ids: ReadonlyArray<string>): Set<string> {
  const snapshotKey = useMemo(() => ids.join('\u0000'), [ids])
  const prevRef = useRef<Set<string> | null>(null)
  const timeoutRef = useRef<Record<string, number>>({})
  const [freshIds, setFreshIds] = useState<string[]>([])

  useEffect(() => {
    const nextIds = new Set(ids.filter(Boolean))
    const prevIds = prevRef.current
    prevRef.current = nextIds
    if (!prevIds) return

    const additions = Array.from(nextIds).filter((id) => !prevIds.has(id))
    if (!additions.length) return

    setFreshIds((state) => Array.from(new Set([...state, ...additions])))

    for (const id of additions) {
      const timeoutId = timeoutRef.current[id]
      if (timeoutId != null) {
        window.clearTimeout(timeoutId)
      }
      timeoutRef.current[id] = window.setTimeout(() => {
        setFreshIds((state) => state.filter((entry) => entry !== id))
      }, TRANSIENT_FEEDBACK_MS)
    }
  }, [ids, snapshotKey])

  useEffect(() => {
    const timeouts = timeoutRef.current
    return () => {
      for (const timeoutId of Object.values(timeouts)) {
        window.clearTimeout(timeoutId)
      }
    }
  }, [])

  return useMemo(() => new Set(freshIds), [freshIds])
}

function useSnapshotUpdatedAt(snapshotKey: string): string {
  return snapshotKey
}

function buildSpacesSnapshotKey(
  spaces: ReadonlyArray<{
    entry?: { ref?: { providerResourceRef?: { id?: string } }; source?: string }
    spaceMeta?: { name?: string }
  }>,
): string {
  return spaces
    .map((space) =>
      [
        space.entry?.ref?.providerResourceRef?.id ?? '',
        space.spaceMeta?.name ?? '',
        space.entry?.source ?? '',
      ].join(':'),
    )
    .join('|')
}

function buildControllersSnapshotKey(
  controllers: ReadonlyArray<{
    id?: string
    version?: string
    description?: string
  }>,
): string {
  return controllers
    .map((controller) =>
      [
        controller.id ?? '',
        controller.version ?? '',
        controller.description ?? '',
      ].join(':'),
    )
    .join('|')
}

function buildDirectivesSnapshotKey(
  directives: ReadonlyArray<{ name?: string; ident?: string }>,
): string {
  return directives
    .map((directive) => [directive.name ?? '', directive.ident ?? ''].join(':'))
    .join('|')
}

function buildPluginsSnapshotKey(
  plugins: ReadonlyArray<{ id?: string; instanceKey?: string; state?: string }>,
): string {
  return plugins
    .map((plugin) =>
      [plugin.id ?? '', plugin.instanceKey ?? '', plugin.state ?? ''].join(':'),
    )
    .join('|')
}

// Selection type for tree navigation.
type Selection =
  | { kind: 'session'; index: number }
  | { kind: 'space'; id: string }
  | { kind: 'controller'; id: string; index: number }
  | { kind: 'plugin'; id: string; instanceKey: string }
  | { kind: 'directive-group'; name: string }
  | { kind: 'spaces' }
  | { kind: 'plugins' }
  | { kind: 'controllers' }
  | { kind: 'directives' }
  | { kind: 'resources' }
  | { kind: 'atoms' }

type SidebarSectionKey =
  | 'accounts'
  | 'spaces'
  | 'plugins'
  | 'controllers'
  | 'directives'
  | 'resources'
  | 'atoms'

const DEFAULT_SELECTED: Selection = { kind: 'controllers' }

const DEFAULT_OPEN_SECTIONS: Record<SidebarSectionKey, boolean> = {
  accounts: true,
  spaces: true,
  plugins: true,
  controllers: true,
  directives: true,
  resources: false,
  atoms: false,
}

function getSelectionLabel(
  selected: Selection,
  spaces: ReadonlyArray<{
    entry?: { ref?: { providerResourceRef?: { id?: string } } }
    spaceMeta?: { name?: string }
  }>,
  controllers: ReadonlyArray<{ id?: string }>,
  plugins: ReadonlyArray<{ id?: string; instanceKey?: string }>,
  directiveGroups: ReadonlyArray<{ name: string }>,
): string {
  if (selected.kind === 'session') {
    return `/u/${selected.index}`
  }
  if (selected.kind === 'space') {
    return (
      spaces.find(
        (space) => space.entry?.ref?.providerResourceRef?.id === selected.id,
      )?.spaceMeta?.name ?? 'Space'
    )
  }
  if (selected.kind === 'controller') {
    return controllers[selected.index]?.id ?? 'Controller'
  }
  if (selected.kind === 'plugin') {
    return (
      plugins.find(
        (plugin) =>
          plugin.id === selected.id &&
          (plugin.instanceKey ?? '') === selected.instanceKey,
      )?.id ?? 'Plugin'
    )
  }
  if (selected.kind === 'directive-group') {
    return (
      directiveGroups.find((group) => group.name === selected.name)?.name ??
      'Directive'
    )
  }
  if (selected.kind === 'spaces') {
    return 'Spaces'
  }
  if (selected.kind === 'plugins') {
    return 'Plugins'
  }
  if (selected.kind === 'controllers') {
    return 'Controllers'
  }
  if (selected.kind === 'directives') {
    return 'Directives'
  }
  if (selected.kind === 'resources') {
    return 'Resources'
  }
  return 'State Atoms'
}

export interface SystemStatusDashboardProps {
  onClose?: () => void
}

// SystemStatusDashboard renders the system status overlay with sidebar
// tree navigation and detail panels backed by live streaming data.
export function SystemStatusDashboard({ onClose }: SystemStatusDashboardProps) {
  const ns = useStateNamespace(['system-status-dashboard'])
  const buildInfo = useAppBuildInfo()
  const sessionList = useSessionList()
  const sessions = useMemo(
    () => sessionList.value?.sessions ?? [],
    [sessionList.value?.sessions],
  )
  const watchedSpaces = useWatchSpacesList()
  const spaces = useMemo(() => watchedSpaces ?? [], [watchedSpaces])

  const controllersResp = useWatchControllers()
  const controllerCount = controllersResp?.controllerCount ?? 0
  const controllers = useMemo(
    () => controllersResp?.controllers ?? [],
    [controllersResp?.controllers],
  )

  const directivesResp = useWatchDirectives()
  const directiveCount = directivesResp?.directiveCount ?? 0
  const directives = useMemo(
    () => directivesResp?.directives ?? [],
    [directivesResp?.directives],
  )
  const pluginsResp = useWatchPlugins()
  const pluginCount = pluginsResp?.pluginCount ?? 0
  const plugins = useMemo(
    () => pluginsResp?.plugins ?? [],
    [pluginsResp?.plugins],
  )
  const directiveGroups = useMemo(
    () => groupDirectives(directives),
    [directives],
  )
  const statsCounts = useMemo(
    () => ({
      acct: sessions.length,
      spc: spaces.length,
      plug: pluginCount,
      ctrl: controllerCount,
      dir: directiveCount,
    }),
    [
      sessions.length,
      spaces.length,
      pluginCount,
      controllerCount,
      directiveCount,
    ],
  )
  const statsDeltas = useCountDeltas(statsCounts)
  const statsUpdatedAt = useSnapshotUpdatedAt(
    [
      sessions.length,
      spaces.length,
      pluginCount,
      controllerCount,
      directiveCount,
    ].join('|'),
  )
  const spacesUpdatedAt = useSnapshotUpdatedAt(buildSpacesSnapshotKey(spaces))
  const controllersUpdatedAt = useSnapshotUpdatedAt(
    buildControllersSnapshotKey(controllers),
  )
  const directivesUpdatedAt = useSnapshotUpdatedAt(
    buildDirectivesSnapshotKey(directives),
  )
  const pluginsUpdatedAt = useSnapshotUpdatedAt(
    buildPluginsSnapshotKey(plugins),
  )
  const isMobile = useIsMobile()

  const [selected, setSelected] = useStateAtom(ns, 'selected', DEFAULT_SELECTED)
  const [mobilePickerOpen, setMobilePickerOpen] = useState(false)
  const selectedLabel = getSelectionLabel(
    selected,
    spaces,
    controllers,
    plugins,
    directiveGroups,
  )
  const mobilePickerVisible = isMobile && mobilePickerOpen

  useEffect(() => {
    if (selected.kind === 'session') {
      const hasSession = sessions.some(
        (session) => (session.sessionIndex ?? 0) === selected.index,
      )
      if (!hasSession) {
        setSelected(DEFAULT_SELECTED)
      }
      return
    }
    if (selected.kind === 'space') {
      const hasSpace = spaces.some(
        (space) => space.entry?.ref?.providerResourceRef?.id === selected.id,
      )
      if (!hasSpace) {
        setSelected({ kind: 'spaces' })
      }
      return
    }
    if (selected.kind === 'plugin') {
      const hasPlugin = plugins.some(
        (plugin) =>
          plugin.id === selected.id &&
          (plugin.instanceKey ?? '') === selected.instanceKey,
      )
      if (!hasPlugin) {
        setSelected({ kind: 'plugins' })
      }
      return
    }
    if (selected.kind === 'directive-group') {
      const hasDirectiveGroup = directiveGroups.some(
        (group) => group.name === selected.name,
      )
      if (!hasDirectiveGroup) {
        setSelected({ kind: 'directives' })
      }
      return
    }
    if (selected.kind !== 'controller') return
    const controller = controllers[selected.index]
    if (controller?.id === selected.id) return
    setSelected({ kind: 'controllers' })
  }, [
    controllers,
    directiveGroups,
    plugins,
    selected,
    sessions,
    setSelected,
    spaces,
  ])

  function handleSelect(next: Selection) {
    setSelected(next)
    if (!isMobile) return
    setMobilePickerOpen(false)
  }

  return (
    <div className="bg-background flex h-full w-full flex-col overflow-hidden">
      {/* Header */}
      <div className="border-foreground/8 flex h-9 shrink-0 items-center justify-between border-b px-4">
        <span className="text-foreground text-sm font-semibold tracking-tight select-none">
          System Status
        </span>
        {onClose && (
          <button
            type="button"
            onClick={onClose}
            className="text-foreground-alt hover:text-foreground transition-colors"
          >
            <LuX className="h-4 w-4" />
          </button>
        )}
      </div>

      {/* Stats ribbon */}
      <StatsRibbon
        sessionCount={sessions.length}
        spaceCount={spaces.length}
        pluginCount={pluginCount}
        controllerCount={controllerCount}
        directiveCount={directiveCount}
        deltas={statsDeltas}
        updatedAt={statsUpdatedAt}
      />

      <div className={cn('flex min-h-0 flex-1', isMobile && 'flex-col')}>
        {!isMobile && (
          <SidebarTree
            sessions={sessions}
            spaces={spaces}
            plugins={plugins}
            controllers={controllers}
            directiveGroups={directiveGroups}
            controllerCount={controllerCount}
            directiveCount={directiveCount}
            pluginCount={pluginCount}
            selected={selected}
            onSelect={handleSelect}
          />
        )}

        <div className="flex min-w-0 flex-1 flex-col">
          {isMobile && (
            <div className="border-foreground/6 border-b px-4 py-3">
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => setMobilePickerOpen(!mobilePickerVisible)}
                  className="border-foreground/8 text-foreground hover:bg-foreground/[0.03] focus-visible:ring-brand/30 rounded-md border px-3 py-1.5 text-xs transition-colors focus-visible:ring-1 focus-visible:outline-none"
                >
                  {mobilePickerVisible ? 'Hide Sections' : 'Sections'}
                </button>
                <span className="text-foreground-alt/45 truncate text-[0.6rem]">
                  {selectedLabel}
                </span>
              </div>
              {mobilePickerVisible && (
                <div className="border-foreground/6 bg-background-card/20 mt-3 max-h-72 overflow-auto rounded-md border">
                  <SidebarTree
                    sessions={sessions}
                    spaces={spaces}
                    plugins={plugins}
                    controllers={controllers}
                    directiveGroups={directiveGroups}
                    controllerCount={controllerCount}
                    directiveCount={directiveCount}
                    pluginCount={pluginCount}
                    selected={selected}
                    onSelect={handleSelect}
                    className="w-full border-r-0"
                  />
                </div>
              )}
            </div>
          )}

          <div className="min-h-0 flex-1 overflow-auto">
            <DetailView
              selected={selected}
              spaces={spaces}
              plugins={plugins}
              controllers={controllers}
              directiveGroups={directiveGroups}
              controllerCount={controllerCount}
              directives={directives}
              directiveCount={directiveCount}
              pluginCount={pluginCount}
              onSelect={handleSelect}
              onClose={onClose}
              buildInfo={buildInfo}
              spacesUpdatedAt={spacesUpdatedAt}
              pluginsUpdatedAt={pluginsUpdatedAt}
              controllersUpdatedAt={controllersUpdatedAt}
              directivesUpdatedAt={directivesUpdatedAt}
            />
          </div>

          {SHOW_LOG_PANEL && <LogPanel namespace={ns} />}
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Shared components
// ---------------------------------------------------------------------------

// Detail card with colored top accent border.
function DetailCard({
  title,
  accent,
  children,
}: {
  title: string
  accent: string
  children: ReactNode
}) {
  return (
    <div className="border-foreground/6 bg-background-card/20 overflow-hidden rounded-md border">
      <div className={cn('rounded-t-md border-t-2', accent)} />
      <div className="flex items-center gap-2 px-3 py-1.5">
        <span className="text-foreground text-xs font-medium">{title}</span>
      </div>
      <div className="border-foreground/6 border-t">{children}</div>
    </div>
  )
}

// Key-value row inside a detail card.
function DetailRow({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex items-baseline justify-between px-3 py-1">
      <span className="text-foreground-alt/40 text-[0.55rem] tracking-wider uppercase">
        {label}
      </span>
      <span
        className={cn(
          'text-foreground/70 text-[0.6rem]',
          mono !== false && 'font-mono',
        )}
      >
        {value}
      </span>
    </div>
  )
}

function LiveIndicator({
  updatedAt,
  label = 'Panel',
}: {
  updatedAt: string
  label?: string
}) {
  return (
    <span
      aria-label={`${label} live`}
      data-updated-at={updatedAt}
      className="text-foreground-alt/35 inline-flex items-center gap-1 text-[0.55rem]"
    >
      <span
        key={updatedAt}
        className="bg-success/80 h-1.5 w-1.5 animate-pulse rounded-full"
      />
      <span>Live</span>
    </span>
  )
}

// ---------------------------------------------------------------------------
// Logs drawer
// ---------------------------------------------------------------------------

type LogLevel = 'error' | 'warn' | 'info' | 'debug'

type LogEntry = {
  ts: string
  level: LogLevel
  source: string
  msg: string
}

// MOCK_LOGS is placeholder sample data until a log stream RPC lands.
const MOCK_LOGS: ReadonlyArray<LogEntry> = [
  {
    ts: '14:23:01.123',
    level: 'info',
    source: 'bifrost/transport',
    msg: 'peer connected: 12D3KooWQr...4mXn via websocket',
  },
  {
    ts: '14:23:02.001',
    level: 'info',
    source: 'controllerbus',
    msg: 'controller started: configset/controller v0.0.1',
  },
  {
    ts: '14:23:02.234',
    level: 'warn',
    source: 'hydra/block-gc',
    msg: 'gc cycle skipped: lock contention (12ms)',
  },
  {
    ts: '14:23:04.890',
    level: 'info',
    source: 'bifrost/pubsub',
    msg: 'topic subscription: /s4wave/sync/v1 (3 peers)',
  },
  {
    ts: '14:23:06.345',
    level: 'info',
    source: 'session/mount',
    msg: 'session 0 ready: 24 controllers, 3 volumes',
  },
  {
    ts: '14:23:08.901',
    level: 'error',
    source: 'hydra/block-gc',
    msg: 'failed to compact shard: OPFS lock timeout after 5s',
  },
  {
    ts: '14:23:11.890',
    level: 'info',
    source: 'hydra/blockstore',
    msg: 'sync complete: 47 blocks, 128KB transferred',
  },
  {
    ts: '14:23:12.123',
    level: 'warn',
    source: 'bifrost/transport',
    msg: 'high latency detected: 12D3KooWPk...8jLp (>100ms)',
  },
]

const LOG_FILTERS: ReadonlyArray<LogLevel | null> = [
  null,
  'error',
  'warn',
  'info',
]

function logColor(level: LogLevel): string {
  switch (level) {
    case 'error':
      return 'text-destructive'
    case 'warn':
      return 'text-warning'
    case 'info':
      return 'text-foreground/70'
    case 'debug':
      return 'text-foreground-alt/40'
  }
}

const LOG_ROW_HEIGHT = 17.2
const LOG_LIST_MIN_HEIGHT = 80
const LOG_LIST_MAX_HEIGHT = 160

type LogRowProps = { logs: ReadonlyArray<LogEntry> }

function LogRow({ index, style, logs }: RowComponentProps<LogRowProps>) {
  const log = logs[index]
  return (
    <div
      style={style}
      className={cn(
        'hover:bg-foreground/[0.015] flex items-start gap-0 px-3',
        log.level === 'error' && 'bg-destructive/[0.03]',
      )}
    >
      <span className="text-foreground-alt/20 w-24 shrink-0 text-[0.55rem]">
        {log.ts}
      </span>
      <span
        className={cn(
          'w-10 shrink-0 text-[0.55rem] font-medium',
          logColor(log.level),
        )}
      >
        {log.level}
      </span>
      <span className="text-brand/30 w-32 shrink-0 truncate text-[0.55rem]">
        {log.source}
      </span>
      <span className="text-foreground/50 min-w-0 text-[0.55rem]">
        {log.msg}
      </span>
    </div>
  )
}

// LogPanel is a collapsible bottom drawer showing recent log entries.
// Backed by sample data; swap MOCK_LOGS for a real stream when available.
// Rows are virtualized via react-window at a fixed row height.
function LogPanel({ namespace }: { namespace: StateNamespace }) {
  const [collapsed, setCollapsed] = useStateAtom(
    namespace,
    'logs-collapsed',
    true,
  )
  const [filter, setFilter] = useState<LogLevel | null>(null)
  const filtered = useMemo(
    () => (filter ? MOCK_LOGS.filter((l) => l.level === filter) : MOCK_LOGS),
    [filter],
  )
  const listHeight = Math.max(
    LOG_LIST_MIN_HEIGHT,
    Math.min(filtered.length * LOG_ROW_HEIGHT, LOG_LIST_MAX_HEIGHT),
  )

  return (
    <div className="border-foreground/6 flex flex-col border-t">
      <div className="bg-background-deep/40 flex items-center gap-1.5 px-3 py-1">
        <button
          type="button"
          onClick={() => setCollapsed(!collapsed)}
          className="hover:text-foreground-alt/80 flex flex-1 items-center gap-1.5 text-left"
          aria-expanded={!collapsed}
          aria-label={collapsed ? 'Expand logs' : 'Collapse logs'}
        >
          <LuTerminal className="text-foreground-alt/30 h-3 w-3" />
          <span className="text-foreground-alt/50 text-[0.6rem] font-medium">
            Logs
          </span>
          <span className="bg-foreground/5 text-foreground-alt/40 rounded px-1 py-0.5 font-mono text-[0.45rem] tracking-wider uppercase">
            sample
          </span>
          <LuChevronDown
            className={cn(
              'text-foreground-alt/20 h-3 w-3 transition-transform',
              collapsed && '-rotate-90',
            )}
          />
        </button>
        {!collapsed && (
          <div className="flex gap-1">
            {LOG_FILTERS.map((level) => (
              <button
                key={level ?? 'all'}
                type="button"
                onClick={() => setFilter(level)}
                className={cn(
                  'rounded px-1.5 py-0.5 font-mono text-[0.5rem] transition-colors',
                  filter === level ?
                    'bg-foreground/10 text-foreground'
                  : 'text-foreground-alt/30 hover:text-foreground-alt/50',
                )}
              >
                {level ?? 'all'}
              </button>
            ))}
          </div>
        )}
        <span className="text-foreground-alt/15 ml-2 font-mono text-[0.5rem]">
          {filtered.length} entries
        </span>
      </div>
      {!collapsed && (
        <div
          className="bg-background-deep/30 font-mono"
          style={{ height: listHeight }}
        >
          {filtered.length > 0 && (
            <VirtualList
              rowHeight={LOG_ROW_HEIGHT}
              rowCount={filtered.length}
              rowComponent={LogRow}
              rowProps={{ logs: filtered }}
            />
          )}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Stats ribbon
// ---------------------------------------------------------------------------

function StatsRibbon({
  sessionCount,
  spaceCount,
  pluginCount,
  controllerCount,
  directiveCount,
  deltas,
  updatedAt,
}: {
  sessionCount: number
  spaceCount: number
  pluginCount: number
  controllerCount: number
  directiveCount: number
  deltas: Partial<Record<StatsKey, number>>
  updatedAt: string
}) {
  return (
    <div className="border-foreground/6 bg-background-deep/30 flex shrink-0 items-center gap-3 border-b px-4 py-1">
      <StatPill
        label={`${sessionCount} acct`}
        delta={deltas.acct}
        icon={<span className="bg-success h-1.5 w-1.5 rounded-full" />}
      />
      <StatPill
        label={`${spaceCount} spc`}
        delta={deltas.spc}
        icon={<LuFolderOpen className="text-foreground-alt/30 h-2.5 w-2.5" />}
      />
      <StatPill
        label={`${pluginCount} plug`}
        delta={deltas.plug}
        icon={<LuPuzzle className="text-foreground-alt/30 h-2.5 w-2.5" />}
      />
      <StatPill
        label={`${controllerCount} ctrl`}
        delta={deltas.ctrl}
        icon={<LuCpu className="text-foreground-alt/30 h-2.5 w-2.5" />}
      />
      <StatPill
        label={`${directiveCount} dir`}
        delta={deltas.dir}
        icon={<LuRadar className="text-foreground-alt/30 h-2.5 w-2.5" />}
      />
      <div className="ml-auto">
        <LiveIndicator updatedAt={updatedAt} label="Ribbon" />
      </div>
    </div>
  )
}

function StatPill({
  label,
  icon,
  delta,
}: {
  label: string
  icon: ReactNode
  delta?: number
}) {
  return (
    <div className="flex items-center gap-1.5">
      {icon}
      <span className="text-foreground/60 text-[0.6rem]">{label}</span>
      {delta != null && (
        <span
          className={cn(
            'rounded-full px-1 py-0.5 font-mono text-[0.5rem] transition-opacity duration-300',
            delta > 0 ?
              'bg-success/10 text-success/80'
            : 'bg-warning/10 text-warning/80',
          )}
        >
          {delta > 0 ? `+${delta}` : String(delta)}
        </span>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Sidebar tree
// ---------------------------------------------------------------------------

type SidebarEntry =
  | {
      id: string
      kind: 'section'
      section: SidebarSectionKey
      label: string
      count?: number
      expanded: boolean
      level: number
    }
  | {
      id: string
      kind: 'session'
      sessionIndex: number
      level: number
      parentSection: SidebarSectionKey
    }
  | {
      id: string
      kind: 'selection'
      label: string
      sublabel?: string
      dot: string
      level: number
      parentSection: SidebarSectionKey
      selection: Selection
      selected: boolean
    }
  | {
      id: string
      kind: 'more'
      section: 'spaces' | 'controllers' | 'directives'
      label: string
      expanded: boolean
      level: number
      parentSection: SidebarSectionKey
    }

function getSidebarSectionIcon(section: SidebarSectionKey): ReactNode {
  if (section === 'accounts') {
    return <LuUser className="h-3 w-3" />
  }
  if (section === 'spaces') {
    return <LuFolderOpen className="h-3 w-3" />
  }
  if (section === 'plugins') {
    return <LuPuzzle className="h-3 w-3" />
  }
  if (section === 'controllers') {
    return <LuCpu className="h-3 w-3" />
  }
  if (section === 'directives') {
    return <LuRadar className="h-3 w-3" />
  }
  if (section === 'resources') {
    return <LuLayers className="h-3 w-3" />
  }
  return <LuBox className="h-3 w-3" />
}

function SidebarTree({
  sessions,
  spaces,
  plugins,
  controllers,
  directiveGroups,
  controllerCount,
  pluginCount,
  directiveCount,
  selected,
  onSelect,
  className,
}: {
  sessions: ReadonlyArray<{ sessionIndex?: number }>
  spaces: ReadonlyArray<{
    entry?: { ref?: { providerResourceRef?: { id?: string } } }
    spaceMeta?: { name?: string }
  }>
  plugins: ReadonlyArray<{ id?: string; instanceKey?: string; state?: string }>
  controllers: ReadonlyArray<{ id?: string }>
  directiveGroups: ReadonlyArray<{ name: string; count: number }>
  controllerCount: number
  pluginCount: number
  directiveCount: number
  selected: Selection
  onSelect: (sel: Selection) => void
  className?: string
}) {
  const ns = useStateNamespace(['system-status-dashboard'])
  const [openSections, setOpenSections] = useStateAtom(
    ns,
    'open-sections',
    DEFAULT_OPEN_SECTIONS,
  )
  const [showAllSpaces, setShowAllSpaces] = useStateAtom(
    ns,
    'show-all-spaces',
    false,
  )
  const [showAllControllers, setShowAllControllers] = useStateAtom(
    ns,
    'show-all-controllers',
    false,
  )
  const [showAllDirectives, setShowAllDirectives] = useStateAtom(
    ns,
    'show-all-directives',
    false,
  )
  const [focusedId, setFocusedId] = useState('section:accounts')
  const itemRefs = useRef<Record<string, HTMLButtonElement | null>>({})
  const visibleSpaces = showAllSpaces ? spaces : spaces.slice(0, 5)
  const visiblePlugins = plugins.slice(0, 5)
  const visibleControllers =
    showAllControllers ? controllers : controllers.slice(0, 5)
  const visibleDirectiveGroups =
    showAllDirectives ? directiveGroups : directiveGroups.slice(0, 5)
  const entries = useMemo<SidebarEntry[]>(() => {
    const nextEntries: SidebarEntry[] = [
      {
        id: 'section:accounts',
        kind: 'section',
        section: 'accounts',
        label: 'Accounts',
        count: sessions.length,
        expanded: openSections.accounts,
        level: 1,
      },
    ]

    if (openSections.accounts) {
      for (const session of sessions) {
        nextEntries.push({
          id: `session:${session.sessionIndex ?? 0}`,
          kind: 'session',
          sessionIndex: session.sessionIndex ?? 0,
          level: 2,
          parentSection: 'accounts',
        })
      }
    }

    nextEntries.push({
      id: 'section:spaces',
      kind: 'section',
      section: 'spaces',
      label: 'Spaces',
      count: spaces.length,
      expanded: openSections.spaces,
      level: 1,
    })

    if (openSections.spaces) {
      nextEntries.push({
        id: 'selection:spaces',
        kind: 'selection',
        label: 'All spaces',
        dot: 'bg-brand/40',
        level: 2,
        parentSection: 'spaces',
        selection: { kind: 'spaces' },
        selected: selected.kind === 'spaces',
      })
      for (const [index, space] of visibleSpaces.entries()) {
        const id = space.entry?.ref?.providerResourceRef?.id ?? ''
        const name = space.spaceMeta?.name ?? 'Untitled'
        nextEntries.push({
          id: `space:${makeOccurrenceKey(id || name, index)}`,
          kind: 'selection',
          label: name,
          dot: 'bg-brand',
          level: 2,
          parentSection: 'spaces',
          selection: { kind: 'space', id },
          selected: selected.kind === 'space' && selected.id === id,
        })
      }
      if (spaces.length > 5) {
        nextEntries.push({
          id: 'more:spaces',
          kind: 'more',
          section: 'spaces',
          label: showAllSpaces ? 'Show fewer' : `+${spaces.length - 5} more`,
          expanded: showAllSpaces,
          level: 2,
          parentSection: 'spaces',
        })
      }
    }

    nextEntries.push({
      id: 'section:plugins',
      kind: 'section',
      section: 'plugins',
      label: 'Plugins',
      count: pluginCount,
      expanded: openSections.plugins,
      level: 1,
    })

    if (openSections.plugins) {
      nextEntries.push({
        id: 'selection:plugins',
        kind: 'selection',
        label: 'All plugins',
        dot: 'bg-brand/40',
        level: 2,
        parentSection: 'plugins',
        selection: { kind: 'plugins' },
        selected: selected.kind === 'plugins',
      })
      for (const [index, plugin] of visiblePlugins.entries()) {
        const id = plugin.id ?? ''
        const instanceKey = plugin.instanceKey ?? ''
        nextEntries.push({
          id: `plugin:${makeOccurrenceKey(`${id}:${instanceKey}`, index)}`,
          kind: 'selection',
          label: id || 'unknown',
          sublabel: plugin.state || 'unknown',
          dot: plugin.state === 'requested' ? 'bg-success' : 'bg-warning/70',
          level: 2,
          parentSection: 'plugins',
          selection: { kind: 'plugin', id, instanceKey },
          selected:
            selected.kind === 'plugin' &&
            selected.id === id &&
            selected.instanceKey === instanceKey,
        })
      }
    }

    nextEntries.push({
      id: 'section:controllers',
      kind: 'section',
      section: 'controllers',
      label: 'Controllers',
      count: controllerCount,
      expanded: openSections.controllers,
      level: 1,
    })

    if (openSections.controllers) {
      nextEntries.push({
        id: 'selection:controllers',
        kind: 'selection',
        label: 'All controllers',
        dot: 'bg-success/50',
        level: 2,
        parentSection: 'controllers',
        selection: { kind: 'controllers' },
        selected: selected.kind === 'controllers',
      })
      for (const [index, controller] of visibleControllers.entries()) {
        nextEntries.push({
          id: `controller:${makeOccurrenceKey(controller.id, index)}`,
          kind: 'selection',
          label: controller.id || 'unknown',
          dot: 'bg-success',
          level: 2,
          parentSection: 'controllers',
          selection: { kind: 'controller', id: controller.id || '', index },
          selected:
            selected.kind === 'controller' &&
            selected.id === controller.id &&
            selected.index === index,
        })
      }
      if (controllers.length > 5) {
        nextEntries.push({
          id: 'more:controllers',
          kind: 'more',
          section: 'controllers',
          label:
            showAllControllers ? 'Show fewer' : (
              `+${controllers.length - 5} more`
            ),
          expanded: showAllControllers,
          level: 2,
          parentSection: 'controllers',
        })
      }
    }

    nextEntries.push({
      id: 'section:directives',
      kind: 'section',
      section: 'directives',
      label: 'Directives',
      count: directiveCount,
      expanded: openSections.directives,
      level: 1,
    })

    if (openSections.directives) {
      nextEntries.push({
        id: 'selection:directives',
        kind: 'selection',
        label: 'All directives',
        dot: 'bg-warning/50',
        level: 2,
        parentSection: 'directives',
        selection: { kind: 'directives' },
        selected: selected.kind === 'directives',
      })
      for (const [index, directiveGroup] of visibleDirectiveGroups.entries()) {
        nextEntries.push({
          id: `directive-group:${makeOccurrenceKey(directiveGroup.name, index)}`,
          kind: 'selection',
          label: directiveGroup.name,
          sublabel: String(directiveGroup.count),
          dot: 'bg-warning/70',
          level: 2,
          parentSection: 'directives',
          selection: {
            kind: 'directive-group',
            name: directiveGroup.name,
          },
          selected:
            selected.kind === 'directive-group' &&
            selected.name === directiveGroup.name,
        })
      }
      if (directiveGroups.length > 5) {
        nextEntries.push({
          id: 'more:directives',
          kind: 'more',
          section: 'directives',
          label:
            showAllDirectives ? 'Show fewer' : (
              `+${directiveGroups.length - 5} more`
            ),
          expanded: showAllDirectives,
          level: 2,
          parentSection: 'directives',
        })
      }
    }

    nextEntries.push({
      id: 'section:resources',
      kind: 'section',
      section: 'resources',
      label: 'Resources',
      expanded: openSections.resources,
      level: 1,
    })

    if (openSections.resources) {
      nextEntries.push({
        id: 'selection:resources',
        kind: 'selection',
        label: 'Resource tree',
        dot: 'bg-brand/40',
        level: 2,
        parentSection: 'resources',
        selection: { kind: 'resources' },
        selected: selected.kind === 'resources',
      })
    }

    nextEntries.push({
      id: 'section:atoms',
      kind: 'section',
      section: 'atoms',
      label: 'State Atoms',
      expanded: openSections.atoms,
      level: 1,
    })

    if (openSections.atoms) {
      nextEntries.push({
        id: 'selection:atoms',
        kind: 'selection',
        label: 'Atom tree',
        dot: 'bg-brand/40',
        level: 2,
        parentSection: 'atoms',
        selection: { kind: 'atoms' },
        selected: selected.kind === 'atoms',
      })
    }

    return nextEntries
  }, [
    controllerCount,
    controllers,
    directiveGroups,
    directiveCount,
    openSections,
    pluginCount,
    plugins,
    selected,
    sessions,
    showAllControllers,
    showAllDirectives,
    showAllSpaces,
    spaces,
    visibleControllers,
    visibleDirectiveGroups,
    visiblePlugins,
    visibleSpaces,
  ])
  const resolvedFocusedId =
    entries.some((entry) => entry.id === focusedId) ? focusedId : (
      (entries[0]?.id ?? '')
    )

  function setSectionExpanded(section: SidebarSectionKey, expanded: boolean) {
    setOpenSections((state) => ({ ...state, [section]: expanded }))
  }

  function focusEntry(index: number) {
    const entry = entries[index]
    if (!entry) return
    setFocusedId(entry.id)
    itemRefs.current[entry.id]?.focus()
  }

  function activateEntry(entry: SidebarEntry) {
    if (entry.kind === 'section') {
      setSectionExpanded(entry.section, !entry.expanded)
      return
    }
    if (entry.kind === 'session') {
      onSelect({ kind: 'session', index: entry.sessionIndex })
      return
    }
    if (entry.kind === 'selection') {
      onSelect(entry.selection)
      return
    }
    if (entry.section === 'spaces') {
      setShowAllSpaces(!showAllSpaces)
      return
    }
    if (entry.section === 'controllers') {
      setShowAllControllers(!showAllControllers)
      return
    }
    setShowAllDirectives(!showAllDirectives)
  }

  function handleEntryKeyDown(event: React.KeyboardEvent, entry: SidebarEntry) {
    const index = entries.findIndex((candidate) => candidate.id === entry.id)
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      focusEntry(Math.min(entries.length - 1, index + 1))
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      focusEntry(Math.max(0, index - 1))
      return
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault()
      if (entry.kind === 'section') {
        if (!entry.expanded) {
          setSectionExpanded(entry.section, true)
          return
        }
        focusEntry(index + 1)
        return
      }
      if (entry.kind === 'more') {
        activateEntry(entry)
      }
      return
    }
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      if (entry.kind === 'section') {
        if (!entry.expanded) return
        setSectionExpanded(entry.section, false)
        return
      }
      const parentIndex = entries.findIndex(
        (candidate) =>
          candidate.kind === 'section' &&
          candidate.section === entry.parentSection,
      )
      focusEntry(parentIndex)
      return
    }
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    activateEntry(entry)
  }

  return (
    <div
      role="tree"
      aria-label="System status navigation"
      className={cn(
        'border-foreground/6 w-48 shrink-0 overflow-auto border-r',
        className,
      )}
    >
      {entries.map((entry) => {
        if (entry.kind === 'section') {
          return (
            <button
              key={entry.id}
              ref={(node) => {
                itemRefs.current[entry.id] = node
              }}
              type="button"
              role="treeitem"
              aria-level={entry.level}
              aria-expanded={entry.expanded}
              tabIndex={resolvedFocusedId === entry.id ? 0 : -1}
              onFocus={() => setFocusedId(entry.id)}
              onKeyDown={(event) => handleEntryKeyDown(event, entry)}
              onClick={() => activateEntry(entry)}
              className="hover:bg-foreground/[0.02] focus-visible:ring-brand/30 flex w-full items-center gap-1.5 px-3 py-1.5 text-left transition-colors focus-visible:ring-1 focus-visible:outline-none"
            >
              <LuChevronRight
                className={cn(
                  'text-foreground-alt/25 h-3 w-3 transition-transform',
                  entry.expanded && 'rotate-90',
                )}
              />
              <span className="text-foreground-alt/40">
                {getSidebarSectionIcon(entry.section)}
              </span>
              <span className="text-foreground-alt/60 text-[0.6rem] font-medium tracking-wider uppercase">
                {entry.label}
              </span>
              {entry.count != null && (
                <span className="text-foreground-alt/25 ml-auto font-mono text-[0.55rem]">
                  {entry.count}
                </span>
              )}
            </button>
          )
        }

        if (entry.kind === 'session') {
          return (
            <SessionSidebarItem
              key={entry.id}
              buttonRef={(node) => {
                itemRefs.current[entry.id] = node
              }}
              sessionIndex={entry.sessionIndex}
              selected={
                selected.kind === 'session' &&
                selected.index === entry.sessionIndex
              }
              focused={resolvedFocusedId === entry.id}
              onFocus={() => setFocusedId(entry.id)}
              onClick={() => activateEntry(entry)}
              onKeyDown={(event) => handleEntryKeyDown(event, entry)}
            />
          )
        }

        if (entry.kind === 'more') {
          return (
            <button
              key={entry.id}
              ref={(node) => {
                itemRefs.current[entry.id] = node
              }}
              type="button"
              role="treeitem"
              aria-level={entry.level}
              tabIndex={resolvedFocusedId === entry.id ? 0 : -1}
              onFocus={() => setFocusedId(entry.id)}
              onKeyDown={(event) => handleEntryKeyDown(event, entry)}
              onClick={() => activateEntry(entry)}
              className="text-foreground-alt/20 hover:text-foreground-alt/40 focus-visible:ring-brand/30 w-full py-0.5 pr-3 pl-7 text-left text-[0.55rem] transition-colors focus-visible:ring-1 focus-visible:outline-none"
            >
              {entry.label}
            </button>
          )
        }

        return (
          <button
            key={entry.id}
            ref={(node) => {
              itemRefs.current[entry.id] = node
            }}
            type="button"
            role="treeitem"
            aria-level={entry.level}
            aria-selected={entry.selected}
            tabIndex={resolvedFocusedId === entry.id ? 0 : -1}
            onFocus={() => setFocusedId(entry.id)}
            onKeyDown={(event) => handleEntryKeyDown(event, entry)}
            onClick={() => activateEntry(entry)}
            className={cn(
              'focus-visible:ring-brand/30 flex w-full items-center gap-1.5 py-0.5 pr-3 pl-7 text-left transition-colors focus-visible:ring-1 focus-visible:outline-none',
              entry.selected ?
                'bg-brand/[0.08] text-foreground'
              : 'text-foreground/60 hover:bg-foreground/[0.02] hover:text-foreground/80',
            )}
          >
            <span
              className={cn('h-1.5 w-1.5 shrink-0 rounded-full', entry.dot)}
            />
            <span className="min-w-0 truncate text-[0.6rem]">
              {entry.label}
            </span>
            {entry.sublabel && (
              <span className="text-foreground-alt/25 ml-auto shrink-0 font-mono text-[0.5rem]">
                {entry.sublabel}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

function SessionSidebarItem({
  sessionIndex,
  selected,
  focused,
  onFocus,
  onClick,
  onKeyDown,
  buttonRef,
}: {
  sessionIndex: number
  selected: boolean
  focused: boolean
  onFocus: () => void
  onClick: () => void
  onKeyDown: (event: React.KeyboardEvent) => void
  buttonRef: (node: HTMLButtonElement | null) => void
}) {
  const metadata = useSessionMetadata(sessionIndex)
  const label = metadata?.displayName || `Session ${sessionIndex}`
  return (
    <button
      ref={buttonRef}
      type="button"
      role="treeitem"
      aria-level={2}
      aria-selected={selected}
      tabIndex={focused ? 0 : -1}
      onFocus={onFocus}
      onKeyDown={onKeyDown}
      onClick={onClick}
      className={cn(
        'focus-visible:ring-brand/30 flex w-full items-center gap-1.5 py-0.5 pr-3 pl-7 text-left transition-colors focus-visible:ring-1 focus-visible:outline-none',
        selected ?
          'bg-brand/[0.08] text-foreground'
        : 'text-foreground/60 hover:bg-foreground/[0.02] hover:text-foreground/80',
      )}
    >
      <span className="bg-success h-1.5 w-1.5 shrink-0 rounded-full" />
      <span className="min-w-0 truncate text-[0.6rem]">{label}</span>
      <span className="text-foreground-alt/25 ml-auto shrink-0 font-mono text-[0.5rem]">
        /u/{sessionIndex}
      </span>
    </button>
  )
}

// ---------------------------------------------------------------------------
// Detail view
// ---------------------------------------------------------------------------

function DetailView({
  selected,
  spaces,
  plugins,
  controllers,
  directiveGroups,
  buildInfo,
  pluginCount,
  controllerCount,
  directives,
  directiveCount,
  onSelect,
  onClose,
  spacesUpdatedAt,
  pluginsUpdatedAt,
  controllersUpdatedAt,
  directivesUpdatedAt,
}: {
  selected: Selection
  spaces: ReadonlyArray<{
    entry?: {
      ref?: {
        providerResourceRef?: {
          id?: string
        }
      }
      source?: string
    }
    spaceMeta?: { name?: string }
  }>
  plugins: ReadonlyArray<{ id?: string; instanceKey?: string; state?: string }>
  controllers: ReadonlyArray<{
    id?: string
    version?: string
    description?: string
  }>
  directiveGroups: ReadonlyArray<{
    name: string
    idents: string[]
    count: number
  }>
  buildInfo: AppBuildInfo
  pluginCount: number
  controllerCount: number
  directives: ReadonlyArray<{ name?: string; ident?: string }>
  directiveCount: number
  onSelect: (sel: Selection) => void
  onClose?: () => void
  spacesUpdatedAt: string
  pluginsUpdatedAt: string
  controllersUpdatedAt: string
  directivesUpdatedAt: string
}) {
  if (selected.kind === 'session') {
    return <SessionDetail sessionIndex={selected.index} onClose={onClose} />
  }

  if (selected.kind === 'spaces') {
    return (
      <SpacesDetail
        spaces={spaces}
        updatedAt={spacesUpdatedAt}
        onSelectSpace={(id) => onSelect({ kind: 'space', id })}
      />
    )
  }

  if (selected.kind === 'space') {
    return (
      <SpaceDetail
        space={spaces.find(
          (space) => space.entry?.ref?.providerResourceRef?.id === selected.id,
        )}
        updatedAt={spacesUpdatedAt}
        onClose={onClose}
      />
    )
  }

  if (selected.kind === 'plugins') {
    return (
      <PluginsDetail
        buildInfo={buildInfo}
        plugins={plugins}
        pluginCount={pluginCount}
        updatedAt={pluginsUpdatedAt}
        onSelectPlugin={(id, instanceKey) =>
          onSelect({ kind: 'plugin', id, instanceKey })
        }
      />
    )
  }

  if (selected.kind === 'plugin') {
    return (
      <PluginDetail
        plugin={plugins.find(
          (plugin) =>
            plugin.id === selected.id &&
            (plugin.instanceKey ?? '') === selected.instanceKey,
        )}
        buildInfo={buildInfo}
        updatedAt={pluginsUpdatedAt}
        onBack={() => onSelect({ kind: 'plugins' })}
      />
    )
  }

  if (selected.kind === 'controllers') {
    return (
      <ControllersDetail
        controllers={controllers}
        controllerCount={controllerCount}
        updatedAt={controllersUpdatedAt}
        onSelectController={(id, index) =>
          onSelect({ kind: 'controller', id, index })
        }
      />
    )
  }

  if (selected.kind === 'controller') {
    const c = controllers[selected.index]
    if (!c) return null
    return (
      <ControllerDetail
        controller={c}
        index={selected.index}
        updatedAt={controllersUpdatedAt}
        onBack={() => onSelect({ kind: 'controllers' })}
      />
    )
  }

  if (selected.kind === 'directives') {
    return (
      <DirectivesDetail
        directives={directives}
        directiveCount={directiveCount}
        updatedAt={directivesUpdatedAt}
      />
    )
  }

  if (selected.kind === 'directive-group') {
    return (
      <DirectiveGroupDetail
        directiveGroup={directiveGroups.find(
          (group) => group.name === selected.name,
        )}
        updatedAt={directivesUpdatedAt}
      />
    )
  }

  if (selected.kind === 'resources') {
    return <ResourcesDetail />
  }

  // atoms
  return <AtomsDetail />
}

// SessionDetail fetches metadata and renders session info.
function SessionDetail({
  sessionIndex,
  onClose,
}: {
  sessionIndex: number
  onClose?: () => void
}) {
  const metadata = useSessionMetadata(sessionIndex)
  const currentSessionIndex = useSessionIndex()
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const name = metadata?.displayName || `Session ${sessionIndex}`
  const initials = name
    .split(' ')
    .map((w) => w[0])
    .join('')
    .slice(0, 2)

  return (
    <div className="space-y-2 p-4">
      <div className="flex items-center gap-2">
        <div className="bg-brand/10 flex h-6 w-6 items-center justify-center rounded-full">
          <span className="text-brand text-[0.5rem] font-bold">{initials}</span>
        </div>
        <div>
          <span className="text-foreground text-sm font-medium">{name}</span>
          <span className="text-foreground-alt/30 ml-2 font-mono text-[0.6rem]">
            /u/{sessionIndex}
          </span>
        </div>
      </div>
      <DetailCard title="Account" accent="border-success/40">
        <div className="py-0.5">
          <DetailRow label="Display Name" value={name} mono={false} />
          <DetailRow label="Session Index" value={String(sessionIndex)} />
          <DetailRow label="Session Path" value={`/u/${sessionIndex}`} />
          {metadata?.providerDisplayName && (
            <DetailRow
              label="Provider"
              value={metadata.providerDisplayName}
              mono={false}
            />
          )}
          {metadata?.cloudEntityId && (
            <DetailRow label="Entity" value={metadata.cloudEntityId} />
          )}
          {metadata?.providerAccountId && (
            <DetailRow label="Account ID" value={metadata.providerAccountId} />
          )}
          {metadata?.cloudAccountId && (
            <DetailRow label="Cloud Account" value={metadata.cloudAccountId} />
          )}
          {metadata?.providerId && (
            <DetailRow label="Provider ID" value={metadata.providerId} />
          )}
          {metadata?.createdAt != null && metadata.createdAt !== 0n && (
            <DetailRow
              label="Created"
              value={formatTimestamp(Number(metadata.createdAt))}
            />
          )}
        </div>
      </DetailCard>
      {metadata?.lockMode != null && (
        <DetailCard title="Security" accent="border-warning/40">
          <div className="flex items-center gap-2 px-3 py-1.5">
            {metadata.lockMode === SessionLockMode.AUTO_UNLOCK ?
              <LuLockOpen className="text-success/60 h-3 w-3" />
            : <LuLock className="text-warning/60 h-3 w-3" />}
            <span className="text-foreground/70 text-xs">
              {metadata.lockMode === SessionLockMode.AUTO_UNLOCK ?
                'Auto-unlock (no PIN)'
              : 'PIN encrypted'}
            </span>
          </div>
        </DetailCard>
      )}
      <DetailCard title="Actions" accent="border-brand/40">
        <div className="flex flex-col gap-2 p-3">
          <button
            type="button"
            onClick={() => {
              navigate({ path: `/u/${sessionIndex}` })
              onClose?.()
            }}
            className="border-foreground/8 text-foreground hover:bg-foreground/[0.03] rounded-md border px-3 py-1.5 text-left text-xs transition-colors"
          >
            Open Session
          </button>
          <button
            type="button"
            onClick={() => {
              if (currentSessionIndex !== sessionIndex) {
                navigate({ path: `/u/${sessionIndex}` })
              }
              queueMicrotask(() => setOpenMenu?.('account'))
            }}
            className="border-foreground/8 text-foreground hover:bg-foreground/[0.03] rounded-md border px-3 py-1.5 text-left text-xs transition-colors"
          >
            Open Session Details
          </button>
        </div>
      </DetailCard>
    </div>
  )
}

function SpacesDetail({
  spaces,
  updatedAt,
  onSelectSpace,
}: {
  spaces: ReadonlyArray<{
    entry?: {
      ref?: {
        providerResourceRef?: {
          id?: string
        }
      }
    }
    spaceMeta?: { name?: string }
  }>
  updatedAt: string
  onSelectSpace: (id: string) => void
}) {
  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <LuFolderOpen className="text-brand/50 h-4 w-4" />
        <span className="text-foreground text-sm font-medium">Spaces</span>
        <span className="text-foreground-alt/30 font-mono text-xs">
          {spaces.length}
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Spaces" />
      </div>
      <div className="border-foreground/6 min-h-0 flex-1 overflow-auto rounded-md border">
        {spaces.length === 0 && (
          <div className="px-3 py-2">
            <span className="text-foreground-alt/30 text-[0.6rem]">
              No spaces mounted.
            </span>
          </div>
        )}
        {spaces.map((space, i) => {
          const id = space.entry?.ref?.providerResourceRef?.id ?? ''
          const name = space.spaceMeta?.name ?? 'Untitled'
          return (
            <button
              type="button"
              key={makeOccurrenceKey(id || name, i)}
              onClick={() => {
                if (!id) return
                onSelectSpace(id)
              }}
              className="border-foreground/4 hover:bg-foreground/[0.02] flex w-full items-center gap-2 border-b px-3 py-1.5 text-left last:border-b-0"
            >
              <span className="bg-brand h-1.5 w-1.5 shrink-0 rounded-full" />
              <div className="min-w-0 flex-1">
                <span className="text-foreground/80 block truncate text-[0.65rem]">
                  {name}
                </span>
                <span className="text-foreground-alt/25 block truncate font-mono text-[0.55rem]">
                  {id || 'unknown'}
                </span>
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function SpaceDetail({
  space,
  updatedAt,
  onClose,
}: {
  space?:
    | {
        entry?: {
          ref?: {
            providerResourceRef?: {
              id?: string
            }
          }
          source?: string
        }
        spaceMeta?: { name?: string }
      }
    | undefined
  updatedAt: string
  onClose?: () => void
}) {
  const navigate = useNavigate()
  const sessionIndex = useSessionIndex()

  if (!space) {
    return (
      <div className="space-y-2 p-4">
        <DetailCard title="Space" accent="border-brand/40">
          <div className="py-0.5">
            <DetailRow label="Status" value="Not found" mono={false} />
          </div>
        </DetailCard>
      </div>
    )
  }

  const id = space.entry?.ref?.providerResourceRef?.id ?? ''
  const name = space.spaceMeta?.name ?? 'Untitled'
  const source = space.entry?.source ?? 'unknown'

  return (
    <div className="space-y-2 p-4">
      <div className="flex items-center gap-2">
        <span className="bg-brand h-2 w-2 rounded-full" />
        <span className="text-foreground text-sm font-medium">{name}</span>
        <LiveIndicator updatedAt={updatedAt} label="Space" />
      </div>
      <DetailCard title="Space" accent="border-brand/40">
        <div className="py-0.5">
          <DetailRow label="Name" value={name} mono={false} />
          <DetailRow label="Space ID" value={id || 'unknown'} />
          <DetailRow label="Source" value={source} mono={false} />
        </div>
      </DetailCard>
      <div>
        <button
          type="button"
          onClick={() => {
            if (!id || !sessionIndex) return
            navigate({ path: `/u/${sessionIndex}/so/${id}` })
            onClose?.()
          }}
          className="border-foreground/8 text-foreground hover:bg-foreground/[0.03] rounded-md border px-3 py-1.5 text-xs transition-colors"
        >
          Open Space
        </button>
      </div>
    </div>
  )
}

function PluginsDetail({
  buildInfo,
  plugins,
  pluginCount,
  updatedAt,
  onSelectPlugin,
}: {
  buildInfo: AppBuildInfo
  plugins: ReadonlyArray<{ id?: string; instanceKey?: string; state?: string }>
  pluginCount: number
  updatedAt: string
  onSelectPlugin: (id: string, instanceKey: string) => void
}) {
  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <LuPuzzle className="text-brand/50 h-4 w-4" />
        <span className="text-foreground text-sm font-medium">Plugins</span>
        <span className="text-foreground-alt/30 font-mono text-xs">
          {pluginCount}
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Plugins" />
      </div>
      <div className="grid gap-2 pb-2 md:grid-cols-2">
        <BuildInfoCard buildInfo={buildInfo} />
        <DetailCard title="Runtime" accent="border-success/40">
          <div className="py-0.5">
            <DetailRow label="Plugin Requests" value={String(pluginCount)} />
            <DetailRow
              label="State Source"
              value="LoadPlugin directives"
              mono={false}
            />
          </div>
        </DetailCard>
      </div>
      <div className="border-foreground/6 min-h-0 flex-1 overflow-auto rounded-md border">
        {plugins.length === 0 && (
          <div className="px-3 py-2">
            <span className="text-foreground-alt/30 text-[0.6rem]">
              No plugin load requests active.
            </span>
          </div>
        )}
        {plugins.map((plugin, index) => {
          const id = plugin.id ?? ''
          const instanceKey = plugin.instanceKey ?? ''
          const state = plugin.state || 'unknown'
          return (
            <button
              type="button"
              key={makeOccurrenceKey(`${id}:${instanceKey}`, index)}
              onClick={() => onSelectPlugin(id, instanceKey)}
              className="border-foreground/4 hover:bg-foreground/[0.02] flex w-full items-center gap-2 border-b px-3 py-1.5 text-left last:border-b-0"
            >
              <span
                className={cn(
                  'h-1.5 w-1.5 shrink-0 rounded-full',
                  state === 'requested' ? 'bg-success' : 'bg-warning/70',
                )}
              />
              <div className="min-w-0 flex-1">
                <span className="text-foreground/80 block truncate text-[0.65rem]">
                  {id || 'unknown'}
                </span>
                <span className="text-foreground-alt/25 block truncate font-mono text-[0.55rem]">
                  {instanceKey || 'shared'}
                </span>
              </div>
              <span className="bg-foreground/5 text-foreground-alt/45 rounded px-1.5 py-0.5 font-mono text-[0.5rem]">
                {state}
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function PluginDetail({
  plugin,
  buildInfo,
  updatedAt,
  onBack,
}: {
  plugin?: { id?: string; instanceKey?: string; state?: string }
  buildInfo: AppBuildInfo
  updatedAt: string
  onBack: () => void
}) {
  if (!plugin) {
    return (
      <div className="space-y-2 p-4">
        <button
          type="button"
          onClick={onBack}
          className="text-foreground-alt/50 hover:text-foreground-alt flex items-center gap-1 text-xs transition-colors"
        >
          <LuArrowLeft className="h-3 w-3" />
          Back to plugins
        </button>
        <DetailCard title="Plugin" accent="border-brand/40">
          <div className="py-0.5">
            <DetailRow label="Status" value="Not found" mono={false} />
          </div>
        </DetailCard>
      </div>
    )
  }

  return (
    <div className="space-y-2 p-4">
      <button
        type="button"
        onClick={onBack}
        className="text-foreground-alt/50 hover:text-foreground-alt flex items-center gap-1 text-xs transition-colors"
      >
        <LuArrowLeft className="h-3 w-3" />
        Back to plugins
      </button>
      <div className="flex items-center gap-2">
        <span
          className={cn(
            'h-2 w-2 rounded-full',
            plugin.state === 'requested' ? 'bg-success' : 'bg-warning/70',
          )}
        />
        <span className="text-foreground text-sm font-medium">
          {plugin.id || 'unknown'}
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Plugin" />
      </div>
      <DetailCard title="Plugin" accent="border-brand/40">
        <div className="py-0.5">
          <DetailRow label="Plugin ID" value={plugin.id || 'unknown'} />
          <DetailRow
            label="Instance"
            value={plugin.instanceKey || 'shared'}
            mono={false}
          />
          <DetailRow
            label="State"
            value={plugin.state || 'unknown'}
            mono={false}
          />
        </div>
      </DetailCard>
      <BuildInfoCard buildInfo={buildInfo} />
    </div>
  )
}

function BuildInfoCard({ buildInfo }: { buildInfo: AppBuildInfo }) {
  return (
    <DetailCard title="Build" accent="border-brand/40">
      <div className="py-0.5">
        <DetailRow label="Version" value={buildInfo.version || 'dev'} />
        <DetailRow
          label="Main Version"
          value={buildInfo.mainVersion || 'n/a'}
        />
        <DetailRow
          label="Runtime"
          value={buildInfo.runtimeLabel || 'unknown'}
          mono={false}
        />
        <DetailRow
          label="Platform"
          value={
            buildInfo.goos && buildInfo.goarch ?
              `${buildInfo.goos}/${buildInfo.goarch}`
            : 'unknown'
          }
        />
        <DetailRow
          label="Browser Gen"
          value={buildInfo.browserGenerationId || 'n/a'}
        />
      </div>
    </DetailCard>
  )
}

function ControllersDetail({
  controllers,
  controllerCount,
  updatedAt,
  onSelectController,
}: {
  controllers: ReadonlyArray<{
    id?: string
    version?: string
    description?: string
  }>
  controllerCount: number
  updatedAt: string
  onSelectController: (id: string, index: number) => void
}) {
  const freshControllers = useFreshKeys(
    useMemo(
      () => controllers.map((controller) => controller.id || ''),
      [controllers],
    ),
  )

  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <LuCpu className="text-success/60 h-4 w-4" />
        <span className="text-foreground text-sm font-medium">Controllers</span>
        <span className="text-foreground-alt/30 font-mono text-xs">
          {controllerCount}
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Controllers" />
      </div>
      <div className="border-foreground/6 min-h-0 flex-1 overflow-auto rounded-md border">
        {controllers.map((controller, index) => (
          <button
            type="button"
            key={makeOccurrenceKey(controller.id, index)}
            onClick={() => onSelectController(controller.id || '', index)}
            className={cn(
              'border-foreground/4 hover:bg-foreground/[0.02] flex w-full items-center gap-2 border-b px-3 py-1.5 text-left transition-colors last:border-b-0',
              freshControllers.has(controller.id || '') &&
                'bg-success/5 ring-success/15 ring-1 ring-inset',
            )}
          >
            <span className="bg-success h-1.5 w-1.5 shrink-0 rounded-full" />
            <div className="min-w-0 flex-1">
              <span className="text-foreground/80 block truncate font-mono text-[0.65rem]">
                {controller.id || 'unknown'}
              </span>
              {controller.description && (
                <span className="text-foreground-alt/30 block truncate text-[0.55rem]">
                  {controller.description}
                </span>
              )}
            </div>
            {controller.version && (
              <span className="text-foreground-alt/20 shrink-0 font-mono text-[0.55rem]">
                v{controller.version}
              </span>
            )}
          </button>
        ))}
        {controllerCount > controllers.length && (
          <div className="border-foreground/4 border-t px-3 py-1.5">
            <span className="text-foreground-alt/20 text-[0.55rem]">
              {controllerCount - controllers.length} more not shown
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

function ControllerDetail({
  controller,
  index,
  updatedAt,
  onBack,
}: {
  controller: {
    id?: string
    version?: string
    description?: string
  }
  index: number
  updatedAt: string
  onBack: () => void
}) {
  return (
    <div className="space-y-2 p-4">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          aria-label="Back to controllers"
          className="text-foreground-alt/60 hover:text-foreground -ml-1 rounded p-1 transition-colors"
        >
          <LuArrowLeft className="h-3.5 w-3.5" />
        </button>
        <span className="bg-success h-2 w-2 rounded-full" />
        <span className="text-foreground text-sm font-medium">
          {controller.id}
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Controller" />
      </div>
      <DetailCard title="Controller" accent="border-success/40">
        <div className="py-0.5">
          <DetailRow label="ID" value={controller.id || ''} />
          <DetailRow label="List Index" value={String(index + 1)} />
          <DetailRow label="Version" value={controller.version || ''} />
          <DetailRow
            label="Description"
            value={controller.description || ''}
            mono={false}
          />
        </div>
      </DetailCard>
    </div>
  )
}

// DirectivesDetail groups and displays directives.
function DirectivesDetail({
  directives,
  directiveCount,
  updatedAt,
}: {
  directives: ReadonlyArray<{ name?: string; ident?: string }>
  directiveCount: number
  updatedAt: string
}) {
  const grouped = useMemo(() => groupDirectives(directives), [directives])
  const maxDir = Math.max(...grouped.map((d) => d.count), 1)
  const directivesNs = useStateNamespace([
    'system-status-dashboard',
    'directives',
  ])
  const [expandedGroups, setExpandedGroups] = useStateAtom<
    Record<string, boolean>
  >(directivesNs, 'expanded-groups', {})
  const freshGroups = useFreshKeys(
    useMemo(() => grouped.map((group) => group.name), [grouped]),
  )

  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <LuRadar className="text-warning/60 h-4 w-4" />
        <span className="text-foreground text-sm font-medium">Directives</span>
        <span className="text-foreground-alt/30 font-mono text-xs">
          {directiveCount}
        </span>
        <span className="text-foreground-alt/20 text-[0.55rem]">
          {grouped.length} types
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Directives" />
      </div>
      <div className="border-foreground/6 min-h-0 flex-1 overflow-auto rounded-md border">
        {grouped.map((d, i) => (
          <DirectiveRow
            key={makeOccurrenceKey(d.name, i)}
            directive={d}
            expanded={!!expandedGroups[d.name]}
            onToggle={() => {
              setExpandedGroups((state) => {
                if (!state[d.name]) {
                  return { ...state, [d.name]: true }
                }
                const next = { ...state }
                delete next[d.name]
                return next
              })
            }}
            fresh={freshGroups.has(d.name)}
            maxCount={maxDir}
          />
        ))}
      </div>
    </div>
  )
}

// Expandable directive row showing idents when expanded.
function DirectiveRow({
  directive,
  expanded,
  onToggle,
  fresh,
  maxCount,
}: {
  directive: { name: string; idents: string[]; count: number }
  expanded: boolean
  onToggle: () => void
  fresh: boolean
  maxCount: number
}) {
  return (
    <div>
      <div
        onClick={onToggle}
        className={cn(
          'hover:bg-foreground/[0.02] flex cursor-pointer items-center gap-2 px-3 py-1 transition-colors',
          fresh && 'bg-warning/6 ring-warning/20 ring-1 ring-inset',
        )}
      >
        <LuChevronRight
          className={cn(
            'text-foreground-alt/20 h-2.5 w-2.5 transition-transform',
            expanded && 'rotate-90',
          )}
        />
        <span className="text-foreground/80 min-w-0 flex-1 truncate font-mono text-[0.65rem]">
          {directive.name}
        </span>
        <div className="bg-foreground/5 h-1.5 w-20 shrink-0 overflow-hidden rounded-full">
          <div
            className="bg-warning/30 h-full rounded-full"
            style={{
              width: `${(directive.count / maxCount) * 100}%`,
            }}
          />
        </div>
        <span className="text-foreground-alt/40 w-8 shrink-0 text-right font-mono text-[0.6rem] tabular-nums">
          {directive.count}
        </span>
      </div>
      {expanded && (
        <div className="bg-foreground/[0.01] border-foreground/4 border-t">
          {directive.idents.map((ident, i) => (
            <div key={i} className="flex items-center gap-1.5 py-0.5 pr-3 pl-9">
              <span className="bg-warning/20 h-1 w-1 shrink-0 rounded-full" />
              <span className="text-foreground-alt/50 truncate font-mono text-[0.55rem]">
                {ident}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function DirectiveGroupDetail({
  directiveGroup,
  updatedAt,
}: {
  directiveGroup?:
    | {
        name: string
        idents: string[]
        count: number
      }
    | undefined
  updatedAt: string
}) {
  if (!directiveGroup) {
    return (
      <div className="space-y-2 p-4">
        <DetailCard title="Directive" accent="border-warning/40">
          <div className="py-0.5">
            <DetailRow label="Status" value="Not found" mono={false} />
          </div>
        </DetailCard>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <span className="bg-warning/70 h-2 w-2 rounded-full" />
        <span className="text-foreground text-sm font-medium">
          {directiveGroup.name}
        </span>
        <span className="text-foreground-alt/30 font-mono text-xs">
          {directiveGroup.count}
        </span>
        <LiveIndicator updatedAt={updatedAt} label="Directive" />
      </div>
      <DetailCard title="Directive Type" accent="border-warning/40">
        <div className="py-0.5">
          <DetailRow label="Name" value={directiveGroup.name} />
          <DetailRow
            label="Active Count"
            value={String(directiveGroup.count)}
          />
        </div>
      </DetailCard>
      <DetailCard title="Instances" accent="border-warning/40">
        <div className="max-h-80 overflow-auto py-1">
          {directiveGroup.idents.map((ident, index) => (
            <div
              key={makeOccurrenceKey(ident, index)}
              className="border-foreground/4 flex items-center gap-2 border-b px-3 py-1 last:border-b-0"
            >
              <span className="bg-warning/20 h-1.5 w-1.5 shrink-0 rounded-full" />
              <span className="text-foreground-alt/60 truncate font-mono text-[0.6rem]">
                {ident}
              </span>
            </div>
          ))}
        </div>
      </DetailCard>
    </div>
  )
}

// ResourcesDetail shows the resource tree with a details side panel.
function ResourcesDetail() {
  const selectedId = useSelectedResourceId()
  const resources = useTrackedResources()
  const selectedResource = selectedId ? resources.get(selectedId) : undefined

  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <LuLayers className="text-brand/40 h-4 w-4" />
        <span className="text-foreground text-sm font-medium">Resources</span>
      </div>
      <div className="flex min-h-0 flex-1 overflow-hidden">
        <div className="min-w-0 flex-1 overflow-auto">
          <ResourceTreeTab />
        </div>
        {selectedResource && (
          <ResourceDetailsPanel resource={selectedResource} />
        )}
      </div>
    </div>
  )
}

// AtomsDetail shows the state atom tree.
function AtomsDetail() {
  const selectedAtomId = useSelectedStateAtomId()
  const entryMap = useStateInspectorEntryMap()
  const selectedEntry =
    selectedAtomId ? entryMap.get(selectedAtomId) : undefined

  return (
    <div className="flex h-full flex-col p-4">
      <div className="mb-2 flex items-center gap-2">
        <LuBox className="text-brand/40 h-4 w-4" />
        <span className="text-foreground text-sm font-medium">State Atoms</span>
      </div>
      <div className="flex min-h-0 flex-1 overflow-hidden">
        <div className="min-w-0 flex-1 overflow-auto">
          <StateTreeTab />
        </div>
        {selectedEntry && <StateDetailsPanel entry={selectedEntry} />}
      </div>
    </div>
  )
}
