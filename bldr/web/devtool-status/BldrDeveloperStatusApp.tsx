import { useState, type ReactNode } from 'react'
import {
  LuActivity,
  LuCheck,
  LuCopy,
  LuExternalLink,
  LuLayers,
  LuPlug,
  LuTerminal,
  LuTriangleAlert,
} from 'react-icons/lu'
import { useWebViewHostServiceClient } from '@aptre/bldr-react'

import {
  DevtoolStatusAttentionSeverity,
  DevtoolStatusCommandState,
  DevtoolStatusControllerState,
  DevtoolStatusManifestState,
  DevtoolStatusPluginState,
  type DevtoolStatusAttentionRow,
  type DevtoolStatusBuildTarget,
  type DevtoolStatusCommand,
  type DevtoolStatusControllerRow,
  type DevtoolStatusManifestBuildRow,
  type DevtoolStatusManifestFetchRow,
  type DevtoolStatusPluginRow,
  type DevtoolStatusSnapshot,
} from '../../devtool/status/status.pb.js'
import {
  DevtoolStatusServiceClient,
  DevtoolStatusServiceServiceName,
} from '../../devtool/status/status_srpc.pb.js'

export type DevtoolStatusConnectionState =
  | 'connecting'
  | 'live'
  | 'disconnected'
  | 'closed'

export interface DevtoolStatusViewState {
  connectionState: DevtoolStatusConnectionState
  snapshot: DevtoolStatusSnapshot | null
  error?: string
}

const devtoolStatusServiceID = `devtool/${DevtoolStatusServiceServiceName}`
const emptyReq = {}
const statusPillStateClass: Record<DevtoolStatusConnectionState, string> = {
  closed: 'bldr-dev-status__pill--closed',
  connecting: 'bldr-dev-status__pill--connecting',
  disconnected: 'bldr-dev-status__pill--disconnected',
  live: 'bldr-dev-status__pill--live',
}

export function BldrDeveloperStatusApp() {
  const status = useDevtoolStatus()
  return <BldrDeveloperStatusSurface status={status} />
}

export function useDevtoolStatus(): DevtoolStatusViewState {
  const [status, setStatus] = useState<DevtoolStatusViewState>({
    connectionState: 'connecting',
    snapshot: null,
  })

  useWebViewHostServiceClient<DevtoolStatusServiceClient>(
    (client) =>
      new DevtoolStatusServiceClient(client, {
        service: devtoolStatusServiceID,
      }),
    (service, abortSignal) => {
      const lastSnapshotRef: { current: DevtoolStatusSnapshot | null } = {
        current: null,
      }
      void (async () => {
        try {
          for await (const resp of service.WatchDevtoolStatus(
            emptyReq,
            abortSignal,
          )) {
            const snapshot = resp.snapshot ?? null
            lastSnapshotRef.current = snapshot
            setStatus({
              connectionState: isClosedSnapshot(snapshot) ? 'closed' : 'live',
              snapshot,
            })
          }
          if (abortSignal.aborted) {
            return
          }
          setStatus((prev) => ({
            connectionState:
              isClosedSnapshot(lastSnapshotRef.current) ? 'closed' : (
                'disconnected'
              ),
            snapshot: lastSnapshotRef.current ?? prev.snapshot,
          }))
        } catch (err) {
          if (abortSignal.aborted) {
            return
          }
          setStatus((prev) => ({
            connectionState: 'disconnected',
            snapshot: prev.snapshot,
            error: err instanceof Error ? err.message : String(err),
          }))
        }
      })()
    },
  )

  return status
}

