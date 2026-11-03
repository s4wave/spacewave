import { useState, type ReactNode } from 'react'

import {
  PromPanelView,
  type PanelAnswer,
} from '@s4wave/app/prom/PromPanelView.js'
import { PromTargetView } from '@s4wave/app/prom/PromTargetView.js'
import type {
  TargetAction,
  TargetState,
} from '@s4wave/app/prom/target-state.js'
import {
  PanelKind,
  PanelUnit,
  ResultType,
  type Panel,
  type Series,
  type TargetInfo,
  type TargetStatus,
} from '@s4wave/sdk/prom/prom.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    overflow: 'auto',
    backgroundColor: themeColors['--color-background-primary'],
    height: '100%',
    width: '100%',
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: { height: `calc(${themeLayout['--spacing']} * 120)` },
  rule4: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule7: { width: `calc(${themeLayout['--spacing']} * 120)` },
  rule8: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: themeLayout['--spacing'],
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
})

// nowMs is the fixture's fixed clock, so every render draws the same frames.
const nowMs = Date.UTC(2026, 9, 24, 19, 30)
const hourMs = 60 * 60_000
const stepMs = 15_000
const panelWidth = 480

// cpuSeries builds a smooth mode series with a gap, to show broken lines.
function cpuSeries(mode: string, base: number, gap: boolean): Series {
  const samples = []
  for (let t = nowMs - hourMs; t <= nowMs; t += stepMs) {
    const i = (t - nowMs + hourMs) / stepMs
    if (gap && i > 120 && i < 140) continue
    samples.push({
      timeMs: BigInt(t),
      value: Math.max(0, base + 8 * Math.sin(i / 17 + base) + (i % 7)),
    })
  }
  return {
    labels: {
      labels: [
        { name: 'instance', value: 'forge-node' },
        { name: 'mode', value: mode },
      ],
    },
    samples,
  }
}

const chartPanel: Panel = {
  id: 'cpu',
  title: 'CPU by mode',
  expr: 'sum by (mode) (rate(node_cpu_seconds_total[5m])) * 100',
  unit: PanelUnit.PERCENT,
  kind: PanelKind.CHART,
}
const valuePanel: Panel = {
  id: 'mem',
  title: 'Memory available',
  expr: 'node_memory_MemAvailable_bytes',
  unit: PanelUnit.BYTES,
  kind: PanelKind.VALUE,
}

const answered = (
  series: Series[],
): Extract<PanelAnswer, { state: 'answered' }> => ({
  state: 'answered',
  resp: { resultType: ResultType.MATRIX, series },
  startMs: nowMs - hourMs,
  endMs: nowMs,
  stepMs,
  refreshing: false,
})

const modes = ['user', 'system', 'iowait']
const manyModes = [
  'user',
  'system',
  'iowait',
  'irq',
  'softirq',
  'steal',
  'nice',
  'guest',
  'idle-a',
  'idle-b',
]

const status: TargetStatus = {
  up: true,
  lastScrapeMs: BigInt(nowMs - 9_000),
  lastDurationMs: 42,
  series: 1873n,
  devicePeerId: '12D3KooWForgeDevicePeer7xQ9',
  commitWindowMs: 60_000,
}
const targetInfo: TargetInfo = {
  objectKey: 'prom/target/forge-node',
  meta: {
    labels: {
      labels: [
        { name: 'instance', value: 'forge-node' },
        { name: 'job', value: 'node' },
      ],
    },
    scrape: { url: 'http://127.0.0.1:9100/metrics', intervalMs: 15_000 },
  },
  status,
}

// targetCases are the target view's states with the action each offers.
const targetCases: {
  title: string
  state: TargetState
  action: TargetAction
  saveError?: string
}[] = [
  {
    title: 'Unapproved',
    state: { state: 'unapproved' },
    action: 'scrape-here',
  },
  { title: 'Waiting', state: { state: 'waiting' }, action: 'stop' },
  { title: 'Up', state: { state: 'up', status }, action: 'stop' },
  {
    title: 'Down',
    state: {
      state: 'down',
      status: { ...status, up: false, lastError: 'connection refused' },
    },
    action: 'stop',
  },
  {
    title: 'Stale, with a failed takeover',
    state: {
      state: 'stale',
      status: { ...status, lastScrapeMs: BigInt(nowMs - 25 * 60_000) },
    },
    action: 'take-over',
    saveError: 'permission denied',
  },
]

