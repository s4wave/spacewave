import { memo, useMemo } from 'react'
import { LuActivity, LuListTodo, LuPlay } from 'react-icons/lu'

import {
  Task,
  State,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { State as ExecutionState } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import {
  Pass,
  State as PassState,
} from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import { messageDecoder } from '@s4wave/sdk/world/entity-set.js'
import { entitySetRows, useEntitySet } from '@s4wave/web/hooks/useEntitySet.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { useForgeBlock } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  forgeBlockStatus,
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { PRED_TASK_TO_PASS } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'
import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'
import { ForgeSetList, type ForgeSetRow } from './ForgeSetList.js'

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

const passDecoder = messageDecoder(Pass)
const passIcon = <LuPlay className="size-3 shrink-0" />

// comparePasses orders passes newest first: by nonce, then by time.
function comparePasses(a: ForgeSetRow<Pass>, b: ForgeSetRow<Pass>): number {
  const aNonce = a.value.passNonce ?? 0n
  const bNonce = b.value.passNonce ?? 0n
  if (aNonce !== bNonce) return bNonce > aNonce ? 1 : -1
  const aTime = a.value.timestamp?.getTime() ?? 0
  const bTime = b.value.timestamp?.getTime() ?? 0
  return bTime - aTime
}

// PassRow draws one pass. It draws again only when its body changed.
const PassRow = memo(function PassRow({ objectKey, value }: ForgeSetRow<Pass>) {
  const execCount = value.execStates?.length ?? 0
  return (
    <div className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition duration-150">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="text-foreground truncate text-sm font-medium">
            Pass #{value.passNonce?.toString() ?? '0'}
          </div>
          <div className="text-foreground-alt/50 truncate text-xs">
            {value.timestamp?.toISOString() ?? objectKey}
          </div>
        </div>
        <StateBadge state={value.passState ?? 0} labels={passStateLabels} />
      </div>
      <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
        <span>
          {execCount} execution{execCount === 1 ? '' : 's'}
        </span>
        <span>Replicas {value.replicas ?? 0}</span>
        <span>{describeResult(value.result)}</span>
      </div>
    </div>
  )
})

function renderPassRow(row: ForgeSetRow<Pass>) {
  return <PassRow key={row.objectKey} {...row} />
}

// ForgeTaskViewer displays a Forge Task entity with tabbed layout.
export function ForgeTaskViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const block = useForgeBlock(objectState, ForgeTaskTypeID, Task)
  const task = block.value ?? undefined
  useObjectName(task?.name)

  const passes = useEntitySet(
    worldState,
    {
      case: 'link',
      origin: objectKey,
      predicate: PRED_TASK_TO_PASS,
      direction: 'out',
    },
    passDecoder,
  )
  const passSet = resourceContent(passes)
  const sortedPasses = useMemo(
    () => entitySetRows(passSet).toSorted(comparePasses),
    [passSet],
  )
  const currentPass = useMemo(
    () =>
      sortedPasses.find(
        (pass) =>
          (pass.value.passState ?? PassState.PassState_UNKNOWN) !==
          PassState.PassState_COMPLETE,
      ) ??
      sortedPasses[0] ??
      null,
    [sortedPasses],
  )
  const currentExecution = useMemo(() => {
    const execStates = currentPass?.value.execStates ?? []
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
          <div className="space-y-3">
            <FigureStrip
              figures={[
                {
                  label: 'Passes',
                  value: passSet?.keys.length ?? '-',
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
              icon={
                <LuActivity
                  aria-hidden
                  className="text-foreground-alt/60 size-3.5"
                />
              }
              title="Current Execution"
            >
              {!currentPass && (
                <div className="text-foreground-alt/50 text-xs">
                  No pass has started yet
                </div>
              )}
              {currentPass && (
                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="text-foreground text-sm font-medium">
                        Pass #{currentPass.value.passNonce?.toString() ?? '0'}
                      </div>
                      <div className="text-foreground-alt/50 text-xs">
                        {currentPass.value.execStates?.length ?? 0} execution
                        {(currentPass.value.execStates?.length ?? 0) === 1
                          ? ''
                          : 's'}
                      </div>
                    </div>
                    <StateBadge
                      state={currentPass.value.passState ?? 0}
                      labels={passStateLabels}
                    />
                  </div>
                  {currentExecution && (
                    <div className="space-y-2">
                      <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <div className="text-foreground truncate text-xs font-medium">
                            {currentExecution.objectKey || 'Execution'}
                          </div>
                          <div className="text-foreground-alt/50 truncate text-xs">
                            {currentExecution.peerId || 'Worker pending'}
                          </div>
                        </div>
                        <StateBadge
                          state={currentExecution.executionState ?? 0}
                          labels={executionStateLabels}
                        />
                      </div>
                      {currentExecution.timestamp && (
                        <div className="text-foreground-alt/50 text-xs">
                          Started {currentExecution.timestamp.toISOString()}
                        </div>
                      )}
                    </div>
                  )}
                  {!currentExecution && (
                    <div className="text-foreground-alt/50 text-xs">
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
          <ForgeSetList
            set={passes}
            what="the passes"
            icon={passIcon}
            loadingLabel="Loading passes..."
            emptyLabel="No passes yet"
            renderRow={renderPassRow}
            compare={comparePasses}
          />
        ),
      },
      {
        id: 'outputs',
        label: 'Outputs',
        content: (
          <div className="space-y-3">
            <ForgeValueSetDisplay
              title="Outputs"
              values={task?.valueSet?.outputs}
              emptyLabel="No outputs captured yet"
            />
            <InfoCard title="Result">
              <div className="text-foreground text-sm font-medium">
                {describeResult(task?.result)}
              </div>
            </InfoCard>
          </div>
        ),
      },
    ],
    [currentExecution, currentPass, passSet, passes, task],
  )

  // Details lists the task's identity behind the bar.
  const facts = (
    <>
      {task?.name && <FactRow kind="text" label="Name" value={task.name} />}
      {task?.peerId && <FactRow label="Assigned To" value={task.peerId} />}
      {task?.timestamp && (
        <FactRow
          kind="text"
          label="Created"
          value={task.timestamp.toISOString()}
        />
      )}
      <FactRow label="Object Key" value={objectKey} />
    </>
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuListTodo className="size-4" />}
      title={task?.name || 'Task'}
      state={task?.taskState}
      stateLabels={taskStateLabels}
      details={{ label: 'Task', facts }}
      status={forgeBlockStatus('the task', block)}
      tabs={tabs}
    />
  )
}
