import { useCallback, useMemo } from 'react'
import {
  LuLayoutDashboard,
  LuBox,
  LuActivity,
  LuBriefcase,
  LuPlus,
  LuServer,
} from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { ForgeDashboard } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
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
import { PRED_DASHBOARD_FORGE_REF } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { ProcessBindingList } from './ProcessBindingList.js'
import { useForgeDashboardActivity } from './useForgeDashboardActivity.js'
import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'

export const ForgeDashboardTypeID = 'spacewave/forge/dashboard'

// entityTypeLabels maps forge type IDs to display labels.
const entityTypeLabels: Record<string, string> = {
  'forge/task': 'TASK',
  'forge/job': 'JOB',
  'forge/cluster': 'CLUSTER',
  'forge/worker': 'WORKER',
  'forge/pass': 'PASS',
  'forge/execution': 'EXECUTION',
}

// ForgeDashboardViewer displays a Forge Dashboard unified control panel.
// Uses client-side data access: useForgeBlockData for the dashboard block,
// lookupGraphQuads for linked entities, SpaceContents for process bindings.
export function ForgeDashboardViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const dashboard = useForgeBlockData(objectState, ForgeDashboard)
  const { navigateToObjects, spaceState, spaceWorld } =
    SpaceContainerContext.useContext()
  const visibleWizardTypeSet = useVisibleObjectWizardTypeSet()

  const { entities, loading: entitiesLoading } = useForgeLinkedEntities(
    worldState,
    objectKey,
    PRED_DASHBOARD_FORGE_REF,
  )
  const { entries: activityEntries, loading: activityLoading } =
    useForgeDashboardActivity(worldState, dashboard, entities)

  // Group entities by type for summary cards.
  const typeCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const e of entities) {
      const label = entityTypeLabels[e.typeId] ?? e.typeId
      counts[label] = (counts[label] ?? 0) + 1
    }
    return counts
  }, [entities])

  // Get process bindings from SpaceContents.
  const contentsResource = SpaceContentsContext.useContext()
  const contents = useResourceValue(contentsResource)
  const contentsState = useStreamingResource(
    contentsResource,
    useCallback((contents, signal) => contents.watchState({}, signal), []),
    [],
  )
  const bindings: ProcessBindingInfo[] = useMemo(
    () => contentsState.value?.processBindings ?? [],
    [contentsState.value?.processBindings],
  )
  const bindingsByObjectKey = useMemo(
    () =>
      new Map(bindings.map((binding) => [binding.objectKey ?? '', binding])),
    [bindings],
  )
  const pendingWorkers = useMemo(
    () =>
      entities.filter((entity) => {
        if (entity.typeId !== 'forge/worker') return false
        return !(bindingsByObjectKey.get(entity.objectKey)?.approved ?? false)
      }),
    [bindingsByObjectKey, entities],
  )
  const clusterEntities = useMemo(
    () => entities.filter((entity) => entity.typeId === 'forge/cluster'),
    [entities],
  )
  const canCreateCluster = visibleWizardTypeSet.has('forge/cluster')
  const canCreateJob = visibleWizardTypeSet.has('forge/job')
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map((obj) => obj.objectKey ?? '') ??
      [],
    [spaceState.worldContents?.objects],
  )

  const openWizard = useCallback(
    async (
      wizardTypeId: string,
      targetTypeId: string,
      targetKeyPrefix: string,
      name: string,
      opts?: {
        initialStep?: number
        initialConfigData?: Uint8Array
      },
    ) => {
      const wizardKey = buildWizardObjectKey(name, existingObjectKeys)
      const opData = CreateWizardObjectOp.toBinary({
        objectKey: wizardKey,
        wizardTypeId,
        targetTypeId,
        targetKeyPrefix,
        name,
        timestamp: new Date(),
        initialStep: opts?.initialStep,
        initialConfigData: opts?.initialConfigData,
      })
      await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
      navigateToObjects([wizardKey])
    },
    [existingObjectKeys, navigateToObjects, spaceWorld],
  )

  const handleToggle = useCallback(
    async (bindingObjectKey: string, approved: boolean) => {
      if (!contents) return
      const binding = bindings.find((b) => b.objectKey === bindingObjectKey)
      await contents.setProcessBinding(
        bindingObjectKey,
        binding?.typeId ?? '',
        approved,
      )
    },
    [contents, bindings],
  )
  const handleStartWorkers = useCallback(async () => {
    if (!contents || pendingWorkers.length === 0) return
    await Promise.all(
      pendingWorkers.map((worker) =>
        contents.setProcessBinding(worker.objectKey, worker.typeId ?? '', true),
      ),
    )
  }, [contents, pendingWorkers])
  const handleCreateCluster = useCallback(async () => {
    try {
      await openWizard(
        'wizard/forge/cluster',
        'forge/cluster',
        'forge/cluster/',
        'Cluster',
      )
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open cluster wizard',
      )
    }
  }, [openWizard])
  const handleCreateJob = useCallback(async () => {
    try {
      const selectedClusterKey =
        clusterEntities.length === 1 ?
          (clusterEntities[0]?.objectKey ?? '')
        : ''
      const configData = ForgeJobCreateOp.toBinary({
        jobKey: '',
        clusterKey: selectedClusterKey,
        taskDefs: [],
        timestamp: new Date(),
      })
      await openWizard('wizard/forge/job', 'forge/job', 'forge/job/', 'Job', {
        initialStep: selectedClusterKey ? 1 : 0,
        initialConfigData: configData,
      })
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open job wizard',
      )
    }
  }, [clusterEntities, openWizard])
  const actions = useMemo(() => {
    const nextActions = []
    if (canCreateCluster) {
      nextActions.push({
        label: 'Create Cluster',
        icon: <LuServer className="h-3.5 w-3.5" />,
        onClick: () => {
          void handleCreateCluster()
        },
      })
    }
    if (canCreateJob) {
      nextActions.push({
        label: 'Create Job',
        icon: <LuBriefcase className="h-3.5 w-3.5" />,
        onClick: () => {
          void handleCreateJob()
        },
      })
    }
    if (pendingWorkers.length !== 0) {
      nextActions.unshift({
        label: pendingWorkers.length === 1 ? 'Start Worker' : 'Start Workers',
        icon: <LuPlus className="h-3.5 w-3.5" />,
        onClick: () => {
          void handleStartWorkers()
        },
      })
    }
    return nextActions
  }, [
    canCreateCluster,
    canCreateJob,
    handleCreateCluster,
    handleCreateJob,
    handleStartWorkers,
    pendingWorkers.length,
  ])

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            {pendingWorkers.length > 0 && (
              <div className="border-brand/20 bg-brand/5 rounded-lg border p-3.5">
                <div className="text-foreground text-sm font-medium">
                  Worker ready to start
                </div>
                <div className="text-foreground-alt/60 mt-1 text-xs">
                  Approve the quickstart worker process binding to start task
                  execution in this session.
                </div>
                <button
                  type="button"
                  onClick={() => {
                    void handleStartWorkers()
                  }}
                  className="border-brand/40 bg-brand/10 hover:border-brand/60 hover:bg-brand/15 text-foreground mt-3 rounded-md border px-3 py-1.5 text-xs font-medium transition-all duration-150"
                >
                  {pendingWorkers.length === 1 ?
                    'Start worker'
                  : 'Start workers'}
                </button>
              </div>
            )}
            {/* Summary counts grid */}
            {Object.keys(typeCounts).length > 0 && (
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {Object.entries(typeCounts).map(([label, count]) => (
                  <div
                    key={label}
                    className="border-foreground/6 bg-background-card/30 rounded-lg border p-3"
                  >
                    <div className="text-foreground-alt/60 mb-1 text-[0.6rem] tracking-widest uppercase">
                      {label}
                    </div>
                    <div className="text-foreground text-xl font-semibold">
                      {count}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {entitiesLoading && entities.length === 0 && (
              <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuBox className="h-3.5 w-3.5 shrink-0" />
                  <span>Loading entities...</span>
                </div>
              </div>
            )}
            {!entitiesLoading && entities.length === 0 && (
              <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuBox className="h-3.5 w-3.5 shrink-0" />
                  <span>No linked Forge entities</span>
                </div>
              </div>
            )}
          </div>
        ),
      },
      {
        id: 'activity',
        label: 'Activity',
        content: (
          <div className="space-y-2">
            {activityLoading && activityEntries.length === 0 && (
              <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuActivity className="h-3.5 w-3.5 shrink-0" />
                  <span>Loading activity...</span>
                </div>
              </div>
            )}
            {!activityLoading && activityEntries.length === 0 && (
              <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuActivity className="h-3.5 w-3.5 shrink-0" />
                  <span>No recent activity yet</span>
                </div>
              </div>
            )}
            {activityEntries.map((entry) => {
              const content = (
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <div className="text-foreground text-xs font-medium">
                    {entry.title}
                  </div>
                  <div className="text-foreground-alt/50 truncate text-xs">
                    {entry.detail}
                  </div>
                  <div className="text-foreground-alt/50 text-[0.6rem]">
                    {entry.timestamp.toISOString()}
                  </div>
                </div>
              )

              if (entry.objectKey) {
                return (
                  <ForgeEntityLink
                    key={entry.id}
                    objectKey={entry.objectKey}
                    icon={
                      <LuActivity className="text-foreground-alt/60 h-3 w-3 shrink-0" />
                    }
                  >
                    {content}
                  </ForgeEntityLink>
                )
              }

              return (
                <div
                  key={entry.id}
                  className="border-foreground/6 bg-background-card/30 flex items-start gap-2 rounded-lg border p-3"
                >
                  <LuActivity className="text-foreground-alt/60 mt-0.5 h-3 w-3 shrink-0" />
                  {content}
                </div>
              )
            })}
          </div>
        ),
      },
      {
        id: 'entities',
        label: 'Entities',
        content: (
          <div className="space-y-1">
            {entities.map((entity) => (
              <ForgeEntityLink
                key={entity.objectKey}
                objectKey={entity.objectKey}
                icon={
                  <LuBox className="text-foreground-alt/60 h-3 w-3 shrink-0" />
                }
              >
                {entity.typeId && (
                  <StateBadge
                    state={0}
                    labels={{
                      0: entityTypeLabels[entity.typeId] || entity.typeId,
                    }}
                    variant="dot"
                  />
                )}
                {entity.objectKey}
              </ForgeEntityLink>
            ))}
          </div>
        ),
      },
      {
        id: 'bindings',
        label: 'Bindings',
        content: (
          <div>
            {bindings.length > 0 ?
              <ProcessBindingList
                bindings={bindings}
                onToggle={(bindingObjectKey, approved) => {
                  void handleToggle(bindingObjectKey, approved)
                }}
              />
            : <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
                <div className="text-foreground-alt/40 flex items-center gap-2 px-1 py-1 text-xs">
                  <LuBox className="h-3.5 w-3.5 shrink-0" />
                  <span>No process bindings</span>
                </div>
              </div>
            }
          </div>
        ),
      },
    ],
    [
      typeCounts,
      entitiesLoading,
      entities,
      activityEntries,
      activityLoading,
      bindings,
      pendingWorkers.length,
      handleStartWorkers,
      handleToggle,
    ],
  )

  return (
    <ForgeViewerShell
      icon={<LuLayoutDashboard className="h-4 w-4" />}
      title={dashboard?.name || 'Forge Dashboard'}
      tabs={tabs}
      actions={actions}
    />
  )
}
