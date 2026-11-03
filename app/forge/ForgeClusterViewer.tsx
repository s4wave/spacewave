/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuServer,
  LuCpu,
  LuBriefcase,
  LuListTodo,
  LuPlay,
  LuPlus,
} from '@s4wave/web/images/icons.js'

import {
  Cluster,
  ClusterStartJobOp,
} from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { State as JobState } from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { State as TaskState } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
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
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { useForgeClusterSnapshot } from './useForgeClusterSnapshot.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

import { motion as themeMotion } from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
  },
  rule5: {
    marginTop: themeLayout['--spacing'],
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule8: { display: 'flex', justifyContent: 'flex-end' },
  rule9: { minWidth: 0 },
  rule10: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule11: {
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule12: {
    transition: 'width 200ms',
    width: 'var(--forge-progress-width)',
    height: '100%',
    backgroundColor: themeColors['--color-brand'],
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    minWidth: 0,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule2: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
  const cluster = useForgeBlockData(objectState, ForgeClusterTypeID, Cluster)
  useObjectName(cluster?.name)
  const { spaceWorld, navigateToObjects } = SpaceContainerContext.useContext()
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
  const {
    snapshot,
    loading: snapshotLoading,
    error: snapshotError,
    retry: retrySnapshot,
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
      const count = snapshot.executions.filter((execution) =>
        peerIds.has(execution.data.peerId ?? ''),
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

  const handleCreateJob = useCallback(async () => {
    setCreatingJob(true)
    try {
      const wizardKey = await buildWizardObjectKey(spaceWorld, 'Job')
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
  }, [spaceWorld, navigateToObjects, objectKey])
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
          <div sx={false}>
            {(cluster?.name || cluster?.peerId) && (
              <InfoCard sx={nativeChildSpacing.s3} title="Cluster identity">
                <div sx={false}>
                  {cluster.name && (
                    <CopyableField
                      sx={nativeChildSpacing.s2}
                      label="Name"
                      value={cluster.name}
                    />
                  )}
                  {cluster.peerId && (
                    <CopyableField
                      sx={nativeChildSpacing.s2}
                      label="Peer ID"
                      value={cluster.peerId}
                    />
                  )}
                </div>
              </InfoCard>
            )}
            <FigureStrip
              sx={nativeChildSpacing.s3}
              figures={[
                {
                  label: 'Workers',
                  value: workersLoading ? '-' : workers.length,
                },
                { label: 'Jobs', value: jobsLoading ? '-' : jobs.length },
              ]}
            />
            <InfoCard
              sx={nativeChildSpacing.s3}
              icon={<LuListTodo sx={stylexStyles.rule1} />}
              title="Task States"
            >
              {snapshotError ? (
                <LoadingCard
                  view={{
                    state: 'error',
                    title: 'Task states unavailable',
                    detail: 'Forge could not read this Cluster snapshot.',
                    error: snapshotError.message,
                    onRetry: retrySnapshot,
                  }}
                />
              ) : snapshotLoading ? (
                <div sx={stylexStyles3.rule1}>Loading task breakdown…</div>
              ) : snapshot.tasks.length === 0 ? (
                <div sx={stylexStyles3.rule1}>No tasks assigned yet</div>
              ) : (
                <div sx={stylexStyles3.rule2}>
                  {Object.entries(taskStateLabels).map(([state, label]) => (
                    <div key={state} sx={stylexStyles3.rule3}>
                      <div sx={stylexStyles3.rule4}>{label}</div>
                      <div sx={stylexStyles3.rule5}>
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
        content: snapshotError ? (
          <LoadingCard
            view={{
              state: 'error',
              title: 'Workers unavailable',
              detail: 'Forge could not read this Cluster snapshot.',
              error: snapshotError.message,
              onRetry: retrySnapshot,
            }}
          />
        ) : snapshot.workers.length === 0 ? (
          <ForgeEntityList
            entities={workers}
            loading={workersLoading || snapshotLoading}
            icon={<LuCpu sx={stylexStyles.rule2} />}
            loadingLabel="Loading workers..."
            emptyLabel="No workers assigned"
          />
        ) : (
          <div sx={false}>
            {snapshot.workers.map((worker) => (
              <div
                key={worker.objectKey}
                sx={[nativeChildSpacingNext.s2, stylexStyles4.rule1]}
              >
                <div sx={[nativeChildSpacing.s2, stylexStyles3.rule6]}>
                  <ForgeEntityLink
                    objectKey={worker.objectKey}
                    sx={stylexStyles2.rule1}
                  >
                    {worker.data.name || worker.objectKey}
                  </ForgeEntityLink>
                  <div sx={stylexStyles3.rule1}>
                    {workerExecutionCounts.get(worker.objectKey) ?? 0} active
                  </div>
                </div>
                <div sx={[nativeChildSpacing.s2, stylexStyles3.rule7]}>
                  <span>{worker.peerIds.length} peer IDs</span>
                  <span>{worker.clusterKeys.length} cluster links</span>
                </div>
              </div>
            ))}
          </div>
        ),
      },
      {
        id: 'jobs',
        label: 'Jobs',
        content: (
          <div sx={false}>
            {canCreateJob && (
              <div sx={[nativeChildSpacingNextNext.s3, stylexStyles3.rule8]}>
                <DashboardButton
                  icon={<LuPlus sx={stylexStyles.rule3} />}
                  onClick={() => {
                    void handleCreateJob()
                  }}
                  disabled={creatingJob}
                >
                  {creatingJob ? 'Creating…' : 'Create Job'}
                </DashboardButton>
              </div>
            )}
            {snapshotError ? (
              <LoadingCard
                sx={nativeChildSpacingNextNext.s3}
                view={{
                  state: 'error',
                  title: 'Jobs unavailable',
                  detail: 'Forge could not read this Cluster snapshot.',
                  error: snapshotError.message,
                  onRetry: retrySnapshot,
                }}
              />
            ) : snapshot.jobs.length === 0 ? (
              <ForgeEntityList
                sx={nativeChildSpacingNextNext.s3}
                entities={jobs}
                loading={jobsLoading || snapshotLoading}
                icon={<LuBriefcase sx={stylexStyles.rule2} />}
                loadingLabel="Loading jobs..."
                emptyLabel="No jobs in cluster"
              />
            ) : (
              <div sx={[nativeChildSpacingNextNext.s3, false]}>
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
                      sx={[nativeChildSpacingNext.s2, stylexStyles4.rule1]}
                    >
                      <div sx={[nativeChildSpacing.s3, stylexStyles3.rule6]}>
                        <div sx={stylexStyles3.rule9}>
                          <ForgeEntityLink
                            objectKey={job.objectKey}
                            sx={stylexStyles2.rule2}
                          />
                          <div sx={stylexStyles3.rule10}>
                            {completeTasks}/{jobTasks.length} tasks complete
                          </div>
                        </div>
                        <StateBadge
                          state={job.data.jobState ?? 0}
                          labels={jobStateLabels}
                        />
                      </div>
                      <div sx={[nativeChildSpacing.s3, stylexStyles3.rule11]}>
                        <div
                          sx={stylexStyles3.rule12}
                          style={{
                            '--forge-progress-width': `${progressPercent}%`,
                          }}
                        />
                      </div>
                      {startable && (
                        <div sx={[nativeChildSpacing.s3, stylexStyles3.rule8]}>
                          <DashboardButton
                            icon={<LuPlay sx={stylexStyles.rule3} />}
                            onClick={() => {
                              void handleStartJob(job.objectKey)
                            }}
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
        ),
      },
      {
        id: 'settings',
        label: 'Settings',
        content: (
          <InfoCard title="Cluster identity">
            <div sx={false}>
              <p sx={[nativeChildSpacing.s2, stylexStyles3.rule13]}>
                The object key identifies this Cluster in the Space graph.
              </p>
              {cluster?.name && (
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Name"
                  value={cluster.name}
                />
              )}
              <CopyableField
                sx={nativeChildSpacing.s2}
                label="Object Key"
                value={objectKey}
              />
              {cluster?.peerId ? (
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Peer ID"
                  value={cluster.peerId}
                />
              ) : (
                <p sx={[nativeChildSpacing.s2, stylexStyles3.rule13]}>
                  No peer ID is assigned. Workers can join this Cluster without
                  a Cluster peer identity.
                </p>
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
      retrySnapshot,
      snapshotError,
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
      stateKey={objectKey}
      icon={<LuServer sx={stylexStyles.rule4} />}
      title={cluster?.name || 'Cluster'}
      tabs={tabs}
    />
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
