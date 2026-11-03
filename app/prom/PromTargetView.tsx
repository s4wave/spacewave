import type { ReactNode } from 'react'
import { LuActivity } from '@s4wave/web/images/icons.js'

import type { TargetInfo, TargetStatus } from '@s4wave/sdk/prom/prom.pb.js'

import { Button } from '@s4wave/web/ui/button.js'

import {
  formatAgo,
  formatClock,
  formatLabels,
  targetName,
} from './prom-view.js'
import type { TargetAction, TargetState } from './target-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeLayout['--text-metadata'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 'var(--text-metadata--line-height)',
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 15%, transparent)`,
    color: themeColors['--color-success'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    columnGap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'grid',
    fontSize: themeFonts['--text-xs'],
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    rowGap: themeLayout['--spacing'],
    maxWidth: 'var(--container-md)',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule7: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: { color: themeColors['--color-foreground-alt'] },
  rule9: { color: themeColors['--color-foreground'], wordBreak: 'break-all' },
  rule10: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// actionLabels are the approval buttons' labels.
const actionLabels: Record<Exclude<TargetAction, null>, string> = {
  'scrape-here': 'Scrape from this Device',
  'take-over': 'Scrape from this Device instead',
  stop: 'Stop scraping',
}

export interface PromTargetViewProps {
  objectKey: string
  info: TargetInfo | undefined
  state: TargetState
  action: TargetAction
  // deviceName names the Device of a peer ID.
  deviceName: (peerId: string) => string
  nowMs: number
  saving: boolean
  // saveError is the reason the last approval write failed.
  saveError: string | null
  onAction: () => void
  // renderChart applies section spacing at the chart's physical root.
  renderChart?: (sx?: stylex.StyleXStyles) => ReactNode
}

// PromTargetView shows a Scrape target's configuration, its scrape state,
// and the approval action this member can take on this Device.
export function PromTargetView({
  objectKey,
  info,
  state,
  action,
  deviceName,
  nowMs,
  saving,
  saveError,
  onAction,
  renderChart,
}: PromTargetViewProps) {
  const scrape = info?.meta?.scrape
  const intervalMs = scrape?.intervalMs || 15_000
  const retentionMs = Number(info?.meta?.retentionMs ?? 0n) || 15 * 86_400_000
  return (
    <div sx={stylexStyles2.rule1}>
      <header sx={stylexStyles2.rule2}>
        <LuActivity sx={stylexStyles.rule1} aria-hidden />
        <span sx={stylexStyles2.rule3}>{targetName(objectKey)}</span>
        <StateBadge state={state} />
      </header>

      <div sx={stylexStyles4.rule1}>
        <p sx={[targetSectionStyles.spacing, stylexStyles2.rule4]}>
          {describeState(state, nowMs, deviceName)}
        </p>

        {action ? (
          <ActionRow
            sx={targetSectionStyles.spacing}
            action={action}
            saving={saving}
            saveError={saveError}
            onAction={onAction}
          />
        ) : null}

        {state.state === 'up' || state.state === 'down' ? (
          <ScrapeFields
            sx={targetSectionStyles.spacing}
            status={state.status}
            deviceName={deviceName}
          />
        ) : null}

        {renderChart?.(targetSectionStyles.spacing)}

        <dl sx={[targetSectionStyles.spacing, stylexStyles2.rule5]}>
          <Field label="URL" mono>
            {scrape?.url || 'none: stores only samples written to it'}
          </Field>
          <Field label="Every">{intervalMs / 1000} s</Field>
          <Field label="Labels" mono>
            {formatLabels(info?.meta?.labels)}
          </Field>
          <Field label="Keeps">
            {Math.round(retentionMs / 86_400_000)} days
          </Field>
        </dl>
      </div>
    </div>
  )
}

// describeState writes the sentence for the target's state.
function describeState(
  state: TargetState,
  nowMs: number,
  deviceName: (peerId: string) => string,
): string {
  switch (state.state) {
    case 'unapproved':
      return 'No Device scrapes this target yet.'
    case 'waiting':
      return 'Waiting for the first scrape from this Device.'
    case 'up':
      return `Up. Last scraped ${formatAgo(Number(state.status.lastScrapeMs ?? 0n), nowMs)} ago.`
    case 'down':
      return `Down: ${state.status.lastError || 'the scrape failed'}.`
    case 'stale':
      return `No scrape since ${formatClock(Number(state.status.lastScrapeMs ?? 0n))}. ${deviceName(state.status.devicePeerId ?? '')} may be offline.`
  }
}

// ActionRow is the approval button and the last write's failure.
function ActionRow({
  sx,
  action,
  saving,
  saveError,
  onAction,
}: {
  action: Exclude<TargetAction, null>
  saving: boolean
  saveError: string | null
  onAction: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule6, sx]}>
      <Button
        size="sm"
        variant={action === 'stop' ? 'outline' : 'default'}
        disabled={saving}
        onClick={onAction}
      >
        {saveError ? 'Retry' : actionLabels[action]}
      </Button>
      {saveError ? (
        <span sx={stylexStyles2.rule7}>Could not save: {saveError}.</span>
      ) : null}
    </div>
  )
}

// ScrapeFields lists the last scrape's Device, time, duration and series.
function ScrapeFields({
  sx,
  status,
  deviceName,
}: {
  status: TargetStatus
  deviceName: (peerId: string) => string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <dl sx={[stylexStyles2.rule5, sx]}>
      <Field label="Device">{deviceName(status.devicePeerId ?? '')}</Field>
      <Field label="Last scrape">
        {formatClock(Number(status.lastScrapeMs ?? 0n))}
      </Field>
      <Field label="Duration">{status.lastDurationMs ?? 0} ms</Field>
      <Field label="Series">{String(status.series ?? 0n)}</Field>
    </dl>
  )
}

// StateBadge labels the state with an icon-free word and a status color.
function StateBadge({ state }: { state: TargetState }) {
  const labels: Record<TargetState['state'], string> = {
    unapproved: 'Unapproved',
    waiting: 'Waiting',
    up: 'Up',
    down: 'Down',
    stale: 'Stale',
  }
  return (
    <span
      // Keep the metadata font role separate from conditional tone utilities.
      sx={[
        stylexStyles3.rule1,
        state.state === 'up' && stylexStyles3.rule2,
        state.state === 'down' && stylexStyles3.rule3,
        state.state === 'stale' && stylexStyles3.rule4,
        !!(state.state === 'unapproved' || state.state === 'waiting') &&
          stylexStyles3.rule5,
      ]}
    >
      {labels[state.state]}
    </span>
  )
}

// Field is one term and its value.
function Field({
  label,
  mono,
  children,
}: {
  label: string
  mono?: boolean
  children: ReactNode
}) {
  return (
    <>
      <dt sx={stylexStyles2.rule8}>{label}</dt>
      <dd sx={[stylexStyles2.rule9, !!mono && stylexStyles2.rule10]}>
        {children}
      </dd>
    </>
  )
}

// targetSectionStyles applies spacing to each rendered target section.
const targetSectionStyles = stylex.create({
  spacing: {
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': 'calc(var(--spacing) * 4)',
    },
    marginBlockStart: { default: null, ':not(:last-child)': 0 },
  },
})
