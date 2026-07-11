/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
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
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
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
  const dashboard = useForgeBlockData(
    objectState,
    ForgeDashboardTypeID,
    ForgeDashboard,
  )
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
  const worldError = worldState.error

  // Group entities by type for summary cards.
  const typeCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const e of entities) {
      const label = entityTypeLabels[e.typeId] ?? e.typeId
      counts[label] = (counts[label] ?? 0) + 1
    }
    return counts
  }, [entities])
  const summaryMetrics = useMemo(() => {
    const activeWorkCount = activityEntries.filter((entry) =>
      /\b(PENDING|RUNNING|CHECKING|RETRY)\b/.test(entry.title),
    ).length

    return [
      {
        label: 'Jobs',
        count: entities.filter((entity) => entity.typeId === 'forge/job')
          .length,
      },
      {
        label: 'Workers',
        count: entities.filter((entity) => entity.typeId === 'forge/worker')
          .length,
      },
      {
        label: 'Clusters',
        count: entities.filter((entity) => entity.typeId === 'forge/cluster')
          .length,
      },
      { label: 'Active work', count: activeWorkCount },
    ]
  }, [activityEntries, entities])

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
  const bindingsLoading = contentsState.loading && bindings.length === 0
  const bindingsError = contentsState.error
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
  const [openingAction, setOpeningAction] = useState<'cluster' | 'job' | null>(
    null,
  )
  const [creationError, setCreationError] = useState('')
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
    setCreationError('')
    setOpeningAction('cluster')
    try {
      await openWizard(
        'wizard/forge/cluster',
        'forge/cluster',
        'forge/cluster/',
        'Cluster',
      )
    } catch {
      const message = 'Cluster creation is unavailable. Try again.'
      setCreationError(message)
      toast.error(message)
    } finally {
      setOpeningAction(null)
    }
  }, [openWizard])
  const handleCreateJob = useCallback(async () => {
    setCreationError('')
    setOpeningAction('job')
    try {
      const selectedClusterKey =
        clusterEntities.length === 1
          ? (clusterEntities[0]?.objectKey ?? '')
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
    } catch {
      const message = 'Job creation is unavailable. Try again.'
      setCreationError(message)
      toast.error(message)
    } finally {
      setOpeningAction(null)
    }
  }, [clusterEntities, openWizard])
  const headerActions = useMemo(() => {
    const nextActions = []
    if (canCreateJob) {
      nextActions.push({
        label: openingAction === 'job' ? 'Opening Job…' : 'Create Job',
        icon: <LuBriefcase className="size-3.5" />,
        variant: 'primary' as const,
        disabled: openingAction !== null,
        onClick: () => {
          void handleCreateJob()
        },
      })
    }
    if (canCreateCluster) {
      nextActions.push({
        label:
          openingAction === 'cluster' ? 'Opening Cluster…' : 'Create Cluster',
        icon: <LuServer className="size-3.5" />,
        disabled: openingAction !== null,
        onClick: () => {
          void handleCreateCluster()
        },
      })
    }
    return nextActions
  }, [
    canCreateCluster,
    canCreateJob,
    handleCreateCluster,
    handleCreateJob,
    openingAction,
  ])

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'overview',
        label: 'Overview',
        content: (
          <div className="space-y-3">
            {openingAction && (
              <div
                role="status"
                className="border-brand/20 bg-brand/5 text-foreground-alt/70 rounded-lg border px-3.5 py-2.5 text-xs"
              >
                Opening Forge {openingAction}…
              </div>
            )}

            {worldError ? (
              <LoadingCard
                view={{
                  state: 'error',
                  title: 'Forge entities unavailable',
                  detail:
                    'This dashboard could not read its linked Forge work.',
                  error: 'Try again to reload Forge entities.',
                  onRetry: worldState.retry,
                }}
              />
            ) : (
              entitiesLoading &&
              entities.length === 0 && (
                <LoadingCard
                  view={{
                    state: 'loading',
                    title: 'Loading Forge entities…',
                    detail: 'Reading Jobs, Workers, Clusters, and active work.',
                    progressIndeterminate: true,
                  }}
                />
              )
            )}

            {!worldError && !entitiesLoading && entities.length === 0 && (
              <InfoCard
                icon={<LuLayoutDashboard className="text-brand size-4" />}
                title="No Forge work yet"
              >
                <p className="text-foreground-alt/70 text-xs leading-relaxed">
                  Create a Job to define work, or create a Cluster to attach
                  execution capacity.
                </p>
              </InfoCard>
            )}

            {entities.length > 0 && (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {summaryMetrics.map((metric) => (
                  <div
                    key={metric.label}
                    className="border-foreground/6 bg-background-card/30 rounded-lg border p-3"
                  >
                    <div className="text-foreground-alt/60 mb-1 text-[0.6rem] font-medium tracking-widest uppercase">
                      {metric.label}
                    </div>
                    <div className="text-foreground text-xl font-semibold tabular-nums">
                      {metric.count}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {pendingWorkers.length > 0 && (
              <InfoCard
                icon={<LuPlus className="text-brand size-4" />}
                title={
                  pendingWorkers.length === 1
                    ? 'Worker ready to start'
                    : 'Workers ready to start'
                }
              >
                <p className="text-foreground-alt/70 text-xs leading-relaxed">
                  Approve the quickstart worker process binding to start task
                  execution in this session.
                </p>
                <button
                  type="button"
                  onClick={() => {
                    void handleStartWorkers()
                  }}
                  className="border-brand/40 bg-brand/10 hover:border-brand/60 hover:bg-brand/15 text-foreground mt-3 inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition-all duration-150"
                >
                  <LuPlus className="size-3.5" />
                  {pendingWorkers.length === 1
                    ? 'Start worker'
                    : 'Start workers'}
                </button>
              </InfoCard>
            )}

            {entities.length > 0 && (
              <div className="grid gap-3 lg:grid-cols-2">
                <InfoCard
                  icon={<LuBox className="text-foreground-alt/70 size-3.5" />}
                  title="Entity health"
                >
                  <div className="space-y-1.5">
                    {Object.entries(typeCounts).map(([label, count]) => (
                      <div
                        key={label}
                        className="text-foreground-alt/70 flex items-center justify-between gap-3 text-xs"
                      >
                        <span>{label}</span>
                        <span className="text-foreground font-medium tabular-nums">
                          {count} linked
                        </span>
                      </div>
                    ))}
                  </div>
                </InfoCard>

                <InfoCard
                  icon={
                    <LuActivity className="text-foreground-alt/70 size-3.5" />
                  }
                  title="Recent activity"
                >
                  {activityLoading && activityEntries.length === 0 && (
                    <div className="text-foreground-alt/60 flex items-center gap-2 text-xs">
                      <LuActivity className="size-3.5 shrink-0" />
                      <span>Loading Forge activity…</span>
                    </div>
                  )}
                  {!activityLoading && activityEntries.length === 0 && (
                    <p className="text-foreground-alt/60 text-xs leading-relaxed">
                      No recent activity yet. Activity will appear as Forge work
                      changes; no action is required.
                    </p>
                  )}
                  {activityEntries.slice(0, 3).map((entry) => (
                    <div
                      key={entry.id}
                      className="border-foreground/6 flex items-start gap-2 border-b py-2 last:border-b-0 last:pb-0"
                    >
                      <LuActivity className="text-foreground-alt/60 mt-0.5 size-3 shrink-0" />
                      <div className="min-w-0">
                        <div className="text-foreground text-xs font-medium">
                          {entry.title}
                        </div>
                        <div className="text-foreground-alt/50 truncate text-xs">
                          {entry.detail}
                        </div>
                      </div>
                    </div>
                  ))}
                </InfoCard>
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
            {worldError ? (
              <LoadingCard
                view={{
                  state: 'error',
                  title: 'Forge activity unavailable',
                  detail: 'This dashboard could not read recent Forge work.',
                  error: 'Try again to reload Forge activity.',
                  onRetry: worldState.retry,
                }}
              />
            ) : activityLoading && activityEntries.length === 0 ? (
              <LoadingCard
                view={{
                  state: 'loading',
                  title: 'Loading Forge activity…',
                  detail: 'Reading recent jobs, tasks, and executions.',
                  progressIndeterminate: true,
                }}
              />
            ) : !activityLoading && activityEntries.length === 0 ? (
              <InfoCard
                icon={
                  <LuActivity className="text-foreground-alt/70 size-3.5" />
                }
                title="No recent activity yet"
              >
                <p className="text-foreground-alt/60 text-xs leading-relaxed">
                  Activity will appear as Forge work changes; no action is
                  required.
                </p>
              </InfoCard>
            ) : null}
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
                      <LuActivity className="text-foreground-alt/60 size-3 shrink-0" />
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
                  <LuActivity className="text-foreground-alt/60 mt-0.5 size-3 shrink-0" />
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
          <div className="space-y-2">
            {worldError ? (
              <LoadingCard
                view={{
                  state: 'error',
                  title: 'Forge entities unavailable',
                  detail:
                    'This dashboard could not read its linked Forge work.',
                  error: 'Try again to reload Forge entities.',
                  onRetry: worldState.retry,
                }}
              />
            ) : entitiesLoading && entities.length === 0 ? (
              <LoadingCard
                view={{
                  state: 'loading',
                  title: 'Loading Forge entities…',
                  detail: 'Reading linked Jobs, Workers, and Clusters.',
                  progressIndeterminate: true,
                }}
              />
            ) : !entitiesLoading && entities.length === 0 ? (
              <InfoCard
                icon={<LuBox className="text-foreground-alt/70 size-3.5" />}
                title="No linked Forge entities"
              >
                <p className="text-foreground-alt/60 text-xs leading-relaxed">
                  Create a Job or Cluster to add Forge entities to this
                  dashboard.
                </p>
              </InfoCard>
            ) : null}
            {entities.map((entity) => (
              <ForgeEntityLink
                key={entity.objectKey}
                objectKey={entity.objectKey}
                icon={
                  <LuBox className="text-foreground-alt/60 size-3 shrink-0" />
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
                <span className="text-sm font-medium">{entity.objectKey}</span>
              </ForgeEntityLink>
            ))}
          </div>
        ),
      },
      {
        id: 'bindings',
        label: 'Bindings',
        content: (
          <div className="space-y-2">
            {bindingsError ? (
              <LoadingCard
                view={{
                  state: 'error',
                  title: 'Process bindings unavailable',
                  detail: 'Forge could not read worker approval state.',
                  error: 'Try again to refresh process bindings.',
                  onRetry: contentsState.retry,
                }}
              />
            ) : bindingsLoading ? (
              <LoadingCard
                view={{
                  state: 'loading',
                  title: 'Loading process bindings…',
                  detail: 'Reading worker approval state for this Space.',
                  progressIndeterminate: true,
                }}
              />
            ) : bindings.length > 0 ? (
              <ProcessBindingList
                bindings={bindings}
                onToggle={(bindingObjectKey, approved) => {
                  void handleToggle(bindingObjectKey, approved)
                }}
              />
            ) : (
              <InfoCard
                icon={<LuBox className="text-foreground-alt/70 size-3.5" />}
                title="No process bindings"
              >
                <p className="text-foreground-alt/60 text-xs leading-relaxed">
                  Bindings appear when Forge workers are linked; no action is
                  required yet.
                </p>
              </InfoCard>
            )}
          </div>
        ),
      },
    ],
    [
      activityEntries,
      activityLoading,
      bindings,
      bindingsError,
      bindingsLoading,
      contentsState.retry,
      entities,
      entitiesLoading,
      worldError,
      worldState.retry,
      handleStartWorkers,
      handleToggle,
      openingAction,
      summaryMetrics,
      typeCounts,
      pendingWorkers.length,
    ],
  )

  return (
    <ForgeViewerShell
      icon={<LuLayoutDashboard className="size-4" />}
      title={dashboard?.name || 'Forge Dashboard'}
      tabs={tabs}
      headerActions={headerActions}
      headerStatus={
        creationError ? (
          <div
            className="border-destructive/15 bg-destructive/5 text-destructive shrink-0 border-b px-4 py-2 text-xs leading-relaxed"
            role="alert"
          >
            {creationError}
          </div>
        ) : undefined
      }
      stateKey={objectKey}
    />
  )
}
