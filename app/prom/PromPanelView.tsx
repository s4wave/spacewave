import { useCallback, useMemo, useState, type ReactNode } from 'react'
import {
  LuArrowDown,
  LuArrowUp,
  LuPencil,
  LuTrash2,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'

import {
  PanelKind,
  ResultType,
  type Panel,
  type QueryResponse,
  type Series,
} from '@s4wave/sdk/prom/prom.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

import { PromChart } from './PromChart.js'
import {
  formatAgo,
  formatClock,
  formatValue,
  seriesColor,
  seriesNames,
  targetName,
} from './prom-view.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-amber-500'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// PanelAnswer is the state of a panel's query.
export type PanelAnswer =
  // The first answer has not arrived.
  | { state: 'loading' }
  // The engine refused or failed the expression.
  | { state: 'error'; message: string }
  // The query answered over startMs to endMs; refreshing is set while a
  // newer answer is on its way.
  | {
      state: 'answered'
      resp: QueryResponse
      startMs: number
      endMs: number
      stepMs: number
      refreshing: boolean
    }

// StaleTarget is a target a panel read whose Device stopped committing.
export interface StaleTarget {
  objectKey: string
  lastScrapeMs: number
}

// PanelActions are the writer's actions on a panel; a reader has none.
export interface PanelActions {
  onEdit: () => void
  onMoveUp?: () => void
  onMoveDown?: () => void
  onRemove: () => void
}

export interface PromPanelViewProps {
  panel: Panel
  answer: PanelAnswer
  // rangeLabel names the time range, such as 1h.
  rangeLabel: string
  stale: readonly StaleTarget[]
  nowMs: number
  // width is the panel body's width in pixels, zero until measured.
  width: number
  hoverMs: number | null
  onHover: (ms: number | null) => void
  onOpenTarget?: (objectKey: string) => void
  actions?: PanelActions
}

// PromPanelView shows one panel in the state of its query: a placeholder of
// the chart's height, a chart or values with a legend, no samples, or the
// engine's error, with notes for stale targets.
export function PromPanelView({
  panel,
  answer,
  rangeLabel,
  stale,
  nowMs,
  width,
  hoverMs,
  onHover,
  onOpenTarget,
  actions,
}: PromPanelViewProps) {
  const isValue = panel.kind === PanelKind.VALUE
  const series = useMemo(
    () => (answer.state === 'answered' ? (answer.resp.series ?? []) : []),
    [answer],
  )
  const names = useMemo(() => seriesNames(series), [series])
  const [hidden, setHidden] = useState<ReadonlySet<string>>(new Set())
  const hiddenIndexes = useMemo(
    () => new Set(names.flatMap((n, i) => (hidden.has(n) ? [i] : []))),
    [names, hidden],
  )
  const toggle = useCallback((name: string) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (!next.delete(name)) next.add(name)
      return next
    })
  }, [])

  return (
    <section
      aria-label={panel.title}
      className="border-foreground/8 bg-background-card/30 flex min-w-0 flex-col rounded-md border"
    >
      <header className="flex h-8 items-center gap-2 px-3">
        <h3 className="text-foreground min-w-0 flex-1 truncate text-xs font-semibold">
          {panel.title}
        </h3>
        {answer.state === 'answered' && answer.refreshing ? (
          <span className="bg-foreground-alt/60 size-1.5 rounded-full" />
        ) : null}
        {actions ? <PanelActionButtons actions={actions} /> : null}
      </header>

      <div className="min-w-0 px-1 pb-2">{renderBody()}</div>

      {stale.length > 0 ? (
        <ul className="border-foreground/8 space-y-0.5 border-t px-3 py-1.5">
          {stale.map((t) => (
            <li
              key={t.objectKey}
              className="text-foreground-alt flex items-center gap-1.5 text-xs"
            >
              <LuTriangleAlert sx={stylexStyles.rule1} aria-hidden />
              <button
                type="button"
                className="hover:text-foreground underline-offset-2 hover:underline"
                onClick={() => onOpenTarget?.(t.objectKey)}
              >
                {targetName(t.objectKey)} last scraped{' '}
                {formatAgo(t.lastScrapeMs, nowMs)} ago.
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )

  // renderBody renders the panel's body for the answer's state.
  function renderBody() {
    switch (answer.state) {
      case 'loading':
        return (
          <div className="bg-foreground/3 mx-2 h-40 animate-pulse rounded" />
        )
      case 'error':
        return (
          <PanelMessage>
            <p className="text-foreground">
              This expression did not run: {answer.message}
            </p>
            <code className="text-foreground-alt mt-1 block text-xs break-all">
              {panel.expr}
            </code>
          </PanelMessage>
        )
      case 'answered':
        if (
          answer.resp.resultType === ResultType.STRING ||
          series.length === 0
        ) {
          return (
            <PanelMessage>
              <p className="text-foreground">
                No samples for this expression in the last {rangeLabel}.
              </p>
              <code className="text-foreground-alt mt-1 block text-xs break-all">
                {panel.expr}
              </code>
            </PanelMessage>
          )
        }
        if (isValue) {
          return <ValueRows series={series} names={names} panel={panel} />
        }
        return (
          <div className={cn(answer.refreshing && 'opacity-90')}>
            {width > 0 ? (
              <PromChart
                series={series}
                hidden={hiddenIndexes}
                startMs={answer.startMs}
                endMs={answer.endMs}
                stepMs={answer.stepMs}
                unit={panel.unit ?? 0}
                width={width}
                hoverMs={hoverMs}
                onHover={onHover}
              />
            ) : (
              <div className="h-40" />
            )}
            <Legend
              series={series}
              names={names}
              hidden={hidden}
              onToggle={toggle}
              hoverMs={hoverMs}
              panel={panel}
            />
          </div>
        )
    }
  }
}

// PanelMessage holds a message in the chart's height.
function PanelMessage({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-40 flex-col justify-center px-3 text-sm">
      {children}
    </div>
  )
}

// PanelActionButtons are the writer's Edit, Move and Remove buttons.
function PanelActionButtons({ actions }: { actions: PanelActions }) {
  const btn =
    'text-foreground-alt hover:text-foreground hover:bg-foreground/5 rounded p-1 disabled:opacity-30'
  return (
    <div className="flex items-center">
      <button
        type="button"
        className={btn}
        title="Move up"
        aria-label="Move up"
        disabled={!actions.onMoveUp}
        onClick={actions.onMoveUp}
      >
        <LuArrowUp sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        className={btn}
        title="Move down"
        aria-label="Move down"
        disabled={!actions.onMoveDown}
        onClick={actions.onMoveDown}
      >
        <LuArrowDown sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        className={btn}
        title="Edit"
        aria-label="Edit"
        onClick={actions.onEdit}
      >
        <LuPencil sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        className={btn}
        title="Remove"
        aria-label="Remove"
        onClick={actions.onRemove}
      >
        <LuTrash2 sx={stylexStyles.rule2} />
      </button>
    </div>
  )
}

// sampleAt returns the latest sample of s at or before ms, or the last one
// when ms is null.
function sampleAt(s: Series, ms: number | null) {
  const samples = s.samples ?? []
  if (ms === null) return samples[samples.length - 1]
  let found = undefined
  for (const sample of samples) {
    if (Number(sample.timeMs ?? 0n) > ms) break
    found = sample
  }
  return found
}

// Legend lists each series with its color and its value at the hover time,
// or its latest value. Clicking a row hides or shows its line.
function Legend({
  series,
  names,
  hidden,
  onToggle,
  hoverMs,
  panel,
}: {
  series: readonly Series[]
  names: readonly string[]
  hidden: ReadonlySet<string>
  onToggle: (name: string) => void
  hoverMs: number | null
  panel: Panel
}) {
  return (
    <ul className="max-h-24 space-y-px overflow-y-auto px-2 pt-1">
      {series.map((s, i) => {
        const sample = sampleAt(s, hoverMs)
        const off = hidden.has(names[i])
        return (
          <li key={names[i]}>
            <button
              type="button"
              aria-pressed={!off}
              onClick={() => onToggle(names[i])}
              className={cn(
                'hover:bg-foreground/5 flex w-full items-center gap-2 rounded px-1 text-left text-xs',
                off && 'opacity-40',
              )}
            >
              <svg className="h-0.5 w-3 shrink-0" aria-hidden>
                <rect width="100%" height="100%" rx="1" fill={seriesColor(i)} />
              </svg>
              <span className="text-foreground-alt min-w-0 flex-1 truncate">
                {names[i]}
              </span>
              <span className="text-foreground tabular-nums">
                {sample ? formatValue(sample.value ?? 0, panel.unit ?? 0) : ''}
              </span>
            </button>
          </li>
        )
      })}
      {hoverMs !== null ? (
        <li className="text-foreground-alt text-metadata px-1 tabular-nums">
          at {formatClock(hoverMs)}
        </li>
      ) : null}
    </ul>
  )
}

// ValueRows shows one value per series with its unit; hovering a row shows
// the sample's time.
function ValueRows({
  series,
  names,
  panel,
}: {
  series: readonly Series[]
  names: readonly string[]
  panel: Panel
}) {
  return (
    <ul className="flex min-h-40 flex-col justify-center gap-1 px-3">
      {series.map((s, i) => {
        const sample = sampleAt(s, null)
        return (
          <li
            key={names[i]}
            title={sample ? formatClock(Number(sample.timeMs ?? 0n)) : ''}
            className="flex items-baseline gap-3"
          >
            <span className="text-foreground-alt min-w-0 flex-1 truncate text-xs">
              {names[i]}
            </span>
            <span className="text-foreground text-lg font-semibold tabular-nums">
              {sample ? formatValue(sample.value ?? 0, panel.unit ?? 0) : ''}
            </span>
          </li>
        )
      })}
    </ul>
  )
}