export function BldrDeveloperStatusSurface({
  status,
}: {
  status: DevtoolStatusViewState
}) {
  const snapshot = status.snapshot
  const command = snapshot?.command
  const project = snapshot?.project
  const buildRows = snapshot?.manifestBuildRows ?? []
  const fetchRows = snapshot?.manifestFetchRows ?? []
  const pluginRows = snapshot?.pluginRows ?? []
  const controllerRows = snapshot?.controllerRows ?? []
  const attentionRows = snapshot?.attentionRows ?? []
  const pluginErrors = pluginRows.filter((row) => row.error)
  const issueCount =
    attentionRows.filter(
      (row) =>
        row.severity ===
        DevtoolStatusAttentionSeverity.DevtoolStatusAttentionSeverity_ERROR,
    ).length + pluginErrors.length

  return (
    <main className="bldr-dev-status">
      <header className="bldr-dev-status__header">
        <div>
          <div className="bldr-dev-status__eyebrow">Bldr Devtool</div>
          <h1>Bldr Status</h1>
        </div>
        <div className="bldr-dev-status__header-meta">
          <StatusPill state={status.connectionState} />
          {project?.projectId && (
            <span className="bldr-dev-status__project">
              {project.projectId}
            </span>
          )}
        </div>
      </header>

      {status.error && (
        <section className="bldr-dev-status__notice" aria-label="Stream error">
          <LuTriangleAlert />
          <span>{status.error}</span>
        </section>
      )}

      <section className="bldr-dev-status__summary" aria-label="Status summary">
        <SummaryTile
          icon={<LuTerminal />}
          label="Command"
          value={command?.name || 'Waiting'}
          detail={commandSummary(command)}
        />
        <SummaryTile
          icon={<LuLayers />}
          label="Targets"
          value={String(project?.buildTargets?.length ?? 0)}
          detail={`${project?.manifestIds?.length ?? 0} manifests`}
        />
        <SummaryTile
          icon={<LuActivity />}
          label="Builds"
          value={String(buildRows.length)}
          detail={manifestStateSummary(buildRows)}
        />
        <SummaryTile
          icon={<LuPlug />}
          label="Plugins"
          value={String(pluginRows.length)}
          detail={pluginStateSummary(pluginRows)}
        />
      </section>

      <section className="bldr-dev-status__grid">
        <Panel title="Command" icon={<LuTerminal />}>
          <CommandPanel command={command} />
        </Panel>
        <Panel title="Project Targets" icon={<LuLayers />}>
          <TargetsPanel targets={project?.buildTargets ?? []} />
        </Panel>
        <Panel title="Manifest Activity" icon={<LuActivity />}>
          <ManifestPanel buildRows={buildRows} fetchRows={fetchRows} />
        </Panel>
        <Panel title="Runtime" icon={<LuPlug />}>
          <RuntimePanel plugins={pluginRows} controllers={controllerRows} />
        </Panel>
        <Panel title="Diagnostics" icon={<LuTriangleAlert />} wide>
          <DiagnosticsPanel
            command={command}
            attentionRows={attentionRows}
            pluginErrors={pluginErrors}
            issueCount={issueCount}
          />
        </Panel>
      </section>
    </main>
  )
}

function StatusPill({ state }: { state: DevtoolStatusConnectionState }) {
  return (
    <span
      className={['bldr-dev-status__pill', statusPillStateClass[state]].join(
        ' ',
      )}
    >
      {state}
    </span>
  )
}

function SummaryTile({
  icon,
  label,
  value,
  detail,
}: {
  icon: ReactNode
  label: string
  value: string
  detail: string
}) {
  return (
    <div className="bldr-dev-status__tile">
      <span className="bldr-dev-status__tile-icon">{icon}</span>
      <span className="bldr-dev-status__tile-label">{label}</span>
      <strong>{value}</strong>
      <span>{detail}</span>
    </div>
  )
}

function Panel({
  children,
  icon,
  title,
  wide,
}: {
  children: ReactNode
  icon: ReactNode
  title: string
  wide?: boolean
}) {
  return (
    <section
      className={
        wide ?
          'bldr-dev-status__panel bldr-dev-status__panel--wide'
        : 'bldr-dev-status__panel'
      }
    >
      <h2>
        {icon}
        {title}
      </h2>
      {children}
    </section>
  )
}

