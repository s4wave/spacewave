import { useCallback, useMemo, useReducer } from 'react'
import { LuChartLine, LuPlus } from 'react-icons/lu'

import { isDesktop } from '@aptre/bldr'
import { resourceContent, resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  DashboardTypeID,
  PromDashboard,
  type PromTargets,
} from '@s4wave/sdk/prom/prom.js'
import { Dashboard, type Panel } from '@s4wave/sdk/prom/prom.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useClock } from '@s4wave/web/hooks/useClock.js'
import {
  useHeldTypedHandle,
  useWatchTypedHandle,
} from '@s4wave/web/hooks/useWatchTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { localNavigation } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import {
  ViewerStatus,
  resourceStatus,
} from '@s4wave/web/object/ViewerStatus.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'

import { AlarmStrip } from '@s4wave/app/alarm/AlarmStrip.js'

import { PromPanel } from './PromPanel.js'
import { PromPanelEditor } from './PromPanelEditor.js'
import { PromTargetForm } from './PromTargetForm.js'
import {
  clockPeriodMs,
  formatClock,
  parseTimeRange,
  timeRanges,
  type TimeRange,
} from './prom-view.js'

export { DashboardTypeID }

// PromDashboardViewer shows a prom/dashboard object under its Alarm strip.
// The plugin that answers its queries runs only on desktop Devices, so the
// viewer follows whether the plugin serves the object here; the strip reads
// the World and shows everywhere.
export function PromDashboardViewer(props: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(props.objectInfo)
  const container = SpaceContainerContext.useContextSafe()
  const openAlarm = useCallback(
    (key: string) => container?.navigateToObjects([key]),
    [container],
  )
  return (
    <div className="flex h-full w-full flex-col">
      <AlarmStrip
        worldState={props.worldState}
        dashboardKey={objectKey}
        onOpen={openAlarm}
      />
      <div className="min-h-0 flex-1">
        <DashboardHandle {...props} />
      </div>
    </div>
  )
}

