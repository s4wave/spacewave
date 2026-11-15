/* eslint-disable react-doctor/no-giant-component */
import { memo, useCallback, useMemo, useState } from 'react'
import {
  LuBriefcase,
  LuGitBranch,
  LuList,
  LuListTodo,
  LuPlus,
} from 'react-icons/lu'

import { Job, State } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import {
  Task,
  State as TaskState,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlock } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  forgeBlockStatus,
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { PRED_JOB_TO_TASK } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { cn } from '@s4wave/web/style/utils.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { messageDecoder } from '@s4wave/sdk/world/entity-set.js'
import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import { entitySetRows, useEntitySet } from '@s4wave/web/hooks/useEntitySet.js'
import { ForgeSetList, type ForgeSetRow } from './ForgeSetList.js'
import { useForgeTaskDependencyGraph } from './useForgeTaskDependencyGraph.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'

export const ForgeJobTypeID = 'forge/job'

const jobStateLabels: Record<number, string> = {
  [State.JobState_UNKNOWN]: 'UNKNOWN',
  [State.JobState_PENDING]: 'PENDING',
  [State.JobState_RUNNING]: 'RUNNING',
  [State.JobState_COMPLETE]: 'COMPLETE',
}

const taskStateLabels: Record<number, string> = {
  [TaskState.TaskState_UNKNOWN]: 'UNKNOWN',
  [TaskState.TaskState_PENDING]: 'PENDING',
  [TaskState.TaskState_RUNNING]: 'RUNNING',
  [TaskState.TaskState_CHECKING]: 'CHECKING',
  [TaskState.TaskState_COMPLETE]: 'COMPLETE',
  [TaskState.TaskState_RETRY]: 'RETRY',
}

const taskDecoder = messageDecoder(Task)

const taskIcon = <LuListTodo className="size-3 shrink-0" />

// TaskRow draws one task; it draws again only when that task's body changes.
const TaskRow = memo(function TaskRow({ objectKey, value }: ForgeSetRow<Task>) {
  return (
    <div className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex items-center justify-between gap-3 rounded-lg border px-3.5 py-2.5 transition duration-150">
      <div className="min-w-0">
        <div className="text-foreground truncate text-sm font-medium">
          {value.name || objectKey}
        </div>
        <div className="text-foreground-alt/50 truncate text-xs">
          {objectKey}
        </div>
      </div>
      <StateBadge state={value.taskState ?? 0} labels={taskStateLabels} />
    </div>
  )
})

// renderTaskRow draws a task row keyed by its object.
function renderTaskRow(row: ForgeSetRow<Task>) {
  return <TaskRow key={row.objectKey} {...row} />
}

