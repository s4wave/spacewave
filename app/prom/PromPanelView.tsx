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

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    maxHeight: `calc(${themeLayout['--spacing']} * 24)`,
    overflowY: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingTop: themeLayout['--spacing'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flexDirection: 'column',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule5: {
    minWidth: 0,
    paddingInline: themeLayout['--spacing'],
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    textUnderlineOffset: '2px',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule8: {
    marginInline: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 40)`,
    animationName: 'pulse',
    animationDuration: '2s',
    animationTimingFunction: 'cubic-bezier(.4, 0, .6, 1)',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
  },
  rule9: { color: themeColors['--color-foreground'] },
  rule10: {
    marginTop: themeLayout['--spacing'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: themeColors['--color-foreground-alt'],
  },
  rule11: { opacity: 0.9 },
  rule12: { height: `calc(${themeLayout['--spacing']} * 40)` },
  rule13: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 40)`,
    flexDirection: 'column',
    justifyContent: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule14: { display: 'flex', alignItems: 'center' },
  rule15: {
    borderRadius: '.25rem',
    padding: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    opacity: { default: null, ':disabled': 0.3 },
  },
  rule16: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    paddingInline: themeLayout['--spacing'],
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule17: { opacity: 0.4 },
  rule18: {
    height: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
  },
  rule19: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
  },
  rule20: {
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule21: {
    paddingInline: themeLayout['--spacing'],
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule22: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 40)`,
    flexDirection: 'column',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule23: {
    display: 'flex',
    alignItems: 'baseline',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule24: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule25: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
})

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
    <section aria-label={panel.title} sx={stylexStyles2.rule1}>
      <header sx={stylexStyles2.rule2}>
        <h3 sx={stylexStyles2.rule3}>{panel.title}</h3>
        {answer.state === 'answered' && answer.refreshing ? (
          <span sx={stylexStyles2.rule4} />
        ) : null}
        {actions ? <PanelActionButtons actions={actions} /> : null}
      </header>

      <div sx={stylexStyles2.rule5}>{renderBody()}</div>

      {stale.length > 0 ? (
        <ul sx={stylexStyles3.rule1}>
          {stale.map((t) => (
            <li
              key={t.objectKey}
              sx={[nativeChildSpacing.s0_5, stylexStyles2.rule6]}
            >
              <LuTriangleAlert sx={stylexStyles.rule1} aria-hidden />
              <button
                type="button"
                sx={stylexStyles2.rule7}
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
        return <div sx={stylexStyles2.rule8} />
      case 'error':
        return (
          <PanelMessage>
            <p sx={stylexStyles2.rule9}>
              This expression did not run: {answer.message}
            </p>
            <code sx={stylexStyles2.rule10}>{panel.expr}</code>
          </PanelMessage>
        )
      case 'answered':
        if (
          answer.resp.resultType === ResultType.STRING ||
          series.length === 0
        ) {
          return (
            <PanelMessage>
              <p sx={stylexStyles2.rule9}>
                No samples for this expression in the last {rangeLabel}.
              </p>
              <code sx={stylexStyles2.rule10}>{panel.expr}</code>
            </PanelMessage>
          )
        }
        if (isValue) {
          return <ValueRows series={series} names={names} panel={panel} />
        }
        return (
          <div sx={[!!answer.refreshing && stylexStyles2.rule11]}>
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
              <div sx={stylexStyles2.rule12} />
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
  return <div sx={stylexStyles2.rule13}>{children}</div>
}

// PanelActionButtons are the writer's Edit, Move and Remove buttons.
function PanelActionButtons({ actions }: { actions: PanelActions }) {
  return (
    <div sx={stylexStyles2.rule14}>
      <button
        type="button"
        sx={stylexStyles2.rule15}
        title="Move up"
        aria-label="Move up"
        disabled={!actions.onMoveUp}
        onClick={actions.onMoveUp}
      >
        <LuArrowUp sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule15}
        title="Move down"
        aria-label="Move down"
        disabled={!actions.onMoveDown}
        onClick={actions.onMoveDown}
      >
        <LuArrowDown sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule15}
        title="Edit"
        aria-label="Edit"
        onClick={actions.onEdit}
      >
        <LuPencil sx={stylexStyles.rule2} />
      </button>
      <button
        type="button"
        sx={stylexStyles2.rule15}
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
    <ul sx={stylexStyles4.rule1}>
      {series.map((s, i) => {
        const sample = sampleAt(s, hoverMs)
        const off = hidden.has(names[i])
        return (
          <li sx={legendRowStyles.row} key={names[i]}>
            <button
              type="button"
              aria-pressed={!off}
              onClick={() => onToggle(names[i])}
              sx={[stylexStyles2.rule16, !!off && stylexStyles2.rule17]}
            >
              <svg sx={stylexStyles2.rule18} aria-hidden>
                <rect width="100%" height="100%" rx="1" fill={seriesColor(i)} />
              </svg>
              <span sx={stylexStyles2.rule19}>{names[i]}</span>
              <span sx={stylexStyles2.rule20}>
                {sample ? formatValue(sample.value ?? 0, panel.unit ?? 0) : ''}
              </span>
            </button>
          </li>
        )
      })}
      {hoverMs !== null ? (
        <li sx={stylexStyles2.rule21}>at {formatClock(hoverMs)}</li>
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
    <ul sx={stylexStyles2.rule22}>
      {series.map((s, i) => {
        const sample = sampleAt(s, null)
        return (
          <li
            key={names[i]}
            title={sample ? formatClock(Number(sample.timeMs ?? 0n)) : ''}
            sx={stylexStyles2.rule23}
          >
            <span sx={stylexStyles2.rule24}>{names[i]}</span>
            <span sx={stylexStyles2.rule25}>
              {sample ? formatValue(sample.value ?? 0, panel.unit ?? 0) : ''}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s0_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// legendRowStyles keeps the one-pixel gap at the physical legend rows.
const legendRowStyles = stylex.create({
  row: {
    marginBlockStart: { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: { default: null, ':not(:last-child)': '1px' },
  },
})
