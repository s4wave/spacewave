import type React from 'react'
import {
  LuBriefcase,
  LuGitBranch,
  LuList,
  LuListTodo,
  LuPlus,
} from 'react-icons/lu'

import { State as JobState } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { State as TaskState } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { ForgeEntityList } from '@s4wave/web/forge/ForgeEntityList.js'
import { ForgeViewerShell } from '@s4wave/web/forge/ForgeViewerShell.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import type { ForgeJobViewerController } from './useForgeJobViewerController.js'

type Controller = ForgeJobViewerController
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

function JobOverviewTab({ controller }: { controller: Controller }) {
  return (
    <div className="space-y-3">
      <InfoCard>
        <div className="space-y-2">
          <CopyableField label="Object Key" value={controller.objectKey} />
          {controller.job?.timestamp && (
            <CopyableField
              label="Created"
              value={controller.job.timestamp.toISOString()}
            />
          )}
        </div>
      </InfoCard>
      <InfoCard
        icon={<LuListTodo className="text-foreground-alt/60 size-3.5" />}
        title="Tasks"
      >
        <div className="text-foreground text-2xl font-semibold">
          {controller.tasksLoading
            ? '-'
            : `${controller.completeTaskCount}/${controller.tasks.length}`}
        </div>
        <div className="text-foreground-alt/50 mt-1 text-xs">
          {controller.progressPercent}% complete
        </div>
        <div className="bg-foreground/8 mt-3 h-1.5 w-full overflow-hidden rounded-full">
          <div
            className="bg-brand h-full transition-[width] duration-200"
            style={{ width: `${controller.progressPercent}%` }}
          />
        </div>
      </InfoCard>
    </div>
  )
}

function JobTaskList({ controller }: { controller: Controller }) {
  if (controller.tasksLoading || controller.decodedTasksLoading)
    return (
      <ForgeEntityList
        entities={controller.tasks}
        loading
        icon={<LuListTodo className="size-3 shrink-0" />}
        loadingLabel="Loading tasks..."
        emptyLabel="No tasks in job"
      />
    )
  if (controller.decodedTasks.length === 0)
    return (
      <ForgeEntityList
        entities={controller.tasks}
        loading={false}
        icon={<LuListTodo className="size-3 shrink-0" />}
        loadingLabel="Loading tasks..."
        emptyLabel="No tasks in job"
      />
    )
  return (
    <div className="space-y-2">
      {controller.decodedTasks.map((task) => (
        <div
          key={task.entity.objectKey}
          className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex items-center justify-between gap-3 rounded-lg border px-3.5 py-2.5 transition duration-150"
        >
          <div className="min-w-0">
            <div className="text-foreground truncate text-sm font-medium">
              {task.data.name || task.entity.objectKey}
            </div>
            <div className="text-foreground-alt/50 truncate text-xs">
              {task.entity.objectKey}
            </div>
          </div>
          <StateBadge
            state={task.data.taskState ?? 0}
            labels={taskStateLabels}
          />
        </div>
      ))}
    </div>
  )
}

function GraphMessage({
  icon,
  children,
  alert,
}: {
  icon: React.ReactNode
  children: React.ReactNode
  alert?: boolean
}) {
  return (
    <div
      className={cn(
        'rounded-lg border p-3.5 text-xs',
        alert
          ? 'border-destructive/20 bg-destructive/5 text-destructive'
          : 'border-foreground/6 bg-background-card/30 text-foreground-alt/40',
      )}
      role={alert ? 'alert' : undefined}
    >
      <div className="flex items-center gap-2 p-1">
        {icon}
        {children}
      </div>
    </div>
  )
}

