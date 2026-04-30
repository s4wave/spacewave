import { useMemo } from 'react'
import { LuActivity, LuCopy, LuPlay } from 'react-icons/lu'

import {
  Pass,
  State,
} from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'
import { Execution } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

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
import { PRED_PASS_TO_EXECUTION } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { StatCard } from '@s4wave/web/ui/StatCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ForgeValueSetPanels } from './ForgeValueSetDisplay.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'

export const ForgePassTypeID = 'forge/pass'

const passStateLabels: Record<number, string> = {
  [State.PassState_UNKNOWN]: 'UNKNOWN',
  [State.PassState_PENDING]: 'PENDING',
  [State.PassState_RUNNING]: 'RUNNING',
  [State.PassState_CHECKING]: 'CHECKING',
  [State.PassState_COMPLETE]: 'COMPLETE',
}

const executionStateLabels: Record<number, string> = {
  0: 'UNKNOWN',
  1: 'PENDING',
  2: 'RUNNING',
  3: 'COMPLETE',
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

// ForgePassViewer displays a Forge Pass entity with execution list.
export function ForgePassViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const pass = useForgeBlockData(objectState, Pass)

  const { entities: executions, loading: executionsLoading } =
    useForgeLinkedEntities(worldState, objectKey, PRED_PASS_TO_EXECUTION)
  const { items: decodedExecutions, loading: decodedExecutionsLoading } =
    useForgeDecodedLinkedEntities(worldState, executions, Execution)

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            <InfoCard>
              <div className="space-y-2">
                {pass?.peerId && (
                  <CopyableField label="Peer ID" value={pass.peerId} />
                )}
                {pass?.timestamp && (
                  <CopyableField
                    label="Created"
                    value={pass.timestamp.toISOString()}
                  />
                )}
                <CopyableField
                  label="Pass Nonce"
                  value={pass?.passNonce?.toString() ?? '0'}
                />
                <CopyableField label="Object Key" value={objectKey} />
              </div>
            </InfoCard>
            <div className="grid grid-cols-2 gap-3">
              <StatCard
                icon={LuCopy}
                label="Replicas"
                value={pass?.replicas ?? 0}
              />
              <StatCard
                icon={LuActivity}
                label="Executions"
                value={
                  decodedExecutionsLoading ? '-' : decodedExecutions.length
                }
              />
            </div>
            <InfoCard title="Result">
              <div className="text-foreground text-sm font-medium">
                {describeResult(pass?.result)}
              </div>
            </InfoCard>
          </div>
        ),
      },
      {
        id: 'executions',
        label: 'Executions',
        content:
          decodedExecutions.length === 0 ?
            <ForgeEntityList
              entities={executions}
              loading={executionsLoading || decodedExecutionsLoading}
              icon={
                <LuActivity className="text-muted-foreground h-3 w-3 shrink-0" />
              }
              loadingLabel="Loading executions..."
              emptyLabel="No executions yet"
            />
          : <div className="space-y-2">
              {decodedExecutions.map((execution) => (
                <div
                  key={execution.entity.objectKey}
                  className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
                >
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <ForgeEntityLink
                        objectKey={execution.entity.objectKey}
                        className="text-foreground truncate text-sm font-medium"
                      />
                      <div className="text-foreground-alt/50 truncate text-xs">
                        {execution.data.peerId || 'Worker pending'}
                      </div>
                    </div>
                    <StateBadge
                      state={execution.data.executionState ?? 0}
                      labels={executionStateLabels}
                    />
                  </div>
                  <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
                    <span>
                      {execution.data.timestamp?.toISOString() ??
                        'No timestamp'}
                    </span>
                    <span>{describeResult(execution.data.result)}</span>
                  </div>
                </div>
              ))}
            </div>,
      },
      {
        id: 'details',
        label: 'Details',
        content: (
          <div className="space-y-3">
            <ForgeValueSetPanels
              valueSet={pass?.valueSet}
              emptyInputsLabel="No pass inputs recorded"
              emptyOutputsLabel="No pass outputs recorded"
            />
            <InfoCard title="Execution Snapshot">
              <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
                <span>{pass?.execStates?.length ?? 0} exec states</span>
                <span>{describeResult(pass?.result)}</span>
              </div>
            </InfoCard>
          </div>
        ),
      },
    ],
    [
      decodedExecutions,
      decodedExecutionsLoading,
      executions,
      executionsLoading,
      objectKey,
      pass,
    ],
  )

  return (
    <ForgeViewerShell
      icon={<LuPlay className="h-4 w-4" />}
      title="Pass"
      state={pass?.passState ?? 0}
      stateLabels={passStateLabels}
      tabs={tabs}
    />
  )
}
