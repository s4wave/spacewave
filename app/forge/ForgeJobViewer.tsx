import { useCallback, useMemo, useState } from 'react'
import { LuBriefcase, LuGitBranch, LuListTodo, LuPlus } from 'react-icons/lu'

import { Job, State } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import {
  Task,
  State as TaskState,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { useForgeLinkedEntities } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import {
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { ForgeEntityList } from '@s4wave/web/forge/ForgeEntityList.js'
import { PRED_JOB_TO_TASK } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { Button } from '@s4wave/web/ui/button.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'
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

// ForgeJobViewer displays a Forge Job entity with tabbed layout.
export function ForgeJobViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const job = useForgeBlockData(objectState, Job)
  const { spaceWorld, navigateToObjects } = SpaceContainerContext.useContext()
  const [creatingTask, setCreatingTask] = useState(false)
  const [tasksView, setTasksView] = useState<'list' | 'dag'>('list')
  const visibleWizardTypeSet = useVisibleObjectWizardTypeSet()
  const canCreateTask = visibleWizardTypeSet.has('forge/task')

  const { entities: tasks, loading: tasksLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_JOB_TO_TASK,
  )
  const { items: decodedTasks, loading: decodedTasksLoading } =
    useForgeDecodedLinkedEntities(worldState, tasks, Task)
  const { edges: taskEdges, loading: taskEdgesLoading } =
    useForgeTaskDependencyGraph(worldState, tasks)
  const taskByKey = useMemo(
    () =>
      new Map(
        decodedTasks.map((task) => [task.entity.objectKey, task] as const),
      ),
    [decodedTasks],
  )

  const completeTaskCount = useMemo(
    () =>
      decodedTasks.filter(
        (task) =>
          (task.data.taskState ?? TaskState.TaskState_UNKNOWN) ===
          TaskState.TaskState_COMPLETE,
      ).length,
    [decodedTasks],
  )
  const progressPercent = useMemo(() => {
    if (tasks.length === 0) return 0
    return Math.round((completeTaskCount / tasks.length) * 100)
  }, [completeTaskCount, tasks.length])

  const handleAddTask = useCallback(async () => {
    setCreatingTask(true)
    try {
      const suffix = Date.now().toString(36)
      const wizardKey = `wizard/forge/task/${suffix}`
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
      if (tasksLoading || decodedTasksLoading) {
        return (
          <ForgeEntityList
            entities={tasks}
            loading
            icon={
              <LuListTodo className="text-muted-foreground h-3 w-3 shrink-0" />
            }
            loadingLabel="Loading tasks..."
            emptyLabel="No tasks in job"
          />
        )
      }
      if (decodedTasks.length === 0) {
        return (
          <ForgeEntityList
            entities={tasks}
            loading={tasksLoading}
            icon={
              <LuListTodo className="text-muted-foreground h-3 w-3 shrink-0" />
            }
            loadingLabel="Loading tasks..."
            emptyLabel="No tasks in job"
          />
        )
      }

      return (
        <div className="space-y-2">
          {decodedTasks.map((task) => (
            <div
              key={task.entity.objectKey}
              className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex items-center justify-between gap-3 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
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

    if (taskEdgesLoading && taskEdges.length === 0) {
      return (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
            <LuGitBranch className="h-3.5 w-3.5 shrink-0" />
            <span>Loading dependency graph...</span>
          </div>
        </div>
      )
    }
    if (decodedTasks.length === 0) {
      return (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
            <LuListTodo className="h-3.5 w-3.5 shrink-0" />
            <span>No tasks in job</span>
          </div>
        </div>
      )
    }
    if (taskEdges.length === 0) {
      return (
        <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
          <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
            <LuGitBranch className="h-3.5 w-3.5 shrink-0" />
            <span>No task dependency edges defined yet</span>
          </div>
        </div>
      )
    }

    return (
      <div className="space-y-2">
        {taskEdges.map((edge) => {
          const fromTask = taskByKey.get(edge.from)
          const toTask = taskByKey.get(edge.to)
          return (
            <div
              key={`${edge.kind}:${edge.from}:${edge.to}`}
              className="border-foreground/6 bg-background-card/30 rounded-lg border px-3.5 py-2.5"
            >
              <div className="text-foreground flex items-center gap-2 text-sm font-medium">
                <LuGitBranch className="h-3.5 w-3.5" />
                <span>{fromTask?.data.name || edge.from}</span>
                <span className="text-foreground-alt/50 text-xs">-&gt;</span>
                <span>{toTask?.data.name || edge.to}</span>
              </div>
              <div className="text-foreground-alt/50 mt-1 text-[0.6rem] tracking-widest uppercase">
                {edge.kind}
              </div>
            </div>
          )
        })}
      </div>
    )
  }, [
    decodedTasks,
    decodedTasksLoading,
    taskByKey,
    taskEdges,
    taskEdgesLoading,
    tasks,
    tasksLoading,
    tasksView,
  ])

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            <InfoCard>
              <div className="space-y-2">
                <CopyableField label="Object Key" value={objectKey} />
                {job?.timestamp && (
                  <CopyableField
                    label="Created"
                    value={job.timestamp.toISOString()}
                  />
                )}
              </div>
            </InfoCard>
            <InfoCard
              icon={
                <LuListTodo className="text-foreground-alt/60 h-3.5 w-3.5" />
              }
              title="Tasks"
            >
              <div className="text-foreground text-2xl font-semibold">
                {tasksLoading ? '-' : `${completeTaskCount}/${tasks.length}`}
              </div>
              <div className="text-foreground-alt/50 mt-1 text-xs">
                {progressPercent}% complete
              </div>
              <div className="bg-foreground/8 mt-3 h-1.5 w-full overflow-hidden rounded-full">
                <div
                  className="bg-brand h-full transition-all duration-200"
                  style={{ width: `${progressPercent}%` }}
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
              <div className="flex gap-2">
                <Button
                  variant={tasksView === 'list' ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setTasksView('list')}
                >
                  List
                </Button>
                <Button
                  variant={tasksView === 'dag' ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setTasksView('dag')}
                >
                  DAG
                </Button>
              </div>
              {canCreateTask && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    void handleAddTask()
                  }}
                  disabled={creatingTask}
                >
                  <LuPlus className="h-3.5 w-3.5" />
                  {creatingTask ? 'Adding...' : 'Add Task'}
                </Button>
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
      job,
      objectKey,
      progressPercent,
      tasks.length,
      tasksContent,
      tasksLoading,
      tasksView,
    ],
  )

  return (
    <ForgeViewerShell
      icon={<LuBriefcase className="h-4 w-4" />}
      title="Job"
      state={job?.jobState ?? 0}
      stateLabels={jobStateLabels}
      tabs={tabs}
    />
  )
}
