import { memo, useMemo } from 'react'
import { LuActivity, LuPlay } from 'react-icons/lu'

import {
  Pass,
  State,
} from '@go/github.com/s4wave/spacewave/forge/pass/pass.pb.js'
import { Execution } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import { messageDecoder } from '@s4wave/sdk/world/entity-set.js'
import { useEntitySet } from '@s4wave/web/hooks/useEntitySet.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlock } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  forgeBlockStatus,
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { ForgeEntityLink } from '@s4wave/web/forge/ForgeEntityLink.js'
import { PRED_PASS_TO_EXECUTION } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'
import { ForgeValueSetPanels } from './ForgeValueSetDisplay.js'
import { ForgeSetList, type ForgeSetRow } from './ForgeSetList.js'

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

const executionDecoder = messageDecoder(Execution)
const executionIcon = <LuActivity className="size-3 shrink-0" />

// ExecutionRow draws one execution; it draws again only when its body
// changed.
const ExecutionRow = memo(function ExecutionRow({
  objectKey,
  value,
}: ForgeSetRow<Execution>) {
  return (
    <div className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition duration-150">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <ForgeEntityLink
            objectKey={objectKey}
            className="text-foreground truncate text-sm font-medium"
          />
          <div className="text-foreground-alt/50 truncate text-xs">
            {value.peerId || 'Worker pending'}
          </div>
        </div>
        <StateBadge
          state={value.executionState ?? 0}
          labels={executionStateLabels}
        />
      </div>
      <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
        <span>{value.timestamp?.toISOString() ?? 'No timestamp'}</span>
        <span>{describeResult(value.result)}</span>
      </div>
    </div>
  )
})

function renderExecutionRow(row: ForgeSetRow<Execution>) {
  return <ExecutionRow key={row.objectKey} {...row} />
}

// ForgePassViewer displays a Forge Pass entity with execution list.
export function ForgePassViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const block = useForgeBlock(objectState, ForgePassTypeID, Pass)
  const pass = block.value ?? undefined

  const executions = useEntitySet(
    worldState,
    {
      case: 'link',
      origin: objectKey,
      predicate: PRED_PASS_TO_EXECUTION,
      direction: 'out',
    },
    executionDecoder,
  )
  const executionCount = resourceContent(executions)?.keys.length

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            <FigureStrip
              figures={[
                { label: 'Replicas', value: pass?.replicas ?? 0 },
                {
                  label: 'Executions',
                  value: executionCount ?? '-',
                },
              ]}
            />
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
        content: (
          <ForgeSetList
            set={executions}
            what="the executions"
            icon={executionIcon}
            loadingLabel="Loading executions..."
            emptyLabel="No executions yet"
            renderRow={renderExecutionRow}
          />
        ),
      },
      {
        id: 'values',
        label: 'Values',
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
    [executionCount, executions, pass],
  )

  // Details lists the pass's identity behind the bar.
  const facts = (
    <>
      {pass?.peerId && <FactRow label="Peer ID" value={pass.peerId} />}
      {pass?.timestamp && (
        <FactRow
          kind="text"
          label="Created"
          value={pass.timestamp.toISOString()}
        />
      )}
      <FactRow
        kind="text"
        label="Pass Nonce"
        value={pass?.passNonce?.toString() ?? '0'}
      />
      <FactRow label="Object Key" value={objectKey} />
    </>
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuPlay className="size-4" />}
      title="Pass"
      state={pass?.passState}
      details={{ label: 'Pass', facts }}
      status={forgeBlockStatus('the pass', block)}
      stateLabels={passStateLabels}
      tabs={tabs}
    />
  )
}