function CommandPanel({ command }: { command?: DevtoolStatusCommand }) {
  if (!command) {
    return <EmptyState text="Waiting for command status" />
  }
  return (
    <div className="bldr-dev-status__stack">
      <KeyValue label="Name" value={command.name || 'unknown'} />
      <KeyValue label="State" value={formatCommandState(command.state)} />
      <KeyValue label="Summary" value={command.summary || 'No summary'} />
      {command.error && <KeyValue label="Error" value={command.error} />}
      {command.logFile && <LogPath value={command.logFile} />}
    </div>
  )
}

function TargetsPanel({ targets }: { targets: DevtoolStatusBuildTarget[] }) {
  if (targets.length === 0) {
    return <EmptyState text="No build targets configured" />
  }
  return (
    <div className="bldr-dev-status__rows">
      {targets.map((target) => (
        <div className="bldr-dev-status__row" key={target.id}>
          <div>
            <strong>{target.id}</strong>
            <span>{target.manifestIds?.join(', ') || 'No manifests'}</span>
          </div>
          <div className="bldr-dev-status__chips">
            {(target.resolvedPlatformIds ?? []).map((platform) => (
              <span key={platform}>{platform}</span>
            ))}
            {target.error && <span className="is-error">{target.error}</span>}
          </div>
        </div>
      ))}
    </div>
  )
}

function ManifestPanel({
  buildRows,
  fetchRows,
}: {
  buildRows: DevtoolStatusManifestBuildRow[]
  fetchRows: DevtoolStatusManifestFetchRow[]
}) {
  if (buildRows.length === 0 && fetchRows.length === 0) {
    return <EmptyState text="No manifest activity" />
  }
  return (
    <div className="bldr-dev-status__rows">
      {buildRows.map((row) => (
        <div className="bldr-dev-status__row" key={`build:${row.id}`}>
          <div>
            <strong>{row.manifestId || row.id}</strong>
            <span>
              {formatManifestState(row.state)} build
              {row.platformId ? ` on ${row.platformId}` : ''}
            </span>
          </div>
          <div className="bldr-dev-status__chips">
            {(row.buildTargetIds ?? []).map((target) => (
              <span key={target}>{target}</span>
            ))}
            {row.cacheHit && <span>cache hit</span>}
            {row.hotRebuild && <span>hot rebuild</span>}
            {row.fullRebuild && <span>full rebuild</span>}
            {row.error && <span className="is-error">{row.error}</span>}
          </div>
        </div>
      ))}
      {fetchRows.map((row) => (
        <div className="bldr-dev-status__row" key={`fetch:${row.id}`}>
          <div>
            <strong>{row.manifestId || row.id}</strong>
            <span>
              {formatManifestState(row.state)} fetch
              {row.blockedOnLocalBuild ? ' blocked on local build' : ''}
            </span>
          </div>
          <div className="bldr-dev-status__chips">
            {(row.platformIds ?? []).map((platform) => (
              <span key={platform}>{platform}</span>
            ))}
            {row.readyRefCount ?
              <span>{row.readyRefCount} refs</span>
            : null}
            {row.error && <span className="is-error">{row.error}</span>}
          </div>
        </div>
      ))}
    </div>
  )
}

