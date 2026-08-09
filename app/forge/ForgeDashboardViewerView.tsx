import {
  LuActivity,
  LuBox,
  LuBriefcase,
  LuLayoutDashboard,
  LuPlus,
  LuServer,
} from 'react-icons/lu'

import { ForgeEntityLink } from '@s4wave/web/forge/ForgeEntityLink.js'
import { ForgeViewerShell } from '@s4wave/web/forge/ForgeViewerShell.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { ProcessBindingList } from './ProcessBindingList.js'
import { forgeEntityTypeLabels } from './useForgeDashboardViewerController.js'
import type { ForgeDashboardViewerController } from './useForgeDashboardViewerController.js'

type Controller = ForgeDashboardViewerController

function EntityLoadingState({
  controller,
  activity,
}: {
  controller: Controller
  activity?: boolean
}) {
  if (controller.worldError)
    return (
      <LoadingCard
        view={{
          state: 'error',
          title: activity
            ? 'Forge activity unavailable'
            : 'Forge entities unavailable',
          detail: activity
            ? 'This dashboard could not read recent Forge work.'
            : 'This dashboard could not read its linked Forge work.',
          error: `Try again to reload Forge ${activity ? 'activity' : 'entities'}.`,
          onRetry: controller.retryWorld,
        }}
      />
    )
  if (controller.entitiesLoading && controller.entities.length === 0)
    return (
      <LoadingCard
        view={{
          state: 'loading',
          title: activity
            ? 'Loading Forge activity…'
            : 'Loading Forge entities…',
          detail: activity
            ? 'Reading recent jobs, tasks, and executions.'
            : 'Reading Jobs, Workers, Clusters, and active work.',
          progressIndeterminate: true,
        }}
      />
    )
  return null
}

function DashboardOverviewTab({ controller }: { controller: Controller }) {
  return (
    <div className="space-y-3">
      {controller.openingAction && (
        <div
          role="status"
          className="border-brand/20 bg-brand/5 text-foreground-alt/70 rounded-lg border px-3.5 py-2.5 text-xs"
        >
          Opening Forge {controller.openingAction}…
        </div>
      )}
      <EntityLoadingState controller={controller} />
      {!controller.worldError &&
        !controller.entitiesLoading &&
        controller.entities.length === 0 && (
          <InfoCard
            icon={<LuLayoutDashboard className="text-brand size-4" />}
            title="No Forge work yet"
          >
            <p className="text-foreground-alt/70 text-xs leading-relaxed">
              Create a Job to define work, or create a Cluster to attach
              execution capacity.
            </p>
          </InfoCard>
        )}
      {controller.entities.length > 0 && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {controller.summaryMetrics.map((metric) => (
            <div
              key={metric.label}
              className="border-foreground/6 bg-background-card/30 rounded-lg border p-3"
            >
              <div className="text-foreground-alt/60 mb-1 text-[0.6rem] font-medium tracking-widest uppercase">
                {metric.label}
              </div>
              <div className="text-foreground text-xl font-semibold tabular-nums">
                {metric.count}
              </div>
            </div>
          ))}
        </div>
      )}
      {controller.pendingWorkers.length > 0 && (
        <InfoCard
          icon={<LuPlus className="text-brand size-4" />}
          title={
            controller.pendingWorkers.length === 1
              ? 'Worker ready to start'
              : 'Workers ready to start'
          }
        >
          <p className="text-foreground-alt/70 text-xs leading-relaxed">
            Approve the quickstart worker process binding to start task
            execution in this session.
          </p>
          <button
            type="button"
            onClick={() => void controller.startWorkers()}
            className="border-brand/40 bg-brand/10 hover:border-brand/60 hover:bg-brand/15 text-foreground mt-3 inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition duration-150"
          >
            <LuPlus className="size-3.5" />
            {controller.pendingWorkers.length === 1
              ? 'Start worker'
              : 'Start workers'}
          </button>
        </InfoCard>
      )}
      {controller.entities.length > 0 && (
        <div className="grid gap-3 lg:grid-cols-2">
          <EntityHealth controller={controller} />
          <RecentActivity controller={controller} />
        </div>
      )}
    </div>
  )
}

