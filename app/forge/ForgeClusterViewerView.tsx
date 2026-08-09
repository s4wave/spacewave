import { useMemo } from 'react'
import {
  LuBriefcase,
  LuCpu,
  LuListTodo,
  LuPlay,
  LuPlus,
  LuServer,
} from 'react-icons/lu'

import { State as JobState } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { State as TaskState } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { ForgeEntityLink } from '@s4wave/web/forge/ForgeEntityLink.js'
import { ForgeEntityList } from '@s4wave/web/forge/ForgeEntityList.js'
import {
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { StatCard } from '@s4wave/web/ui/StatCard.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'

import type { ForgeClusterViewerController } from './useForgeClusterViewerController.js'

type Controller = ForgeClusterViewerController

const jobStateLabels: Record<number, string> = {
  [JobState.JobState_UNKNOWN]: 'UNKNOWN',
  [JobState.JobState_PENDING]: 'PENDING',
  [JobState.JobState_RUNNING]: 'RUNNING',
  [JobState.JobState_COMPLETE]: 'COMPLETE',
}
const taskStateLabels: Record<number, string> = {
  [TaskState.TaskState_UNKNOWN]: 'UNKNOWN',
  [TaskState.TaskState_PENDING]: 'PENDING',
  [TaskState.TaskState_RUNNING]: 'RUNNING',
  [TaskState.TaskState_CHECKING]: 'CHECKING',
  [TaskState.TaskState_COMPLETE]: 'COMPLETE',
  [TaskState.TaskState_RETRY]: 'RETRY',
}

function ClusterOverviewTab({ controller }: { controller: Controller }) {
  const {
    cluster,
    workersLoading,
    workers,
    jobsLoading,
    jobs,
    snapshotLoading,
    snapshot,
    taskStateCounts,
  } = controller
  return (
    <div className="space-y-3">
      <InfoCard>
        <div className="space-y-2">
          {cluster?.name && <CopyableField label="Name" value={cluster.name} />}
          {cluster?.peerId && (
            <CopyableField label="Peer ID" value={cluster.peerId} />
          )}
        </div>
      </InfoCard>
      <div className="grid grid-cols-2 gap-3">
        <StatCard
          icon={LuCpu}
          label="Workers"
          value={workersLoading ? '-' : workers.length}
        />
        <StatCard
          icon={LuBriefcase}
          label="Jobs"
          value={jobsLoading ? '-' : jobs.length}
        />
      </div>
      <InfoCard
        icon={<LuListTodo className="text-foreground-alt/60 size-3.5" />}
        title="Task States"
      >
        {snapshotLoading && (
          <div className="text-foreground-alt/50 text-xs">
            Loading task breakdown…
          </div>
        )}
        {!snapshotLoading && snapshot.tasks.length === 0 && (
          <div className="text-foreground-alt/50 text-xs">
            No tasks assigned yet
          </div>
        )}
        {!snapshotLoading && snapshot.tasks.length > 0 && (
          <div className="grid grid-cols-2 gap-2">
            {Object.entries(taskStateLabels).map(([state, label]) => (
              <div
                key={state}
                className="border-foreground/6 bg-background-card/30 rounded-lg border px-3 py-2"
              >
                <div className="text-foreground-alt/60 text-[0.6rem] tracking-widest uppercase">
                  {label}
                </div>
                <div className="text-foreground mt-1 text-lg font-semibold">
                  {taskStateCounts[Number(state)] ?? 0}
                </div>
              </div>
            ))}
          </div>
        )}
      </InfoCard>
    </div>
  )
}

function ClusterWorkersTab({ controller }: { controller: Controller }) {
  const {
    snapshot,
    workers,
    workersLoading,
    snapshotLoading,
    workerExecutionCounts,
  } = controller
  if (snapshot.workers.length === 0)
    return (
      <ForgeEntityList
        entities={workers}
        loading={workersLoading || snapshotLoading}
        icon={<LuCpu className="size-3 shrink-0" />}
        loadingLabel="Loading workers..."
        emptyLabel="No workers assigned"
      />
    )
  return (
    <div className="space-y-2">
      {snapshot.workers.map((worker) => (
        <div
          key={worker.objectKey}
          className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition duration-150"
        >
          <div className="flex items-center justify-between gap-3">
            <ForgeEntityLink
              objectKey={worker.objectKey}
              className="text-foreground min-w-0 text-sm font-medium"
            >
              {worker.data.name || worker.objectKey}
            </ForgeEntityLink>
            <div className="text-foreground-alt/50 text-xs">
              {workerExecutionCounts.get(worker.objectKey) ?? 0} active
            </div>
          </div>
          <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
            <span>{worker.peerIds.length} peer IDs</span>
            <span>{worker.clusterKeys.length} cluster links</span>
          </div>
        </div>
      ))}
    </div>
  )
}

function ClusterJobsTab({ controller }: { controller: Controller }) {
  const {
    canCreateJob,
    createJob,
    creatingJob,
    snapshot,
    jobs,
    jobsLoading,
    snapshotLoading,
    tasksByJobKey,
    startJob,
  } = controller
  return (
    <div className="space-y-3">
      {canCreateJob && (
        <div className="flex justify-end">
          <DashboardButton
            icon={<LuPlus className="size-3.5" />}
            onClick={() => void createJob()}
            disabled={creatingJob}
          >
            {creatingJob ? 'Creating…' : 'Create Job'}
          </DashboardButton>
        </div>
      )}
      {snapshot.jobs.length === 0 ? (
        <ForgeEntityList
          entities={jobs}
          loading={jobsLoading || snapshotLoading}
          icon={<LuBriefcase className="size-3 shrink-0" />}
          loadingLabel="Loading jobs..."
          emptyLabel="No jobs in cluster"
        />
      ) : (
        <div className="space-y-2">
          {snapshot.jobs.map((job) => {
            const jobTasks = tasksByJobKey.get(job.objectKey) ?? []
            const completeTasks = jobTasks.filter(
              (task) =>
                (task.data.taskState ?? TaskState.TaskState_UNKNOWN) ===
                TaskState.TaskState_COMPLETE,
            ).length
            const progressPercent =
              jobTasks.length === 0
                ? 0
                : Math.round((completeTasks / jobTasks.length) * 100)
            const startable =
              (job.data.jobState ?? JobState.JobState_UNKNOWN) ===
              JobState.JobState_PENDING
            return (
              <div
                key={job.objectKey}
                className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-3 rounded-lg border px-3.5 py-2.5 transition duration-150"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <ForgeEntityLink
                      objectKey={job.objectKey}
                      className="text-foreground truncate text-sm font-medium"
                    />
                    <div className="text-foreground-alt/50 mt-1 text-xs">
                      {completeTasks}/{jobTasks.length} tasks complete
                    </div>
                  </div>
                  <StateBadge
                    state={job.data.jobState ?? 0}
                    labels={jobStateLabels}
                  />
                </div>
                <div className="bg-foreground/8 h-1.5 w-full overflow-hidden rounded-full">
                  <div
                    className="bg-brand h-full transition-[width] duration-200"
                    style={{ width: `${progressPercent}%` }}
                  />
                </div>
                {startable && (
                  <div className="flex justify-end">
                    <DashboardButton
                      icon={<LuPlay className="size-3.5" />}
                      onClick={() => void startJob(job.objectKey)}
                    >
                      Start Job
                    </DashboardButton>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function ClusterSettingsTab({ controller }: { controller: Controller }) {
  return (
    <InfoCard>
      <div className="space-y-2">
        <CopyableField label="Object Key" value={controller.objectKey} />
        {controller.cluster?.peerId && (
          <CopyableField label="Peer ID" value={controller.cluster.peerId} />
        )}
      </div>
    </InfoCard>
  )
}

export function ForgeClusterViewerView({
  controller,
}: {
  controller: Controller
}) {
  const tabs = useMemo<ForgeViewerTab[]>(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: <ClusterOverviewTab controller={controller} />,
      },
      {
        id: 'workers',
        label: 'Workers',
        content: <ClusterWorkersTab controller={controller} />,
      },
      {
        id: 'jobs',
        label: 'Jobs',
        content: <ClusterJobsTab controller={controller} />,
      },
      {
        id: 'settings',
        label: 'Settings',
        content: <ClusterSettingsTab controller={controller} />,
      },
    ],
    [controller],
  )
  return (
    <ForgeViewerShell
      stateKey={controller.objectKey}
      icon={<LuServer className="size-4" />}
      title={controller.cluster?.name || 'Cluster'}
      tabs={tabs}
      headerStatus={
        controller.snapshotError ? (
          <div
            className="text-destructive border-destructive/15 bg-destructive/5 border-b px-4 py-2 text-xs"
            role="alert"
          >
            {controller.snapshotError.message}
          </div>
        ) : undefined
      }
    />
  )
}
