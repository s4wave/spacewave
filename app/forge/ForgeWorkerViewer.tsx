import { useCallback, useMemo } from 'react'
import { LuActivity, LuCpu } from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { State as ExecutionState } from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'
import { Worker } from '@go/github.com/s4wave/spacewave/forge/worker/worker.pb.js'
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
import { PRED_CLUSTER_TO_WORKER } from '@s4wave/web/forge/predicates.js'
import { StateBadge } from '@s4wave/web/forge/StateBadge.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { useForgeClusterSnapshot } from './useForgeClusterSnapshot.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

import { motion as themeMotion } from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
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

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: { minWidth: 0 },
  rule7: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
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

const stylexStyles2 = stylex.create({
  rule1: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
  const worker = useForgeBlockData(objectState, ForgeWorkerTypeID, Worker)
  useObjectName(worker?.name)

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
  const peerIdSet = useMemo(() => new Set(peerIds), [peerIds])
  const executions = useMemo(
    () =>
      snapshot.executions.filter((execution) =>
        peerIdSet.has(execution.data.peerId ?? ''),
      ),
    [peerIdSet, snapshot.executions],
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
        icon: <LuActivity sx={stylexStyles.rule1} />,
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
          <div sx={false}>
            <InfoCard sx={nativeChildSpacing.s3}>
              <div sx={false}>
                {worker?.name && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Name"
                    value={worker.name}
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
                  label: 'Clusters',
                  value: clustersLoading ? '-' : clusters.length,
                },
                {
                  label: 'Active executions',
                  value: activeExecutions.length,
                  unit: `of ${Math.max(peerIds.length, 1)}`,
                  detail: 'one per peer',
                },
              ]}
            />
            <InfoCard sx={nativeChildSpacing.s3} title="Peer IDs">
              {snapshotLoading && (
                <div sx={stylexStyles3.rule1}>Loading worker identities…</div>
              )}
              {!snapshotLoading && peerIds.length === 0 && (
                <div sx={stylexStyles3.rule1}>
                  No keypairs linked to this worker
                </div>
              )}
              {!snapshotLoading && peerIds.length > 0 && (
                <div sx={false}>
                  {peerIds.map((peerId) => (
                    <CopyableField
                      sx={nativeChildSpacing.s2}
                      key={peerId}
                      label="Peer ID"
                      value={peerId}
                    />
                  ))}
                </div>
              )}
              {binding && (
                <div sx={stylexStyles3.rule2}>
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
          <div sx={false}>
            {activeExecutions.length === 0 && (
              <div sx={[nativeChildSpacingNext.s2, stylexStyles3.rule3]}>
                <div sx={stylexStyles3.rule4}>
                  <LuActivity sx={stylexStyles.rule2} />
                  <span>No active executions assigned to this worker</span>
                </div>
              </div>
            )}
            {activeExecutions.map((execution) => (
              <div
                key={execution.objectKey}
                sx={[nativeChildSpacingNext.s2, stylexStyles4.rule1]}
              >
                <div sx={[nativeChildSpacing.s2, stylexStyles3.rule5]}>
                  <div sx={stylexStyles3.rule6}>
                    <ForgeEntityLink
                      objectKey={execution.objectKey}
                      sx={stylexStyles2.rule1}
                    />
                    <div sx={stylexStyles3.rule7}>
                      {execution.jobKey} / {execution.taskKey}
                    </div>
                  </div>
                  <StateBadge
                    state={execution.data.executionState ?? 0}
                    labels={executionStateLabels}
                  />
                </div>
                <div sx={[nativeChildSpacing.s2, stylexStyles3.rule8]}>
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
          <div sx={false}>
            {completeExecutions.length === 0 && (
              <div sx={[nativeChildSpacingNext.s2, stylexStyles3.rule3]}>
                <div sx={stylexStyles3.rule4}>
                  <LuActivity sx={stylexStyles.rule2} />
                  <span>No completed executions recorded yet</span>
                </div>
              </div>
            )}
            {completeExecutions.map((execution) => (
              <div
                key={execution.objectKey}
                sx={[nativeChildSpacingNext.s2, stylexStyles4.rule1]}
              >
                <div sx={[nativeChildSpacing.s2, stylexStyles3.rule5]}>
                  <div sx={stylexStyles3.rule6}>
                    <ForgeEntityLink
                      objectKey={execution.objectKey}
                      sx={stylexStyles2.rule1}
                    />
                    <div sx={stylexStyles3.rule7}>
                      {execution.jobKey} / {execution.taskKey}
                    </div>
                  </div>
                  <StateBadge
                    state={execution.data.executionState ?? 0}
                    labels={executionStateLabels}
                  />
                </div>
                <div sx={[nativeChildSpacing.s2, stylexStyles3.rule8]}>
                  <span>
                    {(execution.data.result?.success ?? false)
                      ? 'Success'
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
      stateKey={objectKey}
      icon={<LuCpu sx={stylexStyles.rule3} />}
      title={worker?.name || 'Worker'}
      tabs={tabs}
      actions={actions}
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
