import { useCallback, useMemo, useState } from 'react'
import {
  LuServer,
  LuCpu,
  LuBriefcase,
  LuListTodo,
  LuPlay,
  LuPlus,
} from 'react-icons/lu'

import {
  Cluster,
  ClusterStartJobOp,
} from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { State as JobState } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { State as TaskState } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { useForgeLinkedEntities } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import {
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { ForgeEntityList } from '@s4wave/web/forge/ForgeEntityList.js'
import { ForgeEntityLink } from '@s4wave/web/forge/ForgeEntityLink.js'
import {
  PRED_CLUSTER_TO_JOB,
  PRED_CLUSTER_TO_WORKER,
} from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { StatCard } from '@s4wave/web/ui/StatCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { Button } from '@s4wave/web/ui/button.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { useForgeClusterSnapshot } from './useForgeClusterSnapshot.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'

export const ForgeClusterTypeID = 'forge/cluster'

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

// ForgeClusterViewer displays a Forge Cluster entity with tabbed layout.
export function ForgeClusterViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const cluster = useForgeBlockData(objectState, Cluster)
  const { spaceState, spaceWorld, navigateToObjects } =
    SpaceContainerContext.useContext()
  const [creatingJob, setCreatingJob] = useState(false)
  const visibleWizardTypeSet = useVisibleObjectWizardTypeSet()
  const canCreateJob = visibleWizardTypeSet.has('forge/job')

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
  const { snapshot, loading: snapshotLoading } = useForgeClusterSnapshot(
    worldState,
    [objectKey],
  )
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
      const count = snapshot.executions.filter((execution) =>
        worker.peerIds.includes(execution.data.peerId ?? ''),
      ).length
      counts.set(worker.objectKey, count)
    }
    return counts
  }, [snapshot.executions, snapshot.workers])
  const snapshotTasks = snapshot.tasks
  const tasksByJobKey = useMemo(() => {
    const map = new Map<string, typeof snapshotTasks>()
    for (const task of snapshotTasks) {
      const prev = map.get(task.jobKey) ?? []
      prev.push(task)
      map.set(task.jobKey, prev)
    }
    return map
  }, [snapshotTasks])
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map((obj) => obj.objectKey ?? '') ??
      [],
    [spaceState.worldContents?.objects],
  )

  const handleCreateJob = useCallback(async () => {
    setCreatingJob(true)
    try {
      const wizardKey = buildWizardObjectKey('Job', existingObjectKeys)
      const configData = ForgeJobCreateOp.toBinary({
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
        initialConfigData: configData,
      })
      await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
      navigateToObjects([wizardKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open job wizard',
      )
    } finally {
      setCreatingJob(false)
    }
  }, [existingObjectKeys, spaceWorld, navigateToObjects, objectKey])
  const handleStartJob = useCallback(
    async (jobKey: string) => {
      try {
        const opData = ClusterStartJobOp.toBinary({
          clusterKey: objectKey,
          jobKey,
        })
        await spaceWorld.applyWorldOp('forge/cluster/start-job', opData, '')
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to start job')
      }
    },
    [objectKey, spaceWorld],
  )

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            <InfoCard>
              <div className="space-y-2">
                {cluster?.name && (
                  <CopyableField label="Name" value={cluster.name} />
                )}
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
              icon={
                <LuListTodo className="text-foreground-alt/60 h-3.5 w-3.5" />
              }
              title="Task States"
            >
              {snapshotLoading && (
                <div className="text-foreground-alt/50 text-xs">
                  Loading task breakdown...
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
        ),
      },
      {
        id: 'workers',
        label: 'Workers',
        content:
          snapshot.workers.length === 0 ?
            <ForgeEntityList
              entities={workers}
              loading={workersLoading || snapshotLoading}
              icon={
                <LuCpu className="text-muted-foreground h-3 w-3 shrink-0" />
              }
              loadingLabel="Loading workers..."
              emptyLabel="No workers assigned"
            />
          : <div className="space-y-2">
              {snapshot.workers.map((worker) => (
                <div
                  key={worker.objectKey}
                  className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
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
            </div>,
      },
      {
        id: 'jobs',
        label: 'Jobs',
        content: (
          <div className="space-y-3">
            {canCreateJob && (
              <div className="flex justify-end">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    void handleCreateJob()
                  }}
                  disabled={creatingJob}
                >
                  <LuPlus className="h-3.5 w-3.5" />
                  {creatingJob ? 'Creating...' : 'Create Job'}
                </Button>
              </div>
            )}
            {snapshot.jobs.length === 0 ?
              <ForgeEntityList
                entities={jobs}
                loading={jobsLoading || snapshotLoading}
                icon={
                  <LuBriefcase className="text-muted-foreground h-3 w-3 shrink-0" />
                }
                loadingLabel="Loading jobs..."
                emptyLabel="No jobs in cluster"
              />
            : <div className="space-y-2">
                {snapshot.jobs.map((job) => {
                  const jobTasks = tasksByJobKey.get(job.objectKey) ?? []
                  const completeTasks = jobTasks.filter(
                    (task) =>
                      (task.data.taskState ?? TaskState.TaskState_UNKNOWN) ===
                      TaskState.TaskState_COMPLETE,
                  ).length
                  const progressPercent =
                    jobTasks.length === 0 ?
                      0
                    : Math.round((completeTasks / jobTasks.length) * 100)
                  const startable =
                    (job.data.jobState ?? JobState.JobState_UNKNOWN) ===
                    JobState.JobState_PENDING
                  return (
                    <div
                      key={job.objectKey}
                      className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-3 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
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
                          className="bg-brand h-full transition-all duration-200"
                          style={{ width: `${progressPercent}%` }}
                        />
                      </div>
                      {startable && (
                        <div className="flex justify-end">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              void handleStartJob(job.objectKey)
                            }}
                          >
                            <LuPlay className="h-3.5 w-3.5" />
                            Start Job
                          </Button>
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            }
          </div>
        ),
      },
      {
        id: 'settings',
        label: 'Settings',
        content: (
          <InfoCard>
            <div className="space-y-2">
              <CopyableField label="Object Key" value={objectKey} />
              {cluster?.peerId && (
                <CopyableField label="Peer ID" value={cluster.peerId} />
              )}
            </div>
          </InfoCard>
        ),
      },
    ],
    [
      cluster,
      canCreateJob,
      creatingJob,
      handleCreateJob,
      handleStartJob,
      jobs,
      jobsLoading,
      objectKey,
      snapshot.jobs,
      snapshot.tasks,
      snapshot.workers,
      snapshotLoading,
      taskStateCounts,
      tasksByJobKey,
      workerExecutionCounts,
      workers,
      workersLoading,
    ],
  )

  return (
    <ForgeViewerShell
      icon={<LuServer className="h-4 w-4" />}
      title={cluster?.name || 'Cluster'}
      tabs={tabs}
    />
  )
}
