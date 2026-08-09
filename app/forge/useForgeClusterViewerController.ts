import { useCallback, useMemo, useState } from 'react'

import {
  Cluster,
  ClusterStartJobOp,
} from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { State as TaskState } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { useForgeLinkedEntities } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import {
  PRED_CLUSTER_TO_JOB,
  PRED_CLUSTER_TO_WORKER,
} from '@s4wave/web/forge/predicates.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'
import { useForgeClusterSnapshot } from './useForgeClusterSnapshot.js'

export const ForgeClusterTypeID = 'forge/cluster'

export function useForgeClusterViewerController({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const cluster = useForgeBlockData(objectState, ForgeClusterTypeID, Cluster)
  const { spaceState, spaceWorld, navigateToObjects } =
    SpaceContainerContext.useContext()
  const [creatingJob, setCreatingJob] = useState(false)
  const canCreateJob = useVisibleObjectWizardTypeSet().has('forge/job')
  const { entities: workers, loading: workersLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_CLUSTER_TO_WORKER,
  )
  const { entities: jobs, loading: jobsLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_CLUSTER_TO_JOB,
  )
  const {
    snapshot,
    loading: snapshotLoading,
    error: snapshotError,
  } = useForgeClusterSnapshot(worldState, [objectKey])

  const taskStateCounts = useMemo(() => {
    const counts: Record<number, number> = {}
    for (const task of snapshot.tasks) {
      const state = task.data.taskState ?? TaskState.TaskState_UNKNOWN
      counts[state] = (counts[state] ?? 0) + 1
    }
    return counts
  }, [snapshot.tasks])
  const workerExecutionCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const worker of snapshot.workers) {
      const peerIds = new Set(worker.peerIds)
      counts.set(
        worker.objectKey,
        snapshot.executions.filter((execution) =>
          peerIds.has(execution.data.peerId ?? ''),
        ).length,
      )
    }
    return counts
  }, [snapshot.executions, snapshot.workers])
  const tasksByJobKey = useMemo(() => {
    const map = new Map<string, typeof snapshot.tasks>()
    for (const task of snapshot.tasks)
      map.set(task.jobKey, [...(map.get(task.jobKey) ?? []), task])
    return map
  }, [snapshot.tasks])
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map(
        (object) => object.objectKey ?? '',
      ) ?? [],
    [spaceState.worldContents?.objects],
  )

  const createJob = useCallback(async () => {
    setCreatingJob(true)
    try {
      const wizardKey = buildWizardObjectKey('Job', existingObjectKeys)
      const initialConfigData = ForgeJobCreateOp.toBinary({
        jobKey: '',
        clusterKey: objectKey,
        taskDefs: [],
        timestamp: new Date(),
      })
      const opData = CreateWizardObjectOp.toBinary({
        objectKey: wizardKey,
        wizardTypeId: 'wizard/forge/job',
        targetTypeId: 'forge/job',
        targetKeyPrefix: 'forge/job/',
        name: 'Job',
        timestamp: new Date(),
        initialStep: 1,
        initialConfigData,
      })
      await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
      navigateToObjects([wizardKey])
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : 'Failed to open job wizard',
      )
    } finally {
      setCreatingJob(false)
    }
  }, [existingObjectKeys, navigateToObjects, objectKey, spaceWorld])
  const startJob = useCallback(
    async (jobKey: string) => {
      try {
        const opData = ClusterStartJobOp.toBinary({
          clusterKey: objectKey,
          jobKey,
        })
        await spaceWorld.applyWorldOp('forge/cluster/start-job', opData, '')
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : 'Failed to start job',
        )
      }
    },
    [objectKey, spaceWorld],
  )

  return {
    objectKey,
    cluster,
    workers,
    workersLoading,
    jobs,
    jobsLoading,
    snapshot,
    snapshotLoading,
    snapshotError,
    taskStateCounts,
    workerExecutionCounts,
    tasksByJobKey,
    canCreateJob,
    creatingJob,
    createJob,
    startJob,
  }
}

export type ForgeClusterViewerController = ReturnType<
  typeof useForgeClusterViewerController
>
