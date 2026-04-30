import { useMemo } from 'react'
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

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
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
import { StatCard } from '@s4wave/web/ui/StatCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'

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
  const task = useForgeBlockData(objectState, Task)

  const { entities: passes, loading: passesLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_TASK_TO_PASS,
  )
  const { items: decodedPasses, loading: decodedPassesLoading } =
    useForgeDecodedLinkedEntities(worldState, passes, Pass)
  const sortedPasses = useMemo(
    () =>
      [...decodedPasses].sort((a, b) => {
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
          <div className="space-y-3">
            <InfoCard>
              <div className="space-y-2">
                {task?.name && <CopyableField label="Name" value={task.name} />}
                {task?.peerId && (
                  <CopyableField label="Assigned To" value={task.peerId} />
                )}
                {task?.timestamp && (
                  <CopyableField
                    label="Created"
                    value={task.timestamp.toISOString()}
                  />
                )}
                <CopyableField label="Object Key" value={objectKey} />
              </div>
            </InfoCard>
            <StatCard
              icon={LuPlay}
              label="Passes"
              value={passesLoading ? '-' : passes.length}
              detail={`Current nonce ${task?.passNonce?.toString() ?? '0'}`}
            />
            <InfoCard
              icon={
                <LuActivity className="text-muted-foreground h-3.5 w-3.5" />
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
                        Pass #{currentPass.data.passNonce?.toString() ?? '0'}
                      </div>
                      <div className="text-foreground-alt/50 text-xs">
                        {currentPass.data.execStates?.length ?? 0} execution
                        {(currentPass.data.execStates?.length ?? 0) === 1 ?
                          ''
                        : 's'}
                      </div>
                    </div>
                    <StateBadge
                      state={currentPass.data.passState ?? 0}
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
            <StatCard
              icon={LuListTodo}
              label="Task Outputs"
              value={task?.valueSet?.outputs?.length ?? 0}
              detail={describeResult(task?.result)}
            />
          </div>
        ),
      },
      {
        id: 'passes',
        label: 'Pass History',
        content: (
          <div className="space-y-2">
            {sortedPasses.length === 0 && (
              <ForgeEntityList
                entities={passes}
                loading={passesLoading || decodedPassesLoading}
                icon={
                  <LuPlay className="text-muted-foreground h-3 w-3 shrink-0" />
                }
                loadingLabel="Loading passes..."
                emptyLabel="No passes yet"
              />
            )}
            {sortedPasses.map((pass) => (
              <div
                key={pass.entity.objectKey}
                className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-foreground truncate text-sm font-medium">
                      Pass #{pass.data.passNonce?.toString() ?? '0'}
                    </div>
                    <div className="text-foreground-alt/50 truncate text-xs">
                      {pass.data.timestamp?.toISOString() ??
                        pass.entity.objectKey}
                    </div>
                  </div>
                  <StateBadge
                    state={pass.data.passState ?? 0}
                    labels={passStateLabels}
                  />
                </div>
                <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
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
      icon={<LuListTodo className="h-4 w-4" />}
      title={task?.name || 'Task'}
      state={task?.taskState ?? 0}
      stateLabels={taskStateLabels}
      tabs={tabs}
    />
  )
}