// DashboardHandle follows whether the metrics plugin serves the Dashboard
// here and shows it when it does. Past its reconnect backoff the watch fails;
// the Dashboard it last served stays mounted under the failure, so an open
// editor keeps its draft, and writes wait for the next handle.
function DashboardHandle({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useWatchTypedHandle(
    worldState,
    objectKey,
    PromDashboard,
    DashboardTypeID,
  )

  const value = handle.value
  const held = useHeldTypedHandle(handle, objectKey)
  if (held) {
    return (
      <DashboardBody
        dashboard={held.handle}
        live={held.live}
        lost={handle.error}
        retryHandle={handle.retry}
      />
    )
  }
  if (handle.error) {
    return (
      <ViewerStatus
        state="failed"
        message={`Could not read the Dashboard: ${handle.error.message}`}
        retry={handle.retry}
      />
    )
  }
  switch (value?.state) {
    case 'absent':
      return (
        <ViewerStatus
          state="failed"
          message="This Dashboard no longer exists."
        />
      )
    case 'no-handler':
      return isDesktop ? (
        <ViewerStatus state="waiting" what="the metrics plugin" />
      ) : (
        <ViewerStatus
          state="empty"
          icon={<LuChartLine />}
          message="Metrics queries run on a desktop Device. Open this Space in the Spacewave desktop app to see these panels."
        />
      )
    default:
      return <ViewerStatus state="waiting" what="the Dashboard" />
  }
}

// View is the Dashboard's local state: what the writer is editing, the
// shared hover time, and the outcome of the last panel action.
type View = {
  editing:
    | { state: 'none' }
    | { state: 'panel'; panelId: string | null }
    | { state: 'target' }
  hoverMs: number | null
  actionError: string | null
}

// ViewEvent moves the Dashboard view between states.
type ViewEvent =
  | { kind: 'edit-panel'; panelId: string | null }
  | { kind: 'add-target' }
  | { kind: 'close' }
  | { kind: 'hover'; ms: number | null }
  | { kind: 'action-failed'; reason: string }
  | { kind: 'action-done' }

// reduce applies one event to the view.
function reduce(view: View, ev: ViewEvent): View {
  switch (ev.kind) {
    case 'edit-panel':
      return { ...view, editing: { state: 'panel', panelId: ev.panelId } }
    case 'add-target':
      return { ...view, editing: { state: 'target' } }
    case 'close':
      return { ...view, editing: { state: 'none' } }
    case 'hover':
      return view.hoverMs === ev.ms ? view : { ...view, hoverMs: ev.ms }
    case 'action-failed':
      return { ...view, actionError: ev.reason }
    case 'action-done':
      return view.actionError === null ? view : { ...view, actionError: null }
  }
}

const initialView: View = {
  editing: { state: 'none' },
  hoverMs: null,
  actionError: null,
}

// DashboardBody shows the Dashboard's panels over the time range in the
// view's path, rerunning them as targets commit. Unless live, dashboard is
// the last handle the watch served, which failed with lost or reloads: the
// records and any open editor stay, drawn under the failure, and no query
// or write is offered.
function DashboardBody({
  dashboard,
  live,
  lost,
  retryHandle,
}: {
  dashboard: PromDashboard
  live: boolean
  lost: Error | null
  retryHandle: () => void
}) {
  const container = SpaceContainerContext.useContextSafe()
  const canSave = live
  const canWrite = (container?.canDeleteObjects ?? false) && canSave
  const range = parseTimeRange(usePath())
  const navigate = useNavigate()
  const [view, dispatch] = useReducer(reduce, initialView)
  const nowMs = useClock(clockPeriodMs)

  // Follow the Dashboard and the Space's targets. A lost handle fails both
  // watches over their last records, and their Retry watches the handle;
  // they stay failed over them while it reloads, until the next handle's
  // watches yield.
  const dashResource = useMemo(
    () =>
      resourceOf({
        value: dashboard,
        loading: !live && !lost,
        error: lost,
        retry: lost ? retryHandle : noop,
      }),
    [dashboard, live, lost, retryHandle],
  )
  const dash = useStreamingResource(
    dashResource,
    useCallback(
      (d: PromDashboard, signal: AbortSignal) => d.watchDashboard(signal),
      [],
    ),
    [],
    { equals: Dashboard.equals, reconnect: true },
  )
  const targets = useStreamingResource(
    dashResource,
    useCallback(
      (d: PromDashboard, signal: AbortSignal) => d.watchTargets(signal),
      [],
    ),
    [],
    { reconnect: true },
  )
  // Past their reconnect backoff the watches fail and keep their last
  // records, drawn under the failure notice.
  const dashValue = resourceContent(dash)
  const targetsValue = resourceContent(targets)
  const failure = resourceStatus('the Dashboard', dash, targets)
  const panels = dashValue?.panels ?? []

  // Run a panel action, keeping its failure on screen.
  const act = useCallback(async (fn: () => Promise<void>) => {
    try {
      await fn()
      dispatch({ kind: 'action-done' })
    } catch (err) {
      dispatch({
        kind: 'action-failed',
        reason: err instanceof Error ? err.message : String(err),
      })
    }
  }, [])
  const onHover = useCallback(
    (ms: number | null) => dispatch({ kind: 'hover', ms }),
    [],
  )
  const openTarget = useCallback(
    (key: string) => container?.navigateToObjects([key]),
    [container],
  )
  const closeEditor = useCallback(() => dispatch({ kind: 'close' }), [])
  const setRange = useCallback(
    (r: TimeRange) => navigate(localNavigation({ path: `/${r.id}` })),
    [navigate],
  )

  const editing = view.editing
  const editingPanelId = editing.state === 'panel' ? editing.panelId : undefined
  const status = renderStatus()
  const body = status ? null : renderBody()
  const notice = !status && failure ? <ViewerStatus {...failure} /> : null
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <header className="border-foreground/8 flex h-9 shrink-0 items-center gap-3 border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold tracking-tight select-none">
          <LuChartLine className="text-foreground-alt size-3.5" aria-hidden />
          Dashboard
        </div>
        <UpdatedAt targets={targetsValue} />
        <div
          className="ml-auto flex items-center gap-0.5"
          role="group"
          aria-label="Time range"
        >
          {timeRanges.map((r) => (
            <button
              key={r.id}
              type="button"
              aria-pressed={r.id === range.id}
              onClick={() => setRange(r)}
              className={cn(
                'rounded px-1.5 py-0.5 text-xs tabular-nums',
                r.id === range.id
                  ? 'bg-foreground/10 text-foreground'
                  : 'text-foreground-alt hover:text-foreground',
              )}
            >
              {r.id}
            </button>
          ))}
        </div>
        {canWrite && panels.length > 0 ? (
          <DashboardButton
            icon={<LuPlus className="size-3.5" />}
            onClick={() => dispatch({ kind: 'edit-panel', panelId: null })}
          >
            Add panel
          </DashboardButton>
        ) : null}
      </header>
      {view.actionError ? (
        <div className="px-4 pt-3">
          <ErrorState
            variant="inline"
            title="Could not save the Dashboard"
            message={view.actionError}
          />
        </div>
      ) : null}
      {notice}
      {status ? (
        <div className="min-h-0 flex-1">{status}</div>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto p-3">{body}</div>
      )}
    </div>
  )

  // renderStatus renders the Dashboard's read state until it and its targets
  // arrive, flush with the pane under its header. The body needs both
  // records, so a failure of either replaces it until both arrived; after
  // that the failure is a notice above the kept records.
  function renderStatus() {
    if (dashValue && targetsValue) return null
    if (failure) return <ViewerStatus {...failure} />
    return <ViewerStatus state="waiting" what="the Dashboard" />
  }

  // renderBody renders the Dashboard once it and its targets arrive.
  function renderBody() {
    // Draw nothing until the status gives way to both records. Kept records
    // under a failure make no empty claim.
    if (!dashValue || !targetsValue) return null
    const failed = failure?.state === 'failed'
    if (editing.state === 'target') {
      return (
        <PromTargetForm
          dashboard={dashboard}
          canSave={canSave}
          onCancel={closeEditor}
          onCreated={(key) => {
            closeEditor()
            openTarget(key)
          }}
        />
      )
    }
    if (targetsValue.size === 0 && !failed) {
      return (
        <EmptyState
          icon={<LuChartLine className="text-foreground-alt size-5" />}
          title="No Scrape targets in this Space yet."
          action={
            canWrite
              ? {
                  label: 'Add Scrape target',
                  onClick: () => dispatch({ kind: 'add-target' }),
                }
              : undefined
          }
        />
      )
    }
    if (panels.length === 0 && editingPanelId === undefined && !failed) {
      return (
        <EmptyState
          icon={<LuChartLine className="text-foreground-alt size-5" />}
          title="This Dashboard has no panels."
          action={
            canWrite
              ? {
                  label: 'Add panel',
                  onClick: () =>
                    dispatch({ kind: 'edit-panel', panelId: null }),
                }
              : undefined
          }
        />
      )
    }

    const editedRemoved =
      typeof editingPanelId === 'string' &&
      !panels.some((p) => p.id === editingPanelId)
    return (
      <div className="@container">
        <div className="grid gap-3 @3xl:grid-cols-2 @6xl:grid-cols-3">
          {panels.map((panel, i) =>
            panel.id === editingPanelId ? (
              <PromPanelEditor
                key={panel.id}
                dashboard={dashboard}
                live={live}
                panel={panel}
                removed={false}
                targets={targetsValue}
                range={range}
                nowMs={nowMs}
                onClose={closeEditor}
              />
            ) : (
              <PromPanel
                key={panel.id}
                query={dashboard}
                live={live}
                panel={panel}
                range={range}
                targets={targetsValue}
                nowMs={nowMs}
                hoverMs={view.hoverMs}
                onHover={onHover}
                onOpenTarget={openTarget}
                actions={canWrite ? panelActions(panel, i) : undefined}
              />
            ),
          )}
          {editingPanelId === null || editedRemoved ? (
            <PromPanelEditor
              key={editingPanelId ?? 'new'}
              dashboard={dashboard}
              live={live}
              panel={null}
              removed={editedRemoved}
              targets={targetsValue}
              range={range}
              nowMs={nowMs}
              onClose={closeEditor}
            />
          ) : null}
        </div>
      </div>
    )
  }

  // panelActions returns a writer's actions on the panel at index.
  function panelActions(panel: Panel, index: number) {
    const id = panel.id ?? ''
    return {
      onEdit: () => dispatch({ kind: 'edit-panel', panelId: id }),
      onMoveUp:
        index > 0
          ? () => void act(() => dashboard.movePanel(id, index - 1))
          : undefined,
      onMoveDown:
        index < panels.length - 1
          ? () => void act(() => dashboard.movePanel(id, index + 1))
          : undefined,
      onRemove: () => void act(() => dashboard.removePanel(id)),
    }
  }
}

// UpdatedAt shows when the latest target commit landed.
function UpdatedAt({ targets }: { targets: PromTargets | null }) {
  let latest = 0
  for (const info of targets?.values() ?? []) {
    latest = Math.max(latest, Number(info.status?.lastScrapeMs ?? 0n))
  }
  if (latest === 0) return null
  return (
    <span className="text-foreground-alt text-xs tabular-nums">
      Updated {formatClock(latest)}
    </span>
  )
}

// noop is the retry of a resource that cannot fail.
function noop() {}
