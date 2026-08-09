import { useCallback, useMemo, useState } from 'react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import { ForgeDashboard } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import type { ProcessBindingInfo } from '@s4wave/sdk/space/space.pb.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { SpaceContentsContext } from '@s4wave/web/contexts/contexts.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import { useForgeLinkedEntities } from '@s4wave/web/forge/useForgeLinkedEntities.js'
import { PRED_DASHBOARD_FORGE_REF } from '@s4wave/web/forge/predicates.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { useVisibleObjectWizardTypeSet } from '../space/useVisibleObjectWizardTypeSet.js'
import { useForgeDashboardActivity } from './useForgeDashboardActivity.js'

export const ForgeDashboardTypeID = 'spacewave/forge/dashboard'
export const forgeEntityTypeLabels: Record<string, string> = {
  'forge/task': 'TASK',
  'forge/job': 'JOB',
  'forge/cluster': 'CLUSTER',
  'forge/worker': 'WORKER',
  'forge/pass': 'PASS',
  'forge/execution': 'EXECUTION',
}

export function useForgeDashboardViewerController({
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
  const typeCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const entity of entities) {
      const label = forgeEntityTypeLabels[entity.typeId] ?? entity.typeId
      counts[label] = (counts[label] ?? 0) + 1
    }
    return counts
  }, [entities])
  const summaryMetrics = useMemo(
    () => [
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
      {
        label: 'Active work',
        count: activityEntries.filter((entry) =>
          /\b(PENDING|RUNNING|CHECKING|RETRY)\b/.test(entry.title),
        ).length,
      },
    ],
    [activityEntries, entities],
  )

  const contentsResource = SpaceContentsContext.useContext()
  const contents = useResourceValue(contentsResource)
  const contentsState = useStreamingResource(
    contentsResource,
    useCallback((value, signal) => value.watchState({}, signal), []),
    [],
  )
  const bindings: ProcessBindingInfo[] = useMemo(
    () => contentsState.value?.processBindings ?? [],
    [contentsState.value?.processBindings],
  )
  const bindingsLoading = contentsState.loading && bindings.length === 0
  const bindingsByObjectKey = useMemo(
    () =>
      new Map(bindings.map((binding) => [binding.objectKey ?? '', binding])),
    [bindings],
  )
  const pendingWorkers = useMemo(
    () =>
      entities.filter(
        (entity) =>
          entity.typeId === 'forge/worker' &&
          !(bindingsByObjectKey.get(entity.objectKey)?.approved ?? false),
      ),
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
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map(
        (object) => object.objectKey ?? '',
      ) ?? [],
    [spaceState.worldContents?.objects],
  )

  const openWizard = useCallback(
    async (
      wizardTypeId: string,
      targetTypeId: string,
      targetKeyPrefix: string,
      name: string,
      opts?: { initialStep?: number; initialConfigData?: Uint8Array },
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
  const toggleBinding = useCallback(
    async (bindingObjectKey: string, approved: boolean) => {
      if (!contents) return
      const binding = bindings.find(
        (item) => item.objectKey === bindingObjectKey,
      )
      await contents.setProcessBinding(
        bindingObjectKey,
        binding?.typeId ?? '',
        approved,
      )
    },
    [bindings, contents],
  )
  const startWorkers = useCallback(async () => {
    if (!contents) return
    await Promise.all(
      pendingWorkers.map((worker) =>
        contents.setProcessBinding(worker.objectKey, worker.typeId ?? '', true),
      ),
    )
  }, [contents, pendingWorkers])
  const createCluster = useCallback(async () => {
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
  const createJob = useCallback(async () => {
    setCreationError('')
    setOpeningAction('job')
    try {
      const clusterKey =
        clusterEntities.length === 1
          ? (clusterEntities[0]?.objectKey ?? '')
          : ''
      const initialConfigData = ForgeJobCreateOp.toBinary({
        jobKey: '',
        clusterKey,
        taskDefs: [],
        timestamp: new Date(),
      })
      await openWizard('wizard/forge/job', 'forge/job', 'forge/job/', 'Job', {
        initialStep: clusterKey ? 1 : 0,
        initialConfigData,
      })
    } catch {
      const message = 'Job creation is unavailable. Try again.'
      setCreationError(message)
      toast.error(message)
    } finally {
      setOpeningAction(null)
    }
  }, [clusterEntities, openWizard])

  return {
    objectKey,
    dashboard,
    entities,
    entitiesLoading,
    activityEntries,
    activityLoading,
    worldError,
    retryWorld: worldState.retry,
    typeCounts,
    summaryMetrics,
    bindings,
    bindingsLoading,
    bindingsError: contentsState.error,
    retryBindings: contentsState.retry,
    pendingWorkers,
    openingAction,
    creationError,
    canCreateCluster: visibleWizardTypeSet.has('forge/cluster'),
    canCreateJob: visibleWizardTypeSet.has('forge/job'),
    createCluster,
    createJob,
    startWorkers,
    toggleBinding,
  }
}

export type ForgeDashboardViewerController = ReturnType<
  typeof useForgeDashboardViewerController
>