function EntityHealth({ controller }: { controller: Controller }) {
  return (
    <InfoCard
      icon={<LuBox className="text-foreground-alt/70 size-3.5" />}
      title="Entity health"
    >
      <div className="space-y-1.5">
        {Object.entries(controller.typeCounts).map(([label, count]) => (
          <div
            key={label}
            className="text-foreground-alt/70 flex items-center justify-between gap-3 text-xs"
          >
            <span>{label}</span>
            <span className="text-foreground font-medium tabular-nums">
              {count} linked
            </span>
          </div>
        ))}
      </div>
    </InfoCard>
  )
}
function RecentActivity({ controller }: { controller: Controller }) {
  return (
    <InfoCard
      icon={<LuActivity className="text-foreground-alt/70 size-3.5" />}
      title="Recent activity"
    >
      {controller.activityLoading &&
        controller.activityEntries.length === 0 && (
          <div className="text-foreground-alt/60 flex items-center gap-2 text-xs">
            <LuActivity className="size-3.5 shrink-0" />
            <span>Loading Forge activity…</span>
          </div>
        )}
      {!controller.activityLoading &&
        controller.activityEntries.length === 0 && (
          <p className="text-foreground-alt/60 text-xs leading-relaxed">
            No recent activity yet. Activity will appear as Forge work changes;
            no action is required.
          </p>
        )}
      {controller.activityEntries.slice(0, 3).map((entry) => (
        <div
          key={entry.id}
          className="border-foreground/6 flex items-start gap-2 border-b py-2 last:border-b-0 last:pb-0"
        >
          <LuActivity className="text-foreground-alt/60 mt-0.5 size-3 shrink-0" />
          <div className="min-w-0">
            <div className="text-foreground text-xs font-medium">
              {entry.title}
            </div>
            <div className="text-foreground-alt/50 truncate text-xs">
              {entry.detail}
            </div>
          </div>
        </div>
      ))}
    </InfoCard>
  )
}

