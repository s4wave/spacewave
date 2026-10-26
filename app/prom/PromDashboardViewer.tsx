import { useCallback, useMemo, useReducer, type ReactNode } from 'react'
import { LuChartLine, LuPlus } from 'react-icons/lu'

import { isDesktop } from '@aptre/bldr'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  DashboardTypeID,
  PromDashboard,
  type PromTargets,
} from '@s4wave/sdk/prom/prom.js'
import { Dashboard, type Panel } from '@s4wave/sdk/prom/prom.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useClock } from '@s4wave/web/hooks/useClock.js'
import { useWatchTypedHandle } from '@s4wave/web/hooks/useWatchTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { localNavigation } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

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
// here and shows it when it does.
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
  if (handle.error) {
    return (
      <Frame>
        <ErrorState
          title="Dashboard unavailable"
          message={handle.error.message}
          onRetry={handle.retry}
        />
      </Frame>
    )
  }
  if (!value) {
    return (
      <Frame>
        <LoadingInline label="Opening the Dashboard" />
      </Frame>
    )
  }
  switch (value.state) {
    case 'absent':
      return (
        <Frame>
          <EmptyState
            icon={<LuChartLine className="text-foreground-alt size-5" />}
            title="This Dashboard no longer exists."
          />
        </Frame>
      )
    case 'no-handler':
      return (
        <Frame>
          {isDesktop ? (
            <LoadingInline label="Starting the metrics plugin" />
          ) : (
            <EmptyState
              icon={<LuChartLine className="text-foreground-alt size-5" />}
              title="Metrics queries run on a desktop Device."
              description="Open this Space in the Spacewave desktop app to see these panels."
            />
          )}
        </Frame>
      )
    case 'ready':
      return <DashboardBody dashboard={value.handle} />
  }
}

// Frame centers a viewer-wide message.
function Frame({ children }: { children: ReactNode }) {
  return (
    <div className="bg-background-primary flex h-full w-full items-center justify-center p-6">
      {children}
    </div>
  )
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
// view's path, rerunning them as targets commit.
function DashboardBody({ dashboard }: { dashboard: PromDashboard }) {
  const container = SpaceContainerContext.useContextSafe()
  const canWrite = container?.canDeleteObjects ?? false
  const range = parseTimeRange(usePath())
  const navigate = useNavigate()
  const [view, dispatch] = useReducer(reduce, initialView)
  const nowMs = useClock(clockPeriodMs)

  // Follow the Dashboard and the Space's targets.
  const dashResource = useMemo(
    () => ({ value: dashboard, loading: false, error: null, retry: noop }),
    [dashboard],
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
  const panels = dash.value?.panels ?? []

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
  const body = renderBody()
  return (
    <div className="bg-background-primary flex h-full w-full flex-col">
      <header className="border-foreground/8 flex h-9 shrink-0 items-center gap-3 border-b px-4">
        <div className="text-foreground flex items-center gap-2 text-sm font-semibold tracking-tight select-none">
          <LuChartLine className="text-foreground-alt size-3.5" aria-hidden />
          Dashboard
        </div>
        <UpdatedAt targets={targets.value} />
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
      <div className="min-h-0 flex-1 overflow-auto p-3">{body}</div>
    </div>
  )

  // renderBody renders the Dashboard's state below its header.
  function renderBody() {
    if (dash.error) {
      return (
        <ErrorState
          title="Dashboard unavailable"
          message={dash.error.message}
          onRetry={dash.retry}
        />
      )
    }
    if (!dash.value || !targets.value) {
      return <LoadingInline label="Loading the Dashboard" />
    }
    if (editing.state === 'target') {
      return (
        <PromTargetForm
          dashboard={dashboard}
          onCancel={closeEditor}
          onCreated={(key) => {
            closeEditor()
            openTarget(key)
          }}
        />
      )
    }
    if (targets.value.size === 0) {
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
    if (panels.length === 0 && editingPanelId === undefined) {
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
                panel={panel}
                removed={false}
                targets={targets.value}
                range={range}
                nowMs={nowMs}
                onClose={closeEditor}
              />
            ) : (
              <PromPanel
                key={panel.id}
                query={dashboard}
                panel={panel}
                range={range}
                targets={targets.value}
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
              panel={null}
              removed={editedRemoved}
              targets={targets.value}
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
