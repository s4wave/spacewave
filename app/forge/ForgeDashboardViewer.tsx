import { forgeControlStyles } from '@s4wave/web/style/forge-controls.stylex.js'
/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuLayoutDashboard,
  LuBox,
  LuActivity,
  LuBriefcase,
  LuPlus,
  LuServer,
} from '@s4wave/web/images/icons.js'

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
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(4, minmax(0, 1fr))',
    },
  },
  rule4: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: '10px',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    marginBottom: themeLayout['--spacing'],
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-numeric-spacing': 'tabular-nums',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule7: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    display: 'inline-flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'space-between',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-numeric-spacing': 'tabular-nums',
    color: themeColors['--color-foreground'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule13: { minWidth: 0 },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule16: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
  },
  rule17: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule18: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
  },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule20: {
    '--tw-leading': 'var(--leading-relaxed)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
  useObjectName(dashboard?.name)
  const { navigateToObjects, spaceWorld } = SpaceContainerContext.useContext()
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
      const wizardKey = await buildWizardObjectKey(spaceWorld, name)
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
    [navigateToObjects, spaceWorld],
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
        'cluster',
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
        icon: <LuBriefcase sx={stylexStyles.rule1} />,
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
        icon: <LuServer sx={stylexStyles.rule1} />,
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
          <div sx={false}>
            {openingAction && (
              <div
                role="status"
                sx={[nativeChildSpacing.s3, stylexStyles2.rule1]}
              >
                Opening Forge {openingAction}…
              </div>
            )}

            {worldError ? (
              <LoadingCard
                sx={nativeChildSpacing.s3}
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
                  sx={nativeChildSpacing.s3}
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
                sx={nativeChildSpacing.s3}
                icon={<LuLayoutDashboard sx={stylexStyles.rule2} />}
                title="No Forge work yet"
              >
                <p sx={stylexStyles2.rule2}>
                  Create a Job to define work, or create a Cluster to attach
                  execution capacity.
                </p>
              </InfoCard>
            )}

            {entities.length > 0 && (
              <div sx={[nativeChildSpacing.s3, stylexStyles2.rule3]}>
                {summaryMetrics.map((metric) => (
                  <div key={metric.label} sx={stylexStyles2.rule4}>
                    <div sx={stylexStyles2.rule5}>{metric.label}</div>
                    <div sx={stylexStyles2.rule6}>{metric.count}</div>
                  </div>
                ))}
              </div>
            )}

            {pendingWorkers.length > 0 && (
              <InfoCard
                sx={nativeChildSpacing.s3}
                icon={<LuPlus sx={stylexStyles.rule2} />}
                title={
                  pendingWorkers.length === 1
                    ? 'Worker ready to start'
                    : 'Workers ready to start'
                }
              >
                <p sx={stylexStyles2.rule2}>
                  Approve the quickstart worker process binding to start task
                  execution in this session.
                </p>
                <button
                  type="button"
                  onClick={() => {
                    void handleStartWorkers()
                  }}
                  sx={[stylexStyles2.rule7, forgeControlStyles.required]}
                >
                  <LuPlus sx={stylexStyles.rule1} />
                  {pendingWorkers.length === 1
                    ? 'Start worker'
                    : 'Start workers'}
                </button>
              </InfoCard>
            )}

            {entities.length > 0 && (
              <div sx={[nativeChildSpacing.s3, stylexStyles2.rule8]}>
                <InfoCard
                  icon={<LuBox sx={stylexStyles.rule3} />}
                  title="Entity health"
                >
                  <div sx={false}>
                    {Object.entries(typeCounts).map(([label, count]) => (
                      <div
                        key={label}
                        sx={[nativeChildSpacing.s1_5, stylexStyles2.rule9]}
                      >
                        <span>{label}</span>
                        <span sx={stylexStyles2.rule10}>{count} linked</span>
                      </div>
                    ))}
                  </div>
                </InfoCard>

                <InfoCard
                  icon={<LuActivity sx={stylexStyles.rule3} />}
                  title="Recent activity"
                >
                  {activityLoading && activityEntries.length === 0 && (
                    <div sx={stylexStyles2.rule11}>
                      <LuActivity sx={stylexStyles.rule4} />
                      <span>Loading Forge activity…</span>
                    </div>
                  )}
                  {!activityLoading && activityEntries.length === 0 && (
                    <p sx={stylexStyles2.rule12}>
                      No recent activity yet. Activity will appear as Forge work
                      changes; no action is required.
                    </p>
                  )}
                  {activityEntries.slice(0, 3).map((entry) => (
                    <div key={entry.id} sx={activityRowStyles.row}>
                      <LuActivity sx={stylexStyles.rule5} />
                      <div sx={stylexStyles2.rule13}>
                        <div sx={stylexStyles2.rule14}>{entry.title}</div>
                        <div sx={stylexStyles2.rule15}>{entry.detail}</div>
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
          <div sx={false}>
            {worldError ? (
              <LoadingCard
                sx={nativeChildSpacing.s2}
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
                sx={nativeChildSpacing.s2}
                view={{
                  state: 'loading',
                  title: 'Loading Forge activity…',
                  detail: 'Reading recent jobs, tasks, and executions.',
                  progressIndeterminate: true,
                }}
              />
            ) : !activityLoading && activityEntries.length === 0 ? (
              <InfoCard
                sx={nativeChildSpacing.s2}
                icon={<LuActivity sx={stylexStyles.rule3} />}
                title="No recent activity yet"
              >
                <p sx={stylexStyles2.rule12}>
                  Activity will appear as Forge work changes; no action is
                  required.
                </p>
              </InfoCard>
            ) : null}
            {activityEntries.map((entry) => {
              const content = (
                <div sx={stylexStyles2.rule16}>
                  <div sx={stylexStyles2.rule14}>{entry.title}</div>
                  <div sx={stylexStyles2.rule15}>{entry.detail}</div>
                  <div sx={stylexStyles2.rule17}>
                    {entry.timestamp.toISOString()}
                  </div>
                </div>
              )

              if (entry.objectKey) {
                return (
                  <ForgeEntityLink
                    sx={nativeChildSpacing.s2}
                    key={entry.id}
                    objectKey={entry.objectKey}
                    icon={<LuActivity sx={stylexStyles.rule6} />}
                  >
                    {content}
                  </ForgeEntityLink>
                )
              }

              return (
                <div
                  key={entry.id}
                  sx={[nativeChildSpacing.s2, stylexStyles2.rule18]}
                >
                  <LuActivity sx={stylexStyles.rule5} />
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
          <div sx={false}>
            {worldError ? (
              <LoadingCard
                sx={nativeChildSpacing.s2}
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
                sx={nativeChildSpacing.s2}
                view={{
                  state: 'loading',
                  title: 'Loading Forge entities…',
                  detail: 'Reading linked Jobs, Workers, and Clusters.',
                  progressIndeterminate: true,
                }}
              />
            ) : !entitiesLoading && entities.length === 0 ? (
              <InfoCard
                sx={nativeChildSpacing.s2}
                icon={<LuBox sx={stylexStyles.rule3} />}
                title="No linked Forge entities"
              >
                <p sx={stylexStyles2.rule12}>
                  Create a Job or Cluster to add Forge entities to this
                  dashboard.
                </p>
              </InfoCard>
            ) : null}
            {entities.map((entity) => (
              <ForgeEntityLink
                sx={nativeChildSpacing.s2}
                key={entity.objectKey}
                objectKey={entity.objectKey}
                icon={<LuBox sx={stylexStyles.rule6} />}
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
                <span sx={stylexStyles2.rule19}>{entity.objectKey}</span>
              </ForgeEntityLink>
            ))}
          </div>
        ),
      },
      {
        id: 'bindings',
        label: 'Bindings',
        content: (
          <div sx={false}>
            {bindingsError ? (
              <LoadingCard
                sx={nativeChildSpacingNext.s2}
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
                sx={nativeChildSpacingNext.s2}
                view={{
                  state: 'loading',
                  title: 'Loading process bindings…',
                  detail: 'Reading worker approval state for this Space.',
                  progressIndeterminate: true,
                }}
              />
            ) : bindings.length > 0 ? (
              <ProcessBindingList
                sx={nativeChildSpacingNext.s2}
                bindings={bindings}
                onToggle={(bindingObjectKey, approved) => {
                  void handleToggle(bindingObjectKey, approved)
                }}
              />
            ) : (
              <InfoCard
                sx={nativeChildSpacingNext.s2}
                icon={<LuBox sx={stylexStyles.rule3} />}
                title="No process bindings"
              >
                <p sx={stylexStyles2.rule12}>
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
      icon={<LuLayoutDashboard sx={stylexStyles.rule7} />}
      title={dashboard?.name || 'Forge Dashboard'}
      tabs={tabs}
      headerActions={headerActions}
      headerStatus={
        creationError ? (
          <div sx={stylexStyles2.rule20} role="alert">
            {creationError}
          </div>
        ) : undefined
      }
      stateKey={objectKey}
    />
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
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
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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
})

// activityRowStyles preserves the final activity row's shorter bottom inset.
const activityRowStyles = stylex.create({
  row: {
    borderColor: 'color-mix(in oklab, var(--color-foreground) 6%, transparent)',
    gap: 'calc(var(--spacing) * 2)',
    alignItems: 'flex-start',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: { default: '1px', ':last-child': 0 },
    paddingBottom: { default: 'calc(var(--spacing) * 2)', ':last-child': 0 },
    paddingTop: 'calc(var(--spacing) * 2)',
  },
})