function ActivityEntry({
  entry,
}: {
  entry: Controller['activityEntries'][number]
}) {
  const content = (
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      <div className="text-foreground text-xs font-medium">{entry.title}</div>
      <div className="text-foreground-alt/50 truncate text-xs">
        {entry.detail}
      </div>
      <div className="text-foreground-alt/50 text-xs">
        {entry.timestamp.toISOString()}
      </div>
    </div>
  )
  return entry.objectKey ? (
    <ForgeEntityLink
      objectKey={entry.objectKey}
      icon={<LuActivity className="text-foreground-alt/60 size-3 shrink-0" />}
    >
      {content}
    </ForgeEntityLink>
  ) : (
    <div className="border-foreground/6 bg-background-card/30 flex items-start gap-2 rounded-lg border p-3">
      <LuActivity className="text-foreground-alt/60 mt-0.5 size-3 shrink-0" />
      {content}
    </div>
  )
}
function DashboardActivityTab({ controller }: { controller: Controller }) {
  return (
    <div className="space-y-2">
      <EntityLoadingState controller={controller} activity />
      {!controller.worldError &&
        !controller.activityLoading &&
        controller.activityEntries.length === 0 && (
          <InfoCard
            icon={<LuActivity className="text-foreground-alt/70 size-3.5" />}
            title="No recent activity yet"
          >
            <p className="text-foreground-alt/60 text-xs leading-relaxed">
              Activity will appear as Forge work changes; no action is required.
            </p>
          </InfoCard>
        )}
      {controller.activityEntries.map((entry) => (
        <ActivityEntry key={entry.id} entry={entry} />
      ))}
    </div>
  )
}
function DashboardEntitiesTab({ controller }: { controller: Controller }) {
  return (
    <div className="space-y-2">
      <EntityLoadingState controller={controller} />
      {!controller.worldError &&
        !controller.entitiesLoading &&
        controller.entities.length === 0 && (
          <InfoCard
            icon={<LuBox className="text-foreground-alt/70 size-3.5" />}
            title="No linked Forge entities"
          >
            <p className="text-foreground-alt/60 text-xs leading-relaxed">
              Create a Job or Cluster to add Forge entities to this dashboard.
            </p>
          </InfoCard>
        )}
      {controller.entities.map((entity) => (
        <ForgeEntityLink
          key={entity.objectKey}
          objectKey={entity.objectKey}
          icon={<LuBox className="text-foreground-alt/60 size-3 shrink-0" />}
        >
          {entity.typeId && (
            <StateBadge
              state={0}
              labels={{
                0: forgeEntityTypeLabels[entity.typeId] || entity.typeId,
              }}
              variant="dot"
            />
          )}
          <span className="text-sm font-medium">{entity.objectKey}</span>
        </ForgeEntityLink>
      ))}
    </div>
  )
}
function DashboardBindingsTab({ controller }: { controller: Controller }) {
  if (controller.bindingsError)
    return (
      <LoadingCard
        view={{
          state: 'error',
          title: 'Process bindings unavailable',
          detail: 'Forge could not read worker approval state.',
          error: 'Try again to refresh process bindings.',
          onRetry: controller.retryBindings,
        }}
      />
    )
  if (controller.bindingsLoading)
    return (
      <LoadingCard
        view={{
          state: 'loading',
          title: 'Loading process bindings…',
          detail: 'Reading worker approval state for this Space.',
          progressIndeterminate: true,
        }}
      />
    )
  if (controller.bindings.length > 0)
    return (
      <ProcessBindingList
        bindings={controller.bindings}
        onToggle={(objectKey, approved) =>
          void controller.toggleBinding(objectKey, approved)
        }
      />
    )
  return (
    <InfoCard
      icon={<LuBox className="text-foreground-alt/70 size-3.5" />}
      title="No process bindings"
    >
      <p className="text-foreground-alt/60 text-xs leading-relaxed">
        Bindings appear when Forge workers are linked; no action is required
        yet.
      </p>
    </InfoCard>
  )
}

export function ForgeDashboardViewerView({
  controller,
}: {
  controller: Controller
}) {
  const headerActions = [
    ...(controller.canCreateJob
      ? [
          {
            label:
              controller.openingAction === 'job'
                ? 'Opening Job…'
                : 'Create Job',
            icon: <LuBriefcase className="size-3.5" />,
            variant: 'primary' as const,
            disabled: controller.openingAction !== null,
            onClick: () => void controller.createJob(),
          },
        ]
      : []),
    ...(controller.canCreateCluster
      ? [
          {
            label:
              controller.openingAction === 'cluster'
                ? 'Opening Cluster…'
                : 'Create Cluster',
            icon: <LuServer className="size-3.5" />,
            disabled: controller.openingAction !== null,
            onClick: () => void controller.createCluster(),
          },
        ]
      : []),
  ]
  const tabs = [
    {
      id: 'overview',
      label: 'Overview',
      content: <DashboardOverviewTab controller={controller} />,
    },
    {
      id: 'activity',
      label: 'Activity',
      content: <DashboardActivityTab controller={controller} />,
    },
    {
      id: 'entities',
      label: 'Entities',
      content: <DashboardEntitiesTab controller={controller} />,
    },
    {
      id: 'bindings',
      label: 'Bindings',
      content: <DashboardBindingsTab controller={controller} />,
    },
  ]
  const headerStatus = controller.creationError ? (
    <div
      className="border-destructive/15 bg-destructive/5 text-destructive shrink-0 border-b px-4 py-2 text-xs leading-relaxed"
      role="alert"
    >
      {controller.creationError}
    </div>
  ) : undefined
  return (
    <ForgeViewerShell
      icon={<LuLayoutDashboard className="size-4" />}
      title={controller.dashboard?.name || 'Forge Dashboard'}
      tabs={tabs}
      headerActions={headerActions}
      headerStatus={headerStatus}
      stateKey={controller.objectKey}
    />
  )
}
