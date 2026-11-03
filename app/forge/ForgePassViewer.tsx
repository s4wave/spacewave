import { useMemo } from 'react'
import { LuActivity, LuPlay } from '@s4wave/web/images/icons.js'

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
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ForgeValueSetPanels } from './ForgeValueSetDisplay.js'
import { useForgeDecodedLinkedEntities } from './useForgeDecodedLinkedEntities.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: { minWidth: 0 },
  rule4: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
  const pass = useForgeBlockData(objectState, ForgePassTypeID, Pass)

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
          <div sx={false}>
            <InfoCard sx={nativeChildSpacing.s3}>
              <div sx={false}>
                {pass?.peerId && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Peer ID"
                    value={pass.peerId}
                  />
                )}
                {pass?.timestamp && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Created"
                    value={pass.timestamp.toISOString()}
                  />
                )}
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Pass Nonce"
                  value={pass?.passNonce?.toString() ?? '0'}
                />
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
                { label: 'Replicas', value: pass?.replicas ?? 0 },
                {
                  label: 'Executions',
                  value: decodedExecutionsLoading
                    ? '-'
                    : decodedExecutions.length,
                },
              ]}
            />
            <InfoCard sx={nativeChildSpacing.s3} title="Result">
              <div sx={stylexStyles3.rule1}>{describeResult(pass?.result)}</div>
            </InfoCard>
          </div>
        ),
      },
      {
        id: 'executions',
        label: 'Executions',
        content:
          decodedExecutions.length === 0 ? (
            <ForgeEntityList
              entities={executions}
              loading={executionsLoading || decodedExecutionsLoading}
              icon={<LuActivity sx={stylexStyles.rule1} />}
              loadingLabel="Loading executions..."
              emptyLabel="No executions yet"
            />
          ) : (
            <div sx={false}>
              {decodedExecutions.map((execution) => (
                <div
                  key={execution.entity.objectKey}
                  sx={[nativeChildSpacingNext.s2, stylexStyles4.rule1]}
                >
                  <div sx={[nativeChildSpacing.s2, stylexStyles3.rule2]}>
                    <div sx={stylexStyles3.rule3}>
                      <ForgeEntityLink
                        objectKey={execution.entity.objectKey}
                        sx={stylexStyles2.rule1}
                      />
                      <div sx={stylexStyles3.rule4}>
                        {execution.data.peerId || 'Worker pending'}
                      </div>
                    </div>
                    <StateBadge
                      state={execution.data.executionState ?? 0}
                      labels={executionStateLabels}
                    />
                  </div>
                  <div sx={[nativeChildSpacing.s2, stylexStyles3.rule5]}>
                    <span>
                      {execution.data.timestamp?.toISOString() ??
                        'No timestamp'}
                    </span>
                    <span>{describeResult(execution.data.result)}</span>
                  </div>
                </div>
              ))}
            </div>
          ),
      },
      {
        id: 'details',
        label: 'Details',
        content: (
          <div sx={false}>
            <ForgeValueSetPanels
              sx={nativeChildSpacingNext.s3}
              valueSet={pass?.valueSet}
              emptyInputsLabel="No pass inputs recorded"
              emptyOutputsLabel="No pass outputs recorded"
            />
            <InfoCard sx={nativeChildSpacingNext.s3} title="Execution Snapshot">
              <div sx={stylexStyles3.rule5}>
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
      stateKey={objectKey}
      icon={<LuPlay sx={stylexStyles.rule2} />}
      title="Pass"
      state={pass?.passState ?? 0}
      stateLabels={passStateLabels}
      tabs={tabs}
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
})