function JobTaskGraph({ controller }: { controller: Controller }) {
  if (controller.taskEdgesError)
    return (
      <GraphMessage icon={<LuGitBranch className="size-3.5 shrink-0" />} alert>
        {controller.taskEdgesError.message}
      </GraphMessage>
    )
  if (controller.taskEdgesLoading && controller.taskEdges.length === 0)
    return (
      <GraphMessage icon={<LuGitBranch className="size-3.5 shrink-0" />}>
        Loading dependency graph…
      </GraphMessage>
    )
  if (controller.decodedTasks.length === 0)
    return (
      <GraphMessage icon={<LuListTodo className="size-3.5 shrink-0" />}>
        No tasks in job
      </GraphMessage>
    )
  if (controller.taskEdges.length === 0)
    return (
      <GraphMessage icon={<LuGitBranch className="size-3.5 shrink-0" />}>
        No task dependency edges defined yet
      </GraphMessage>
    )
  return (
    <div className="space-y-2">
      {controller.taskEdges.map((edge) => (
        <div
          key={`${edge.kind}:${edge.from}:${edge.to}`}
          className="border-foreground/6 bg-background-card/30 rounded-lg border px-3.5 py-2.5"
        >
          <div className="text-foreground flex items-center gap-2 text-sm font-medium">
            <LuGitBranch className="size-3.5" />
            <span>
              {controller.taskByKey.get(edge.from)?.data.name || edge.from}
            </span>
            <span className="text-foreground-alt/50 text-xs">-&gt;</span>
            <span>
              {controller.taskByKey.get(edge.to)?.data.name || edge.to}
            </span>
          </div>
          <div className="text-foreground-alt/50 mt-1 text-[0.6rem] tracking-widest uppercase">
            {edge.kind}
          </div>
        </div>
      ))}
    </div>
  )
}

function JobTasksTab({ controller }: { controller: Controller }) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="bg-foreground/5 inline-flex gap-1 rounded-md p-1">
          <button
            type="button"
            onClick={() => controller.setTasksView('list')}
            className={cn(
              'flex items-center gap-1 rounded border px-2.5 py-1 text-xs font-medium transition-all duration-150 select-none',
              controller.tasksView === 'list'
                ? 'border-brand/30 bg-brand/10 text-foreground'
                : 'text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5 border-transparent',
            )}
          >
            <LuList className="size-3.5" />
            List
          </button>
          <button
            type="button"
            onClick={() => controller.setTasksView('dag')}
            className={cn(
              'flex items-center gap-1 rounded border px-2.5 py-1 text-xs font-medium transition-all duration-150 select-none',
              controller.tasksView === 'dag'
                ? 'border-brand/30 bg-brand/10 text-foreground'
                : 'text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5 border-transparent',
            )}
          >
            <LuGitBranch className="size-3.5" />
            DAG
          </button>
        </div>
        {controller.canCreateTask && (
          <DashboardButton
            icon={<LuPlus className="size-3.5" />}
            onClick={() => void controller.addTask()}
            disabled={controller.creatingTask}
          >
            {controller.creatingTask ? 'Adding…' : 'Add Task'}
          </DashboardButton>
        )}
      </div>
      {controller.tasksView === 'list' ? (
        <JobTaskList controller={controller} />
      ) : (
        <JobTaskGraph controller={controller} />
      )}
    </div>
  )
}

export function ForgeJobViewerView({ controller }: { controller: Controller }) {
  const tabs = [
    {
      id: 'overview',
      label: 'Overview',
      content: <JobOverviewTab controller={controller} />,
    },
    {
      id: 'tasks',
      label: 'Tasks',
      content: <JobTasksTab controller={controller} />,
    },
  ]
  return (
    <ForgeViewerShell
      stateKey={controller.objectKey}
      icon={<LuBriefcase className="size-4" />}
      title="Job"
      state={controller.jobState}
      stateLabels={jobStateLabels}
      tabs={tabs}
      headerStatus={
        controller.taskEdgesError ? (
          <div
            className="text-destructive border-destructive/15 bg-destructive/5 border-b px-4 py-2 text-xs"
            role="alert"
          >
            {controller.taskEdgesError.message}
          </div>
        ) : undefined
      }
    />
  )
}
