import { forgeControlStyles } from '@s4wave/web/style/forge-controls.stylex.js'
/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuBriefcase,
  LuGitBranch,
  LuList,
  LuListTodo,
  LuPlus,
} from '@s4wave/web/images/icons.js'

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
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'
import { useForgeTaskDependencyGraph } from './useForgeTaskDependencyGraph.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    display: 'flex',
    justifyContent: 'space-between',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: { minWidth: 0 },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule4: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule6: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.6rem',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    marginTop: themeLayout['--spacing'],
  },
  rule11: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
  },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule13: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    width: '100%',
  },
  rule14: {
    transition: 'width 200ms',
    backgroundColor: themeColors['--color-brand'],
    height: '100%',
    width: 'var(--forge-progress-width)',
  },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule16: {
    padding: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-md'],
    gap: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'inline-flex',
  },
  rule17: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
  },
  rule18: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule19: {
    borderColor: '#0000',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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

/** ForgeJobViewer displays a Forge Job entity with tabbed layout. */
export function ForgeJobViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const job = useForgeBlockData(objectState, ForgeJobTypeID, Job)
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

  const renderTasksContent = useCallback(
    (sx?: stylex.StyleXStyles) => {
      if (tasksView === 'list') {
        if (tasksLoading || decodedTasksLoading) {
          return (
            <ForgeEntityList
              sx={sx}
              entities={tasks}
              loading
              icon={<LuListTodo sx={stylexStyles.rule1} />}
              loadingLabel="Loading tasks..."
              emptyLabel="No tasks in job"
            />
          )
        }
        if (decodedTasks.length === 0) {
          return (
            <ForgeEntityList
              sx={sx}
              entities={tasks}
              loading={tasksLoading}
              icon={<LuListTodo sx={stylexStyles.rule1} />}
              loadingLabel="Loading tasks..."
              emptyLabel="No tasks in job"
            />
          )
        }

        return (
          <div sx={[false, sx]}>
            {decodedTasks.map((task) => (
              <div
                key={task.entity.objectKey}
                sx={[nativeChildSpacing.s2, stylexStyles2.rule1]}
              >
                <div sx={stylexStyles2.rule2}>
                  <div sx={stylexStyles2.rule3}>
                    {task.data.name || task.entity.objectKey}
                  </div>
                  <div sx={stylexStyles2.rule4}>{task.entity.objectKey}</div>
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
          <div sx={[stylexStyles2.rule5, sx]}>
            <div sx={stylexStyles2.rule6}>
              <LuGitBranch sx={stylexStyles.rule2} />
              <span>Loading dependency graph…</span>
            </div>
          </div>
        )
      }
      if (decodedTasks.length === 0) {
        return (
          <div sx={[stylexStyles2.rule5, sx]}>
            <div sx={stylexStyles2.rule6}>
              <LuListTodo sx={stylexStyles.rule2} />
              <span>No tasks in job</span>
            </div>
          </div>
        )
      }
      if (taskEdges.length === 0) {
        return (
          <div sx={[stylexStyles2.rule5, sx]}>
            <div sx={stylexStyles2.rule6}>
              <LuGitBranch sx={stylexStyles.rule2} />
              <span>No task dependency edges defined yet</span>
            </div>
          </div>
        )
      }

      return (
        <div sx={[false, sx]}>
          {taskEdges.map((edge) => {
            const fromTask = taskByKey.get(edge.from)
            const toTask = taskByKey.get(edge.to)
            return (
              <div
                key={`${edge.kind}:${edge.from}:${edge.to}`}
                sx={[nativeChildSpacing.s2, stylexStyles2.rule7]}
              >
                <div sx={stylexStyles2.rule8}>
                  <LuGitBranch sx={stylexStyles.rule3} />
                  <span>{fromTask?.data.name || edge.from}</span>
                  <span sx={stylexStyles2.rule9}>-&gt;</span>
                  <span>{toTask?.data.name || edge.to}</span>
                </div>
                <div sx={stylexStyles2.rule10}>{edge.kind}</div>
              </div>
            )
          })}
        </div>
      )
    },
    [
      decodedTasks,
      decodedTasksLoading,
      taskByKey,
      taskEdges,
      taskEdgesLoading,
      tasks,
      tasksLoading,
      tasksView,
    ],
  )

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div sx={false}>
            <InfoCard sx={nativeChildSpacing.s3}>
              <div sx={false}>
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Object Key"
                  value={objectKey}
                />
                {job?.timestamp && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Created"
                    value={job.timestamp.toISOString()}
                  />
                )}
              </div>
            </InfoCard>
            <InfoCard
              sx={nativeChildSpacing.s3}
              icon={<LuListTodo sx={stylexStyles.rule4} />}
              title="Tasks"
            >
              <div sx={stylexStyles2.rule11}>
                {tasksLoading ? '-' : `${completeTaskCount}/${tasks.length}`}
              </div>
              <div sx={stylexStyles2.rule12}>{progressPercent}% complete</div>
              <div sx={stylexStyles2.rule13}>
                <div
                  sx={stylexStyles2.rule14}
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
          <div sx={false}>
            <div sx={[nativeChildSpacing.s3, stylexStyles2.rule15]}>
              <div sx={stylexStyles2.rule16}>
                <button
                  type="button"
                  onClick={() => setTasksView('list')}
                  sx={[
                    [
                      stylexStyles2.rule17,
                      tasksView === 'list'
                        ? stylexStyles2.rule18
                        : stylexStyles2.rule19,
                    ],
                    forgeControlStyles.required,
                  ]}
                >
                  <LuList sx={stylexStyles.rule3} />
                  List
                </button>
                <button
                  type="button"
                  onClick={() => setTasksView('dag')}
                  sx={[
                    [
                      stylexStyles2.rule17,
                      tasksView === 'dag'
                        ? stylexStyles2.rule18
                        : stylexStyles2.rule19,
                    ],
                    forgeControlStyles.required,
                  ]}
                >
                  <LuGitBranch sx={stylexStyles.rule3} />
                  DAG
                </button>
              </div>
              {canCreateTask && (
                <DashboardButton
                  icon={<LuPlus sx={stylexStyles.rule3} />}
                  onClick={() => {
                    void handleAddTask()
                  }}
                  disabled={creatingTask}
                >
                  {creatingTask ? 'Adding…' : 'Add Task'}
                </DashboardButton>
              )}
            </div>
            {renderTasksContent(nativeChildSpacing.s3)}
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
      renderTasksContent,
      tasksLoading,
      tasksView,
    ],
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuBriefcase sx={stylexStyles.rule5} />}
      title="Job"
      state={job?.jobState ?? 0}
      stateLabels={jobStateLabels}
      tabs={tabs}
    />
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
