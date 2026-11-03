import { useMemo } from 'react'
import { LuActivity, LuListTodo, LuPlay } from '@s4wave/web/images/icons.js'

import {
  Task,
  State,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { State as ExecutionState } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import {
  Pass,
  State as PassState,
} from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'

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
import { PRED_TASK_TO_PASS } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
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

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: { minWidth: 0 },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule7: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule8: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
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
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export const ForgeTaskTypeID = 'forge/task'

const taskStateLabels: Record<number, string> = {
  [State.TaskState_UNKNOWN]: 'UNKNOWN',
  [State.TaskState_PENDING]: 'PENDING',
  [State.TaskState_RUNNING]: 'RUNNING',
  [State.TaskState_CHECKING]: 'CHECKING',
  [State.TaskState_COMPLETE]: 'COMPLETE',
  [State.TaskState_RETRY]: 'RETRY',
}

const passStateLabels: Record<number, string> = {
  [PassState.PassState_UNKNOWN]: 'UNKNOWN',
  [PassState.PassState_PENDING]: 'PENDING',
  [PassState.PassState_RUNNING]: 'RUNNING',
  [PassState.PassState_CHECKING]: 'CHECKING',
  [PassState.PassState_COMPLETE]: 'COMPLETE',
}

const executionStateLabels: Record<number, string> = {
  [ExecutionState.ExecutionState_UNKNOWN]: 'UNKNOWN',
  [ExecutionState.ExecutionState_PENDING]: 'PENDING',
  [ExecutionState.ExecutionState_RUNNING]: 'RUNNING',
  [ExecutionState.ExecutionState_COMPLETE]: 'COMPLETE',
}

function describeResult(result?: {
  success?: boolean
  canceled?: boolean
  failError?: string
}): string {
  if (!result) return 'No result recorded yet'
  if (result.canceled ?? false) return 'Canceled'
  if (result.success ?? false) return 'Success'
  if (result.failError) return result.failError
  return 'Failed'
}

// ForgeTaskViewer displays a Forge Task entity with tabbed layout.
export function ForgeTaskViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const task = useForgeBlockData(objectState, ForgeTaskTypeID, Task)
  useObjectName(task?.name)

  const { entities: passes, loading: passesLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_TASK_TO_PASS,
  )
  const { items: decodedPasses, loading: decodedPassesLoading } =
    useForgeDecodedLinkedEntities(worldState, passes, Pass)
  const sortedPasses = useMemo(
    () =>
      decodedPasses.toSorted((a, b) => {
        const aNonce = Number(a.data.passNonce ?? 0n)
        const bNonce = Number(b.data.passNonce ?? 0n)
        if (aNonce !== bNonce) return bNonce - aNonce
        const aTime = a.data.timestamp?.getTime() ?? 0
        const bTime = b.data.timestamp?.getTime() ?? 0
        return bTime - aTime
      }),
    [decodedPasses],
  )
  const currentPass = useMemo(
    () =>
      sortedPasses.find(
        (pass) =>
          (pass.data.passState ?? PassState.PassState_UNKNOWN) !==
          PassState.PassState_COMPLETE,
      ) ??
      sortedPasses[0] ??
      null,
    [sortedPasses],
  )
  const currentExecution = useMemo(() => {
    const execStates = currentPass?.data.execStates ?? []
    return (
      execStates.find(
        (execution) =>
          (execution.executionState ??
            ExecutionState.ExecutionState_UNKNOWN) !==
          ExecutionState.ExecutionState_COMPLETE,
      ) ??
      execStates[0] ??
      null
    )
  }, [currentPass])

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div sx={false}>
            <InfoCard sx={nativeChildSpacing.s3}>
              <div sx={false}>
                {task?.name && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Name"
                    value={task.name}
                  />
                )}
                {task?.peerId && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Assigned To"
                    value={task.peerId}
                  />
                )}
                {task?.timestamp && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Created"
                    value={task.timestamp.toISOString()}
                  />
                )}
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Object Key"
                  value={objectKey}
                />
              </div>
            </InfoCard>
            <FigureStrip
              sx={nativeChildSpacing.s3}
              figures={[
                {
                  label: 'Passes',
                  value: passesLoading ? '-' : passes.length,
                  detail: `Current nonce ${task?.passNonce?.toString() ?? '0'}`,
                },
                {
                  label: 'Task outputs',
                  value: task?.valueSet?.outputs?.length ?? 0,
                  detail: describeResult(task?.result),
                },
              ]}
            />
            <InfoCard
              sx={nativeChildSpacing.s3}
              icon={<LuActivity sx={stylexStyles.rule1} />}
              title="Current Execution"
            >
              {!currentPass && (
                <div sx={stylexStyles2.rule1}>No pass has started yet</div>
              )}
              {currentPass && (
                <div sx={false}>
                  <div sx={[nativeChildSpacingNext.s3, stylexStyles2.rule2]}>
                    <div sx={stylexStyles2.rule3}>
                      <div sx={stylexStyles2.rule4}>
                        Pass #{currentPass.data.passNonce?.toString() ?? '0'}
                      </div>
                      <div sx={stylexStyles2.rule1}>
                        {currentPass.data.execStates?.length ?? 0} execution
                        {(currentPass.data.execStates?.length ?? 0) === 1
                          ? ''
                          : 's'}
                      </div>
                    </div>
                    <StateBadge
                      state={currentPass.data.passState ?? 0}
                      labels={passStateLabels}
                    />
                  </div>
                  {currentExecution && (
                    <div sx={[nativeChildSpacingNext.s3, false]}>
                      <div sx={[nativeChildSpacing.s2, stylexStyles2.rule2]}>
                        <div sx={stylexStyles2.rule3}>
                          <div sx={stylexStyles2.rule5}>
                            {currentExecution.objectKey || 'Execution'}
                          </div>
                          <div sx={stylexStyles2.rule6}>
                            {currentExecution.peerId || 'Worker pending'}
                          </div>
                        </div>
                        <StateBadge
                          state={currentExecution.executionState ?? 0}
                          labels={executionStateLabels}
                        />
                      </div>
                      {currentExecution.timestamp && (
                        <div sx={[nativeChildSpacing.s2, stylexStyles2.rule1]}>
                          Started {currentExecution.timestamp.toISOString()}
                        </div>
                      )}
                    </div>
                  )}
                  {!currentExecution && (
                    <div sx={[nativeChildSpacingNext.s3, stylexStyles2.rule1]}>
                      No execution snapshot recorded yet
                    </div>
                  )}
                </div>
              )}
            </InfoCard>
          </div>
        ),
      },
      {
        id: 'passes',
        label: 'Pass History',
        content: (
          <div sx={false}>
            {sortedPasses.length === 0 && (
              <ForgeEntityList
                sx={nativeChildSpacingNext.s2}
                entities={passes}
                loading={passesLoading || decodedPassesLoading}
                icon={<LuPlay sx={stylexStyles.rule2} />}
                loadingLabel="Loading passes..."
                emptyLabel="No passes yet"
              />
            )}
            {sortedPasses.map((pass) => (
              <div
                key={pass.entity.objectKey}
                sx={[nativeChildSpacingNext.s2, stylexStyles3.rule1]}
              >
                <div sx={[nativeChildSpacing.s2, stylexStyles2.rule2]}>
                  <div sx={stylexStyles2.rule3}>
                    <div sx={stylexStyles2.rule7}>
                      Pass #{pass.data.passNonce?.toString() ?? '0'}
                    </div>
                    <div sx={stylexStyles2.rule6}>
                      {pass.data.timestamp?.toISOString() ??
                        pass.entity.objectKey}
                    </div>
                  </div>
                  <StateBadge
                    state={pass.data.passState ?? 0}
                    labels={passStateLabels}
                  />
                </div>
                <div sx={[nativeChildSpacing.s2, stylexStyles2.rule8]}>
                  <span>
                    {pass.data.execStates?.length ?? 0} execution
                    {(pass.data.execStates?.length ?? 0) === 1 ? '' : 's'}
                  </span>
                  <span>Replicas {pass.data.replicas ?? 0}</span>
                  <span>{describeResult(pass.data.result)}</span>
                </div>
              </div>
            ))}
          </div>
        ),
      },
      {
        id: 'outputs',
        label: 'Outputs',
        content: (
          <div sx={false}>
            <ForgeValueSetDisplay
              sx={nativeChildSpacingNext.s3}
              title="Outputs"
              values={task?.valueSet?.outputs}
              emptyLabel="No outputs captured yet"
            />
            <InfoCard sx={nativeChildSpacingNext.s3} title="Result">
              <div sx={stylexStyles2.rule4}>{describeResult(task?.result)}</div>
            </InfoCard>
          </div>
        ),
      },
    ],
    [
      currentExecution,
      currentPass,
      decodedPassesLoading,
      objectKey,
      passes,
      passesLoading,
      sortedPasses,
      task,
    ],
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuListTodo sx={stylexStyles.rule3} />}
      title={task?.name || 'Task'}
      state={task?.taskState ?? 0}
      stateLabels={taskStateLabels}
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
