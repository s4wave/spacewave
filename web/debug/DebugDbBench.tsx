import { useCallback, useState } from 'react'
import { LuArrowLeft, LuDownload, LuPlay } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { DebugDb } from '@s4wave/sdk/debugdb/debugdb.js'
import type {
  BenchmarkConfig,
  BenchmarkResults,
  BenchmarkMetric,
  BenchmarkSuite,
  StorageInfo,
  WatchProgressResponse,
} from '@s4wave/sdk/debugdb/debugdb.pb.js'
import { BenchmarkResults as BenchmarkResultsType } from '@s4wave/sdk/debugdb/debugdb.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
  },
  rule2: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule3: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-text-primary'],
  },
  rule6: { overflowX: 'auto' },
  rule7: { width: '100%' },
  rule8: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    color: themeColors['--color-text-muted'],
  },
  rule9: {
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBottom: themeLayout['--spacing'],
    textAlign: 'left',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    textTransform: 'uppercase',
  },
  rule10: {
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBottom: themeLayout['--spacing'],
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    textTransform: 'uppercase',
  },
  rule11: {
    paddingBottom: themeLayout['--spacing'],
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    textTransform: 'uppercase',
  },
  rule12: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule13: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule15: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule16: {
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: themeColors['--color-background-card'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule17: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule18: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    columnGap: `calc(${themeLayout['--spacing']} * 4)`,
    rowGap: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule19: { color: themeColors['--color-text-muted'] },
  rule20: { color: themeColors['--color-text-primary'] },
  rule21: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-text-primary'],
  },
  rule23: {
    height: `calc(${themeLayout['--spacing']} * 2)`,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-background-dark'],
  },
  rule24: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule25: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: themeColors['--color-background-card'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule26: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule27: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule28: { width: `calc(${themeLayout['--spacing']} * 24)` },
  rule29: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-text-primary'],
  },
  rule30: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule31: {
    cursor: 'not-allowed',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    color: themeColors['--color-text-muted'],
  },
  rule32: {
    cursor: 'pointer',
    backgroundColor: {
      default: themeColors['--color-primary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-primary']} 90%, transparent)`,
      },
    },
    color: themeColors['--color-primary-foreground'],
  },
  rule33: {
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-red-500']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-red-400'],
  },
  rule34: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule35: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule36: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule37: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule38: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-3xl)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule39: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule40: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule41: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

// latencyColor returns a Tailwind text color class based on latency value.
function latencyColor(ms: number): string {
  if (ms < 1) return 'text-green-400'
  if (ms <= 10) return 'text-yellow-400'
  return 'text-red-400'
}

// fmtMs formats a millisecond value with appropriate precision.
function fmtMs(ms: number | undefined): string {
  if (ms === undefined || ms === 0) return '-'
  if (ms < 0.01) return '<0.01'
  if (ms < 1) return ms.toFixed(3)
  if (ms < 100) return ms.toFixed(2)
  return ms.toFixed(1)
}

// MetricRow renders one row of the metric table.
function MetricRow({ metric }: { metric: BenchmarkMetric }) {
  return (
    <tr sx={stylexStyles.rule1}>
      <td sx={stylexStyles.rule2}>{metric.name}</td>
      <td sx={stylexStyles.rule3}>{metric.count?.toString() ?? '-'}</td>
      <td sx={stylexStyles.rule3}>{fmtMs(metric.totalMs)}</td>
      <td
        className={
          /* TODO(stylex): computed class helper */ cn(
            'py-1.5 pr-4 text-right font-mono text-xs',
            latencyColor(metric.minMs ?? 0),
          )
        }
      >
        {fmtMs(metric.minMs)}
      </td>
      <td
        className={
          /* TODO(stylex): computed class helper */ cn(
            'py-1.5 pr-4 text-right font-mono text-xs',
            latencyColor(metric.p50Ms ?? 0),
          )
        }
      >
        {fmtMs(metric.p50Ms)}
      </td>
      <td
        className={
          /* TODO(stylex): computed class helper */ cn(
            'py-1.5 pr-4 text-right font-mono text-xs',
            latencyColor(metric.p99Ms ?? 0),
          )
        }
      >
        {fmtMs(metric.p99Ms)}
      </td>
      <td
        className={
          /* TODO(stylex): computed class helper */ cn(
            'py-1.5 text-right font-mono text-xs',
            latencyColor(metric.maxMs ?? 0),
          )
        }
      >
        {fmtMs(metric.maxMs)}
      </td>
    </tr>
  )
}

// SuiteTable renders results for a single benchmark suite.
function SuiteTable({ suite }: { suite: BenchmarkSuite }) {
  const metrics = suite.metrics ?? []
  if (metrics.length === 0) return null

  return (
    <div sx={stylexStyles.rule4}>
      <h3 sx={stylexStyles.rule5}>{suite.name}</h3>
      <div sx={stylexStyles.rule6}>
        <table sx={stylexStyles.rule7}>
          <thead>
            <tr sx={stylexStyles.rule8}>
              <th sx={stylexStyles.rule9}>Metric</th>
              <th sx={stylexStyles.rule10}>Count</th>
              <th sx={stylexStyles.rule10}>Total (ms)</th>
              <th sx={stylexStyles.rule10}>Min</th>
              <th sx={stylexStyles.rule10}>P50</th>
              <th sx={stylexStyles.rule10}>P99</th>
              <th sx={stylexStyles.rule11}>Max</th>
            </tr>
          </thead>
          <tbody>
            {metrics.map((metric) => (
              <MetricRow key={metric.name} metric={metric} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ResultsDisplay renders the full benchmark results.
function ResultsDisplay({ results }: { results: BenchmarkResults }) {
  const suites = results.suites ?? []
  const downloadResults = useCallback(() => {
    const json = BenchmarkResultsType.toJsonString(results, {
      prettySpaces: 2,
    })
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `debugdb-bench-${new Date().toISOString().slice(0, 19).replace(/:/g, '')}.json`
    a.click()
    URL.revokeObjectURL(url)
  }, [results])

  return (
    <div sx={stylexStyles.rule12}>
      <div sx={stylexStyles.rule13}>
        <div sx={stylexStyles.rule14}>
          {suites.length} suites,{' '}
          {results.totalDurationMillis?.toString() ?? '0'}ms total
        </div>
        <button
          type="button"
          onClick={downloadResults}
          className={
            /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary hover:text-text-primary flex cursor-pointer items-center gap-1.5 text-xs transition-colors'
          }
        >
          <LuDownload {...stylex.props(stylexStyles.rule15)} />
          Download JSON
        </button>
      </div>
      {suites.map((suite) => (
        <SuiteTable key={suite.name} suite={suite} />
      ))}
    </div>
  )
}

// StorageInfoPanel displays current storage backend info.
function StorageInfoPanel({ info }: { info: StorageInfo }) {
  return (
    <div sx={stylexStyles.rule16}>
      <h3
        className={
          /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
        }
      >
        Current Backend
      </h3>
      <p sx={stylexStyles.rule17}>
        These values describe the app&apos;s current storage backend, not the
        temporary benchmark run configuration below.
      </p>
      <div sx={stylexStyles.rule18}>
        <span sx={stylexStyles.rule19}>GOOS</span>
        <span sx={stylexStyles.rule20}>{info.goos || '-'}</span>
        <span sx={stylexStyles.rule19}>GOARCH</span>
        <span sx={stylexStyles.rule20}>{info.goarch || '-'}</span>
      </div>
    </div>
  )
}

// ProgressBar renders the benchmark progress.
function ProgressBar({ progress }: { progress: WatchProgressResponse }) {
  return (
    <div sx={stylexStyles.rule4}>
      <div sx={stylexStyles.rule21}>
        <span sx={stylexStyles.rule22}>
          {progress.suiteName || 'Starting…'}
        </span>
        <span sx={stylexStyles.rule19}>
          {progress.suiteIndex?.toString() ?? '0'}/
          {progress.suiteCount?.toString() ?? '0'}
        </span>
      </div>
      <div sx={stylexStyles.rule23}>
        <div
          className={
            /* TODO(stylex): global or unknown class: progress-width */ 'bg-primary progress-width progress-width-transition-medium h-full rounded-full'
          }
          style={{ '--progress-width': `${progress.percentComplete ?? 0}%` }}
        />
      </div>
      {progress.metricName ? (
        <span sx={stylexStyles.rule24}>{progress.metricName}</span>
      ) : null}
    </div>
  )
}

// BenchmarkPanel manages the benchmark lifecycle.
function BenchmarkPanel({ debugDb }: { debugDb: Resource<DebugDb> }) {
  const [duration, setDuration] = useState(10)
  const [includeWorld, setIncludeWorld] = useState(false)
  const [run, setRun] = useState<BenchmarkConfig | null>(null)
  const benchmark = useResource(
    debugDb,
    async (db, signal, cleanup) => {
      if (!run) return null
      return cleanup(await db.startBenchmark(run, signal))
    },
    [run],
  )
  const progress = useStreamingResource(
    benchmark,
    (activeBenchmark, signal) => activeBenchmark.watchProgress(signal),
    [],
  )
  const results = useResource(
    benchmark,
    (activeBenchmark, signal) => activeBenchmark.getResults(signal),
    [],
  )
  const running = run !== null && (benchmark.loading || results.loading)
  const error = benchmark.error ?? progress.error ?? results.error

  const runBenchmark = useCallback(() => {
    if (!debugDb.value) return
    setRun({
      durationSeconds: duration,
      includeWorldSuite: includeWorld,
    })
  }, [debugDb.value, duration, includeWorld])

  return (
    <div sx={stylexStyles.rule12}>
      <div sx={stylexStyles.rule25}>
        <h3
          className={
            /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs font-semibold tracking-wider uppercase'
          }
        >
          Benchmark Run
        </h3>
        <p sx={stylexStyles.rule14}>
          These options apply only to the throwaway benchmark volume and engine
          created for this run. They do not change the app&apos;s current
          storage backend shown above.
        </p>
        <div sx={stylexStyles.rule26}>
          <label sx={stylexStyles.rule27}>
            <span sx={stylexStyles.rule19}>Duration</span>
            <input
              type="range"
              min={5}
              max={60}
              step={5}
              value={duration}
              onChange={(e) => setDuration(Number(e.target.value))}
              disabled={running}
              sx={stylexStyles.rule28}
            />
            <span sx={stylexStyles.rule29}>{duration}s</span>
          </label>
          <label sx={stylexStyles.rule27}>
            <input
              type="checkbox"
              checked={includeWorld}
              onChange={(e) => setIncludeWorld(e.target.checked)}
              disabled={running}
            />
            <span sx={stylexStyles.rule19}>World Transaction Suite</span>
          </label>
          <button
            type="button"
            onClick={runBenchmark}
            disabled={running || !debugDb.value}
            sx={[
              stylexStyles.rule30,
              running ? stylexStyles.rule31 : stylexStyles.rule32,
            ]}
          >
            <LuPlay {...stylex.props(stylexStyles.rule15)} />
            {running ? 'Running…' : 'Run Benchmark'}
          </button>
        </div>
      </div>

      {running && !progress.loading && progress.value ? (
        <ProgressBar progress={progress.value} />
      ) : null}

      {error ? <div sx={stylexStyles.rule33}>{error.message}</div> : null}

      {!results.loading && results.value ? (
        <ResultsDisplay results={results.value} />
      ) : null}
    </div>
  )
}

export function DebugDbBench() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const rootResource = useRootResource()
  const debugDbResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      return cleanup(await root.getDebugDb(signal))
    },
    [],
  )

  const storageInfo = usePromise(
    useCallback(
      (signal: AbortSignal) => {
        const db = debugDbResource.value
        if (!db) return undefined
        return db.getStorageInfo(signal)
      },
      [debugDbResource.value],
    ),
  )

  const loading = rootResource.loading || debugDbResource.loading
  const err = rootResource.error || debugDbResource.error

  return (
    <div sx={stylexStyles.rule34}>
      <div sx={stylexStyles.rule35}>
        <button type="button" onClick={goBack} sx={stylexStyles.rule36}>
          <LuArrowLeft {...stylex.props(stylexStyles.rule37)} />
          Back to home
        </button>

        <h1 sx={stylexStyles.rule38}>Storage Benchmark</h1>
        <p sx={stylexStyles.rule39}>
          Benchmark block storage, garbage collection, metadata, and world
          transaction performance.
        </p>

        {loading ? (
          <div sx={stylexStyles.rule40}>Loading…</div>
        ) : err ? (
          <div sx={stylexStyles.rule33}>{err.message}</div>
        ) : (
          <div sx={stylexStyles.rule41}>
            {storageInfo.data ? (
              <StorageInfoPanel info={storageInfo.data} />
            ) : null}
            <BenchmarkPanel debugDb={debugDbResource} />
          </div>
        )}
      </div>
    </div>
  )
}
