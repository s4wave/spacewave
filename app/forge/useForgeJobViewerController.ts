import { useCallback, useMemo, useState } from 'react'

import { Job, State } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import {
  Task,
  State as TaskState,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { useForgeLinkedEntities } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import { PRED_JOB_TO_TASK } from '@s4wave/web/forge/predicates.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'
import { useForgeTaskDependencyGraph } from './useForgeTaskDependencyGraph.js'

export const ForgeJobTypeID = 'forge/job'

export function useForgeJobViewerController({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const job = useForgeBlockData(objectState, ForgeJobTypeID, Job)
  const { spaceState, spaceWorld, navigateToObjects } =
    SpaceContainerContext.useContext()
  const [creatingTask, setCreatingTask] = useState(false)
  const [tasksView, setTasksView] = useState<'list' | 'dag'>('list')
  const canCreateTask = useVisibleObjectWizardTypeSet().has('forge/task')
  const { entities: tasks, loading: tasksLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_JOB_TO_TASK,
  )
  const { items: decodedTasks, loading: decodedTasksLoading } =
    useForgeDecodedLinkedEntities(worldState, tasks, Task)
  const {
    edges: taskEdges,
    loading: taskEdgesLoading,
    error: taskEdgesError,
  } = useForgeTaskDependencyGraph(worldState, tasks)
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
  const progressPercent =
    tasks.length === 0
      ? 0
      : Math.round((completeTaskCount / tasks.length) * 100)
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map(
        (object) => object.objectKey ?? '',
      ) ?? [],
    [spaceState.worldContents?.objects],
  )

  const addTask = useCallback(async () => {
    setCreatingTask(true)
    try {
      const wizardKey = buildWizardObjectKey('Task', existingObjectKeys)
      const initialConfigData = ForgeTaskCreateOp.toBinary({
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
        initialConfigData,
      })
      await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
      navigateToObjects([wizardKey])
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : 'Failed to open task wizard',
      )
    } finally {
      setCreatingTask(false)
    }
  }, [existingObjectKeys, navigateToObjects, objectKey, spaceWorld])

  return {
    objectKey,
    job,
    tasks,
    tasksLoading,
    decodedTasks,
    decodedTasksLoading,
    taskEdges,
    taskEdgesLoading,
    taskEdgesError,
    taskByKey,
    completeTaskCount,
    progressPercent,
    canCreateTask,
    creatingTask,
    tasksView,
    setTasksView,
    addTask,
    jobState: job?.jobState ?? State.JobState_UNKNOWN,
  }
}

export type ForgeJobViewerController = ReturnType<
  typeof useForgeJobViewerController
>
