import { useCallback, useMemo } from 'react'
import { LuActivity, LuCpu, LuServer } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { State as ExecutionState } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import { Worker } from '@go/github.com/s4wave/spacewave/forge/worker/worker.pb.js'
import type { ProcessBindingInfo } from '@s4wave/sdk/space/space.pb.js'
import { SpaceContentsContext } from '@s4wave/web/contexts/contexts.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { useForgeLinkedEntities } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import {
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { ForgeEntityLink } from '@s4wave/web/forge/ForgeEntityLink.js'
import { PRED_CLUSTER_TO_WORKER } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { StatCard } from '@s4wave/web/ui/StatCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { useForgeClusterSnapshot } from './useForgeClusterSnapshot.js'

export const ForgeWorkerTypeID = 'forge/worker'

const executionStateLabels: Record<number, string> = {
  [ExecutionState.ExecutionState_UNKNOWN]: 'UNKNOWN',
  [ExecutionState.ExecutionState_PENDING]: 'PENDING',
  [ExecutionState.ExecutionState_RUNNING]: 'RUNNING',
  [ExecutionState.ExecutionState_COMPLETE]: 'COMPLETE',
}

// ForgeWorkerViewer displays a Forge Worker entity with tabbed layout.
export function ForgeWorkerViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const worker = useForgeBlockData(objectState, Worker)

  const { entities: clusters, loading: clustersLoading } =
    useForgeLinkedEntities(worldState, objectKey, PRED_CLUSTER_TO_WORKER, 'in')
  const { snapshot, loading: snapshotLoading } = useForgeClusterSnapshot(
    worldState,
    clusters.map((cluster) => cluster.objectKey),
  )
  const workerSnapshot = useMemo(
    () =>
      snapshot.workers.find(
        (workerSnapshot) => workerSnapshot.objectKey === objectKey,
      ) ?? null,
    [objectKey, snapshot.workers],
  )
  const peerIds = useMemo(() => workerSnapshot?.peerIds ?? [], [workerSnapshot])
  const executions = useMemo(
    () =>
      snapshot.executions.filter((execution) =>
        peerIds.includes(execution.data.peerId ?? ''),
      ),
    [peerIds, snapshot.executions],
  )
  const activeExecutions = useMemo(
    () =>
      executions.filter(
        (execution) =>
          (execution.data.executionState ??
            ExecutionState.ExecutionState_UNKNOWN) !==
          ExecutionState.ExecutionState_COMPLETE,
      ),
    [executions],
  )
  const completeExecutions = useMemo(
    () =>
      executions.filter(
        (execution) =>
          (execution.data.executionState ??
            ExecutionState.ExecutionState_UNKNOWN) ===
          ExecutionState.ExecutionState_COMPLETE,
      ),
    [executions],
  )
  const contentsResource = SpaceContentsContext.useContext()
  const contents = useResourceValue(contentsResource)
  const contentsState = useStreamingResource(
    contentsResource,
    useCallback((contents, signal) => contents.watchState({}, signal), []),
    [],
  )
  const bindings = useMemo<ProcessBindingInfo[]>(
    () => contentsState.value?.processBindings ?? [],
    [contentsState.value?.processBindings],
  )
  const binding = useMemo(
    () => bindings.find((binding) => binding.objectKey === objectKey) ?? null,
    [bindings, objectKey],
  )
  const handleToggleWorker = useCallback(async () => {
    if (!contents) return
    await contents.setProcessBinding(
      objectKey,
      binding?.typeId ?? 'forge/worker',
      !(binding?.approved ?? false),
    )
  }, [binding?.approved, binding?.typeId, contents, objectKey])
  const actions = useMemo(() => {
    if (!binding && !contents) return []
    return [
      {
        label: (binding?.approved ?? false) ? 'Stop Worker' : 'Start Worker',
        icon: <LuActivity className="h-3.5 w-3.5" />,
        onClick: () => {
          void handleToggleWorker()
        },
      },
    ]
  }, [binding, contents, handleToggleWorker])

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            <InfoCard>
              <div className="space-y-2">
                {worker?.name && (
                  <CopyableField label="Name" value={worker.name} />
                )}
                <CopyableField label="Object Key" value={objectKey} />
              </div>
            </InfoCard>
            <div className="grid grid-cols-2 gap-3">
              <StatCard
                icon={LuServer}
                label="Clusters"
                value={clustersLoading ? '-' : clusters.length}
              />
              <StatCard
                icon={LuActivity}
                label="Capacity"
                value={`${activeExecutions.length}/${Math.max(peerIds.length, 1)}`}
                detail="active executions / peers"
              />
            </div>
            <InfoCard title="Peer IDs">
              {snapshotLoading && (
                <div className="text-foreground-alt/50 text-xs">
                  Loading worker identities...
                </div>
              )}
              {!snapshotLoading && peerIds.length === 0 && (
                <div className="text-foreground-alt/50 text-xs">
                  No keypairs linked to this worker
                </div>
              )}
              {!snapshotLoading && peerIds.length > 0 && (
                <div className="space-y-2">
                  {peerIds.map((peerId) => (
                    <CopyableField
                      key={peerId}
                      label="Peer ID"
                      value={peerId}
                    />
                  ))}
                </div>
              )}
              {binding && (
                <div className="text-foreground-alt/50 mt-3 text-xs">
                  Process binding:{' '}
                  {(binding.approved ?? false) ? 'approved' : 'unapproved'}
                </div>
              )}
            </InfoCard>
          </div>
        ),
      },
      {
        id: 'assignments',
        label: 'Assignments',
        content: (
          <div className="space-y-2">
            {activeExecutions.length === 0 && (
              <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuActivity className="h-3.5 w-3.5 shrink-0" />
                  <span>No active executions assigned to this worker</span>
                </div>
              </div>
            )}
            {activeExecutions.map((execution) => (
              <div
                key={execution.objectKey}
                className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <ForgeEntityLink
                      objectKey={execution.objectKey}
                      className="text-foreground truncate text-sm font-medium"
                    />
                    <div className="text-foreground-alt/50 truncate text-xs">
                      {execution.jobKey} / {execution.taskKey}
                    </div>
                  </div>
                  <StateBadge
                    state={execution.data.executionState ?? 0}
                    labels={executionStateLabels}
                  />
                </div>
                <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
                  <span>{execution.passKey}</span>
                  <span>
                    {execution.data.timestamp?.toISOString() ?? 'No timestamp'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        ),
      },
      {
        id: 'history',
        label: 'History',
        content: (
          <div className="space-y-2">
            {completeExecutions.length === 0 && (
              <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuActivity className="h-3.5 w-3.5 shrink-0" />
                  <span>No completed executions recorded yet</span>
                </div>
              </div>
            )}
            {completeExecutions.map((execution) => (
              <div
                key={execution.objectKey}
                className="border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 space-y-2 rounded-lg border px-3.5 py-2.5 transition-all duration-150"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <ForgeEntityLink
                      objectKey={execution.objectKey}
                      className="text-foreground truncate text-sm font-medium"
                    />
                    <div className="text-foreground-alt/50 truncate text-xs">
                      {execution.jobKey} / {execution.taskKey}
                    </div>
                  </div>
                  <StateBadge
                    state={execution.data.executionState ?? 0}
                    labels={executionStateLabels}
                  />
                </div>
                <div className="text-foreground-alt/50 flex flex-wrap gap-3 text-xs">
                  <span>
                    {(execution.data.result?.success ?? false) ?
                      'Success'
                    : 'Complete'}
                  </span>
                  <span>
                    {execution.data.timestamp?.toISOString() ?? 'No timestamp'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        ),
      },
    ],
    [
      activeExecutions,
      binding,
      clusters,
      clustersLoading,
      completeExecutions,
      objectKey,
      peerIds,
      snapshotLoading,
      worker,
    ],
  )

  return (
    <ForgeViewerShell
      icon={<LuCpu className="h-4 w-4" />}
      title={worker?.name || 'Worker'}
      tabs={tabs}
      actions={actions}
    />
  )
}