function RuntimePanel({
  plugins,
  controllers,
}: {
  plugins: DevtoolStatusPluginRow[]
  controllers: DevtoolStatusControllerRow[]
}) {
  if (plugins.length === 0 && controllers.length === 0) {
    return <EmptyState text="No runtime rows" />
  }
  return (
    <div className="bldr-dev-status__rows">
      {plugins.map((row) => (
        <div className="bldr-dev-status__row" key={`plugin:${row.id}`}>
          <div>
            <strong>{row.pluginId || row.id}</strong>
            <span>{formatPluginState(row.state)}</span>
          </div>
          <div className="bldr-dev-status__chips">
            {row.instanceKey && <span>{row.instanceKey}</span>}
            {row.error && <span className="is-error">{row.error}</span>}
          </div>
        </div>
      ))}
      {controllers.map((row) => (
        <div className="bldr-dev-status__row" key={`controller:${row.id}`}>
          <div>
            <strong>{row.controllerId || row.id}</strong>
            <span>{row.kind || 'controller'}</span>
          </div>
          <div className="bldr-dev-status__chips">
            <span>{formatControllerState(row.state)}</span>
            {row.error && <span className="is-error">{row.error}</span>}
          </div>
        </div>
      ))}
    </div>
  )
}

function DiagnosticsPanel({
  attentionRows,
  command,
  issueCount,
  pluginErrors,
}: {
  attentionRows: DevtoolStatusAttentionRow[]
  command?: DevtoolStatusCommand
  issueCount: number
  pluginErrors: DevtoolStatusPluginRow[]
}) {
  const hasRows = attentionRows.length !== 0 || pluginErrors.length !== 0
  return (
    <div className="bldr-dev-status__diagnostics">
      <div className="bldr-dev-status__diagnostic-head">
        <span>{issueCount} issues</span>
        <span>Structured logs unavailable</span>
      </div>
      {command?.logFile && <LogPath value={command.logFile} />}
      {!hasRows && <EmptyState text="No diagnostic rows" />}
      {hasRows && (
        <div className="bldr-dev-status__rows">
          {attentionRows.map((row) => (
            <div className="bldr-dev-status__row" key={`attention:${row.id}`}>
              <div>
                <strong>{row.message || row.source || row.id}</strong>
                <span>
                  {row.detail || formatAttentionSeverity(row.severity)}
                </span>
              </div>
              <div className="bldr-dev-status__chips">
                <span>{formatAttentionSeverity(row.severity)}</span>
              </div>
            </div>
          ))}
          {pluginErrors.map((row) => (
            <div
              className="bldr-dev-status__row"
              key={`plugin-error:${row.id}`}
            >
              <div>
                <strong>{row.pluginId || row.id}</strong>
                <span>{row.error}</span>
              </div>
              <div className="bldr-dev-status__chips">
                {row.lastErrorAt && <span>{row.lastErrorAt}</span>}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function LogPath({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="bldr-dev-status__log-path">
      <span>{value}</span>
      <div className="bldr-dev-status__log-path-actions">
        <a
          aria-label="Open log file"
          href={localLogHref(value)}
          rel="noreferrer"
          target="_blank"
          title="Open log file"
        >
          <LuExternalLink />
        </a>
        <button
          aria-label="Copy log path"
          onClick={() => {
            void navigator.clipboard?.writeText(value)
            setCopied(true)
          }}
          title="Copy log path"
          type="button"
        >
          {copied ?
            <LuCheck />
          : <LuCopy />}
        </button>
      </div>
    </div>
  )
}

function localLogHref(value: string): string {
  if (value.startsWith('file://')) {
    return value
  }
  if (value.startsWith('/')) {
    return `file://${value}`
  }
  return value
}

function KeyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="bldr-dev-status__kv">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function EmptyState({ text }: { text: string }) {
  return <p className="bldr-dev-status__empty">{text}</p>
}

function commandSummary(command?: DevtoolStatusCommand): string {
  if (!command) {
    return 'connecting'
  }
  return command.error || command.summary || formatCommandState(command.state)
}

function manifestStateSummary(rows: DevtoolStatusManifestBuildRow[]): string {
  const errored = rows.filter(
    (row) =>
      row.state === DevtoolStatusManifestState.DevtoolStatusManifestState_ERROR,
  ).length
  const running = rows.filter(
    (row) =>
      row.state ===
      DevtoolStatusManifestState.DevtoolStatusManifestState_RUNNING,
  ).length
  return `${running} running, ${errored} errored`
}

function pluginStateSummary(rows: DevtoolStatusPluginRow[]): string {
  const running = rows.filter(
    (row) =>
      row.state === DevtoolStatusPluginState.DevtoolStatusPluginState_RUNNING,
  ).length
  const errored = rows.filter(
    (row) =>
      row.state === DevtoolStatusPluginState.DevtoolStatusPluginState_ERRORED,
  ).length
  return `${running} running, ${errored} errored`
}

function isClosedSnapshot(snapshot: DevtoolStatusSnapshot | null): boolean {
  const state = snapshot?.command?.state
  return (
    state === DevtoolStatusCommandState.DevtoolStatusCommandState_DONE ||
    state === DevtoolStatusCommandState.DevtoolStatusCommandState_ERROR ||
    state === DevtoolStatusCommandState.DevtoolStatusCommandState_CANCELED
  )
}

function formatCommandState(state?: DevtoolStatusCommandState): string {
  switch (state) {
    case DevtoolStatusCommandState.DevtoolStatusCommandState_STARTING:
      return 'starting'
    case DevtoolStatusCommandState.DevtoolStatusCommandState_RUNNING:
      return 'running'
    case DevtoolStatusCommandState.DevtoolStatusCommandState_DONE:
      return 'done'
    case DevtoolStatusCommandState.DevtoolStatusCommandState_ERROR:
      return 'error'
    case DevtoolStatusCommandState.DevtoolStatusCommandState_CANCELED:
      return 'canceled'
    default:
      return 'unknown'
  }
}

function formatManifestState(state?: DevtoolStatusManifestState): string {
  switch (state) {
    case DevtoolStatusManifestState.DevtoolStatusManifestState_QUEUED:
      return 'queued'
    case DevtoolStatusManifestState.DevtoolStatusManifestState_RUNNING:
      return 'running'
    case DevtoolStatusManifestState.DevtoolStatusManifestState_READY:
      return 'ready'
    case DevtoolStatusManifestState.DevtoolStatusManifestState_ERROR:
      return 'error'
    case DevtoolStatusManifestState.DevtoolStatusManifestState_CANCELED:
      return 'canceled'
    default:
      return 'unknown'
  }
}

function formatPluginState(state?: DevtoolStatusPluginState): string {
  switch (state) {
    case DevtoolStatusPluginState.DevtoolStatusPluginState_REQUESTED:
      return 'requested'
    case DevtoolStatusPluginState.DevtoolStatusPluginState_RUNNING:
      return 'running'
    case DevtoolStatusPluginState.DevtoolStatusPluginState_ERRORED:
      return 'errored'
    default:
      return 'unknown'
  }
}

function formatControllerState(state?: DevtoolStatusControllerState): string {
  switch (state) {
    case DevtoolStatusControllerState.DevtoolStatusControllerState_REQUESTED:
      return 'requested'
    case DevtoolStatusControllerState.DevtoolStatusControllerState_RUNNING:
      return 'running'
    case DevtoolStatusControllerState.DevtoolStatusControllerState_IDLE:
      return 'idle'
    case DevtoolStatusControllerState.DevtoolStatusControllerState_ERROR:
      return 'error'
    default:
      return 'unknown'
  }
}

function formatAttentionSeverity(
  severity?: DevtoolStatusAttentionSeverity,
): string {
  switch (severity) {
    case DevtoolStatusAttentionSeverity.DevtoolStatusAttentionSeverity_INFO:
      return 'info'
    case DevtoolStatusAttentionSeverity.DevtoolStatusAttentionSeverity_WARNING:
      return 'warning'
    case DevtoolStatusAttentionSeverity.DevtoolStatusAttentionSeverity_ERROR:
      return 'error'
    default:
      return 'unknown'
  }
}

export const devtoolStatusServiceIDForTest = devtoolStatusServiceID
