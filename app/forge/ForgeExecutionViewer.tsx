import { useEffect, useMemo, useRef } from 'react'
import { LuActivity } from '@s4wave/web/images/icons.js'

import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { color: themeColors['--color-destructive'] },
  rule2: { color: 'var(--color-amber-300)' },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule4: { color: themeColors['--color-foreground'] },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule4: {
    padding: themeLayout['--spacing'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    maxHeight: `calc(${themeLayout['--spacing']} * 64)`,
    overflowY: 'auto',
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule7: {
    flex: 1,
    color: themeColors['--color-foreground'],
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-all',
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

export const ForgeExecutionTypeID = 'forge/execution'

const execStateLabels: Record<number, string> = {
  [State.ExecutionState_UNKNOWN]: 'UNKNOWN',
  [State.ExecutionState_PENDING]: 'PENDING',
  [State.ExecutionState_RUNNING]: 'RUNNING',
  [State.ExecutionState_COMPLETE]: 'COMPLETE',
}

// logLevelStyles returns a text color class for a log level.
function logLevelStyles(level: string): stylex.StyleXStyles {
  switch (level) {
    case 'error':
      return stylexStyles3.rule1
    case 'warn':
      return stylexStyles3.rule2
    case 'debug':
      return stylexStyles3.rule3
    default:
      return stylexStyles3.rule4
  }
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

function ExecutionTimestamp({ timestamp }: { timestamp?: Date }) {
  if (!timestamp) {
    return <span sx={stylexStyles2.rule1}>Not started</span>
  }

  return <span sx={stylexStyles2.rule2}>{timestamp.toISOString()}</span>
}

// LogViewer renders execution log entries with auto-scroll.
function LogViewer({ logEntries }: { logEntries: Execution['logEntries'] }) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const entries = logEntries ?? []

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [entries.length])

  if (entries.length === 0) {
    return (
      <div sx={stylexStyles2.rule3}>
        <div sx={stylexStyles2.rule4}>
          <LuActivity sx={stylexStyles.rule1} />
          <span>No log output</span>
        </div>
      </div>
    )
  }

  return (
    <div ref={scrollRef} sx={stylexStyles2.rule5}>
      {entries.map((entry) => (
        <div
          key={`${entry.level ?? ''}:${entry.message}`}
          sx={stylexStyles2.rule6}
        >
          {entry.level && (
            <span sx={logLevelStyles(entry.level)}>
              [{entry.level.toUpperCase()}]
            </span>
          )}
          <span sx={stylexStyles2.rule7}>{entry.message}</span>
        </div>
      ))}
    </div>
  )
}

// ForgeExecutionViewer displays a Forge Execution entity.
export function ForgeExecutionViewer({
  objectInfo,
  objectState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const execution = useForgeBlockData(
    objectState,
    ForgeExecutionTypeID,
    Execution,
  )

  const tabs: ForgeViewerTab[] = useMemo(
    () => [
      {
        id: 'logs',
        label: 'Logs',
        content: <LogViewer logEntries={execution?.logEntries} />,
      },
      {
        id: 'inputs',
        label: 'Inputs',
        content: (
          <div sx={false}>
            <ForgeValueSetDisplay
              sx={nativeChildSpacingNext.s3}
              title="Inputs"
              values={execution?.valueSet?.inputs}
              emptyLabel="No inputs recorded"
            />
          </div>
        ),
      },
      {
        id: 'outputs',
        label: 'Outputs',
        content: (
          <div sx={false}>
            <ForgeValueSetDisplay
              sx={nativeChildSpacingNext.s3}
              title="Outputs"
              values={execution?.valueSet?.outputs}
              emptyLabel="No outputs recorded"
            />
          </div>
        ),
      },
      {
        id: 'details',
        label: 'Details',
        content: (
          <div sx={false}>
            <InfoCard sx={nativeChildSpacing.s3}>
              <div sx={false}>
                {execution?.peerId && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Worker Peer ID"
                    value={execution.peerId}
                  />
                )}
                {execution?.timestamp && (
                  <CopyableField
                    sx={nativeChildSpacing.s2}
                    label="Created"
                    value={execution.timestamp.toISOString()}
                  />
                )}
                <CopyableField
                  sx={nativeChildSpacing.s2}
                  label="Object Key"
                  value={objectKey}
                />
              </div>
            </InfoCard>
            <InfoCard sx={nativeChildSpacing.s3} title="Runtime">
              <div sx={stylexStyles2.rule8}>
                <div sx={stylexStyles2.rule1}>Started</div>
                <ExecutionTimestamp timestamp={execution?.timestamp} />
              </div>
              <div sx={stylexStyles2.rule9}>
                <span>{execution?.logEntries?.length ?? 0} log entries</span>
                <span>{execution?.valueSet?.inputs?.length ?? 0} inputs</span>
                <span>{execution?.valueSet?.outputs?.length ?? 0} outputs</span>
              </div>
            </InfoCard>
            <InfoCard sx={nativeChildSpacing.s3} title="Result">
              <div sx={stylexStyles2.rule2}>
                {describeResult(execution?.result)}
              </div>
            </InfoCard>
          </div>
        ),
      },
    ],
    [execution, objectKey],
  )

  return (
    <ForgeViewerShell
      stateKey={objectKey}
      icon={<LuActivity sx={stylexStyles.rule2} />}
      title="Execution"
      state={execution?.executionState ?? 0}
      stateLabels={execStateLabels}
      headerStatus={
        execution?.waitingPluginId && (
          <div sx={stylexStyles2.rule10}>
            Waiting for plugin {execution.waitingPluginId} to load
          </div>
        )
      }
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