/** ForgeJobViewer displays a Forge Job entity with tabbed layout. */
export function ForgeJobViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const block = useForgeBlock(objectState, ForgeJobTypeID, Job)
  const job = block.value ?? undefined
  const { spaceWorld, navigateToObjects } = SpaceContainerContext.useContext()
  const [creatingTask, setCreatingTask] = useState(false)
  const [tasksView, setTasksView] = useState<'list' | 'dag'>('list')
  const visibleWizardTypeSet = useVisibleObjectWizardTypeSet()
  const canCreateTask = visibleWizardTypeSet.has('forge/task')

  const tasks = useEntitySet(
    worldState,
    {
      case: 'link',
      origin: objectKey,
      predicate: PRED_JOB_TO_TASK,
      direction: 'out',
    },
    taskDecoder,
  )
  const taskSet = resourceContent(tasks)
  const decodedTasks = useMemo(() => entitySetRows(taskSet), [taskSet])
  const taskCount = taskSet?.keys.length ?? 0
  const { edges: taskEdges, loading: taskEdgesLoading } =
    useForgeTaskDependencyGraph(worldState, objectKey)

  const completeTaskCount = useMemo(
    () =>
      decodedTasks.filter(
        (task) =>
          (task.value.taskState ?? TaskState.TaskState_UNKNOWN) ===
          TaskState.TaskState_COMPLETE,
      ).length,
    [decodedTasks],
  )
  const progressPercent =
    taskCount === 0 ? 0 : Math.round((completeTaskCount / taskCount) * 100)

  const handleAddTask = useCallback(async () => {
    setCreatingTask(true)
    try {
      const wizardKey = await buildWizardObjectKey(spaceWorld, 'Task')
      const configData = ForgeTaskCreateOp.toBinary({
        taskKey: '',
        name: '',
        jobKey: objectKey,
        timestamp: new Date(),
      })
      const opData = CreateWizardObjectOp.toBinary({
        objectKey: wizardKey,
        wizardTypeId: 'wizard/forge/task',
        targetTypeId: 'forge/task',
        targetKeyPrefix: 'forge/task/',
        name: 'Task',
        timestamp: new Date(),
        initialStep: 1,
        initialConfigData: configData,
      })
      await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
      navigateToObjects([wizardKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open task wizard',
      )
    } finally {
      setCreatingTask(false)
    }
  }, [spaceWorld, navigateToObjects, objectKey])

  const tasksContent = useMemo(() => {
    if (tasksView === 'list') {
      return (
        <ForgeSetList
          set={tasks}
          what="the tasks"
          icon={taskIcon}
          loadingLabel="Loading tasks..."
          emptyLabel="No tasks in job"
          renderRow={renderTaskRow}
        />
      )
    }

    if (taskEdgesLoading && taskEdges.length === 0) {
      return (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
            <LuGitBranch className="size-3.5 shrink-0" />
            <span>Loading dependency graph…</span>
          </div>
        </div>
      )
    }
    if (taskSet?.complete && decodedTasks.length === 0) {
      return (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
            <LuListTodo className="size-3.5 shrink-0" />
            <span>No tasks in job</span>
          </div>
        </div>
      )
    }
    if (taskEdges.length === 0) {
      return (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
            <LuGitBranch className="size-3.5 shrink-0" />
            <span>No task dependency edges defined yet</span>
          </div>
        </div>
      )
    }

    return (
      <div className="space-y-2">
        {taskEdges.map((edge) => {
          const fromTask = taskSet?.rows.get(edge.from)?.value
          const toTask = taskSet?.rows.get(edge.to)?.value
          return (
            <div
              key={`${edge.kind}:${edge.from}:${edge.to}`}
              className="border-foreground/6 bg-background-card/30 rounded-lg border px-3.5 py-2.5"
            >
              <div className="text-foreground flex items-center gap-2 text-sm font-medium">
                <LuGitBranch className="size-3.5" />
                <span>{fromTask?.name || edge.from}</span>
                <span className="text-foreground-alt/50 text-xs">-&gt;</span>
                <span>{toTask?.name || edge.to}</span>
              </div>
              <div className="text-foreground-alt/50 micro-text mt-1 tracking-widest uppercase">
                {edge.kind}
              </div>
            </div>
          )
        })}
      </div>
    )
  }, [decodedTasks, taskEdges, taskEdgesLoading, taskSet, tasks, tasksView])

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            <InfoCard
              icon={
                <LuListTodo
                  aria-hidden
                  className="text-foreground-alt/60 size-3.5"
                />
              }
              title="Tasks"
            >
              <div className="text-foreground text-2xl font-semibold">
                {taskSet ? `${completeTaskCount}/${taskCount}` : '-'}
              </div>
              <div className="text-foreground-alt/50 mt-1 text-xs">
                {progressPercent}% complete
              </div>
              <div className="bg-foreground/8 mt-3 h-1.5 w-full overflow-hidden rounded-full">
                <div
                  className="bg-brand forge-progress-width progress-width-transition h-full"
                  style={{ '--forge-progress-width': `${progressPercent}%` }}
                />
              </div>
            </InfoCard>
          </div>
        ),
      },
      {
        id: 'tasks',
        label: 'Tasks',
        content: (
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-2">
              <div className="bg-foreground/5 inline-flex gap-1 rounded-md p-1">
                <button
                  type="button"
                  onClick={() => setTasksView('list')}
                  className={cn(
                    'flex items-center gap-1 rounded border px-2.5 py-1 text-xs font-medium transition-all duration-150 select-none',
                    tasksView === 'list'
                      ? 'border-brand/30 bg-brand/10 text-foreground'
                      : 'text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5 border-transparent',
                  )}
                >
                  <LuList className="size-3.5" />
                  List
                </button>
                <button
                  type="button"
                  onClick={() => setTasksView('dag')}
                  className={cn(
                    'flex items-center gap-1 rounded border px-2.5 py-1 text-xs font-medium transition-all duration-150 select-none',
                    tasksView === 'dag'
                      ? 'border-brand/30 bg-brand/10 text-foreground'
                      : 'text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5 border-transparent',
                  )}
                >
                  <LuGitBranch className="size-3.5" />
                  DAG
                </button>
              </div>
              {canCreateTask && (
                <DashboardButton
                  icon={<LuPlus className="size-3.5" />}
                  onClick={() => {
                    void handleAddTask()
                  }}
                  disabled={creatingTask}
                >
                  {creatingTask ? 'Adding…' : 'Add Task'}
                </DashboardButton>
              )}
            </div>
            {tasksContent}
          </div>
        ),
      },
    ],
    [
      canCreateTask,
      completeTaskCount,
      creatingTask,
      handleAddTask,
      progressPercent,
      taskCount,
      taskSet,
      tasksContent,
      tasksView,
    ],
  )

  // Details lists the job's identity behind the bar.
  const facts = (
    <>
      <FactRow label="Object Key" value={objectKey} />
      {job?.timestamp && (
        <FactRow
          kind="text"
          label="Created"
          value={job.timestamp.toISOString()}
        />
      )}
    </>
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuBriefcase className="size-4" />}
      title="Job"
      state={job?.jobState}
      details={{ label: 'Job', facts }}
      status={forgeBlockStatus('the job', block)}
      stateLabels={jobStateLabels}
      tabs={tabs}
    />
  )
}