// PromViewerDebug renders the Dashboard panel and Scrape target views in
// each state of the viewer state sheet, on fixed data.
export function PromViewerDebug() {
  const [hoverMs, setHoverMs] = useState<number | null>(null)
  const panel = (
    p: Panel,
    answer: PanelAnswer,
    extra?: Partial<Parameters<typeof PromPanelView>[0]>,
  ) => (
    <PromPanelView
      panel={p}
      answer={answer}
      rangeLabel="1h"
      stale={[]}
      nowMs={nowMs}
      width={panelWidth}
      hoverMs={hoverMs}
      onHover={setHoverMs}
      {...extra}
    />
  )
  return (
    <div sx={stylexStyles.rule1}>
      <h1 sx={stylexStyles.rule2}>Metrics viewer states</h1>
      <Section title="Panels">
        <Case title="Loading">{panel(chartPanel, { state: 'loading' })}</Case>
        <Case title="Error">
          {panel(chartPanel, {
            state: 'error',
            message: 'parse error: unexpected right parenthesis',
          })}
        </Case>
        <Case title="No samples">{panel(chartPanel, answered([]))}</Case>
        <Case title="Chart with a gap, writer actions">
          {panel(
            chartPanel,
            answered(modes.map((m, i) => cpuSeries(m, 10 + i * 12, i === 0))),
            {
              actions: {
                onEdit: noop,
                onMoveDown: noop,
                onRemove: noop,
              },
            },
          )}
        </Case>
        <Case title="Ten series: the last two use the overflow gray">
          {panel(
            chartPanel,
            answered(manyModes.map((m, i) => cpuSeries(m, 4 + i * 6, false))),
          )}
        </Case>
        <Case title="Value, with a stale target">
          {panel(
            valuePanel,
            {
              ...answered([
                {
                  labels: {
                    labels: [{ name: 'instance', value: 'forge-node' }],
                  },
                  samples: [{ timeMs: BigInt(nowMs), value: 52.4e9 }],
                },
                {
                  labels: {
                    labels: [{ name: 'instance', value: 'thumper-node' }],
                  },
                  samples: [
                    { timeMs: BigInt(nowMs - 30 * 60_000), value: 7.1e9 },
                  ],
                },
              ]),
              refreshing: true,
            },
            {
              stale: [
                {
                  objectKey: 'prom/target/thumper-node',
                  lastScrapeMs: nowMs - 30 * 60_000,
                },
              ],
              onOpenTarget: noop,
            },
          )}
        </Case>
      </Section>
      <Section title="Scrape targets">
        {targetCases.map((c) => (
          <Case key={c.title} title={c.title}>
            <div sx={stylexStyles.rule3}>
              <PromTargetView
                objectKey="prom/target/forge-node"
                info={c.state.state === 'unapproved' ? undefined : targetInfo}
                state={c.state}
                action={c.action}
                deviceName={(peer) => `Device …${peer.slice(-8)}`}
                nowMs={nowMs}
                saving={false}
                saveError={c.saveError ?? null}
                onAction={noop}
              />
            </div>
          </Case>
        ))}
      </Section>
    </div>
  )
}

// Section groups the cases of one view.
function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section sx={stylexStyles.rule4}>
      <h2 sx={stylexStyles.rule5}>{title}</h2>
      <div sx={stylexStyles.rule6}>{children}</div>
    </section>
  )
}

// Case frames one state at the panel width.
function Case({ title, children }: { title: string; children: ReactNode }) {
  return (
    <figure sx={stylexStyles.rule7}>
      <figcaption sx={stylexStyles.rule8}>{title}</figcaption>
      <div sx={stylexStyles.rule9}>{children}</div>
    </figure>
  )
}

// noop stands in for the viewers' actions.
function noop() {}
