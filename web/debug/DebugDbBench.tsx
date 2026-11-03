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

const stylexStyles5 = stylex.create({
  rule1: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
})

const stylexStyles4 = stylex.create({
  rule1: { color: themeColors['--color-green-400'] },
  rule2: { color: themeColors['--color-yellow-400'] },
  rule3: { color: themeColors['--color-red-400'] },
})

const stylexStyles3 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-text-secondary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-text-primary'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    color: themeColors['--color-text-secondary'],
    fontSize: themeLayout['--text-ui'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    textTransform: 'uppercase',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    transition: 'width 300ms',
    backgroundColor: themeColors['--color-primary'],
    height: '100%',
    width: 'var(--progress-width)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule2: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: themeColors['--color-text-muted'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-text-primary'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule6: { overflowX: 'auto' },
  rule7: { width: '100%' },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    color: themeColors['--color-text-muted'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    textTransform: 'uppercase',
    paddingBottom: themeLayout['--spacing'],
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule10: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    textTransform: 'uppercase',
    paddingBottom: themeLayout['--spacing'],
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule11: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    textTransform: 'uppercase',
    paddingBottom: themeLayout['--spacing'],
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule13: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule14: {
    color: themeColors['--color-text-muted'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule16: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: themeColors['--color-background-card'],
  },
  rule17: {
    color: themeColors['--color-text-muted'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule18: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    columnGap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    rowGap: themeLayout['--spacing'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule19: { color: themeColors['--color-text-muted'] },
  rule20: { color: themeColors['--color-text-primary'] },
  rule21: {
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'space-between',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule22: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-text-primary'],
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule23: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-dark'],
    height: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule24: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-text-muted'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule25: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: themeColors['--color-background-card'],
    display: 'flex',
    flexDirection: 'column',
  },
  rule26: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule27: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule28: { width: `calc(${themeLayout['--spacing']} * 24)` },
  rule29: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-text-primary'],
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule30: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule31: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-white']} 5%, transparent)`,
    color: themeColors['--color-text-muted'],
    cursor: 'not-allowed',
  },
  rule32: {
    backgroundColor: {
      default: themeColors['--color-primary'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-primary']} 90%, transparent)`,
      },
    },
    color: themeColors['--color-primary-foreground'],
    cursor: 'pointer',
  },
  rule33: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-red-500']} 10%, transparent)`,
    color: themeColors['--color-red-400'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule34: {
    flex: 1,
    backgroundColor: themeColors['--color-background'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    overflowY: 'auto',
    width: '100%',
  },
  rule35: {
    marginInline: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    maxWidth: 'var(--container-5xl)',
    width: '100%',
  },
  rule36: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule37: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule38: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-3xl)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule39: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule40: {
    color: themeColors['--color-text-muted'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule41: {
    gap: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    flexDirection: 'column',
  },
})

// latencyColor selects a native text tone for measured latency.
function latencyColor(ms: number): stylex.StyleXStyles {
  if (ms < 1) return [stylexStyles4.rule1]
  if (ms <= 10) return [stylexStyles4.rule2]
  return [stylexStyles4.rule3]
}

// fmtMs formats a millisecond value with appropriate precision.
function fmtMs(ms: number | undefined): string {
  // Preserve useful sub-millisecond precision while treating absent measurements as unavailable.
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
      <td sx={[[stylexStyles5.rule1], [latencyColor(metric.minMs ?? 0)]]}>
        {fmtMs(metric.minMs)}
      </td>
      <td sx={[[stylexStyles5.rule1], [latencyColor(metric.p50Ms ?? 0)]]}>
        {fmtMs(metric.p50Ms)}
      </td>
      <td sx={[[stylexStyles5.rule1], [latencyColor(metric.p99Ms ?? 0)]]}>
        {fmtMs(metric.p99Ms)}
      </td>
      <td sx={[[stylexStyles5.rule2], [latencyColor(metric.maxMs ?? 0)]]}>
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
    // Encode the reported metrics into a downloadable JSON resource.
    const json = BenchmarkResultsType.toJsonString(results, {
      prettySpaces: 2,
    })
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)

    // Trigger the local download and release its temporary object URL.
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
          sx={stylexStyles3.rule1}
        >
          <LuDownload sx={stylexStyles.rule15} />
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
      <h3 sx={stylexStyles3.rule2}>Current Backend</h3>
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
          sx={stylexStyles2.rule1}
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
  // Keep the form draft separate from the configuration of the active benchmark.
  const [duration, setDuration] = useState(10)
  const [includeWorld, setIncludeWorld] = useState(false)
  const [run, setRun] = useState<BenchmarkConfig | null>(null)

  // Own the benchmark and watch progress and completion through its resource.
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

  // Resolve the controls from the active resource state and its first failure.
  const running = run !== null && (benchmark.loading || results.loading)
  const error = benchmark.error ?? progress.error ?? results.error

  // Snapshot the selected settings for one benchmark resource lifetime.
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
        <h3 sx={stylexStyles3.rule3}>Benchmark Run</h3>
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
            <LuPlay sx={stylexStyles.rule15} />
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

export function DebugDbBench({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  // Keep navigation within the active application router.
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  // Resolve the debug database from the current root resource.
  const rootResource = useRootResource()
  const debugDbResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      return cleanup(await root.getDebugDb(signal))
    },
    [],
  )

  // Refresh storage information when the debug database resource changes.
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

  // Show loading or the first resource failure before offering benchmark controls.
  const loading = rootResource.loading || debugDbResource.loading
  const err = rootResource.error || debugDbResource.error

  // Render the resource status and its available benchmark panel.
  return (
    <div sx={[stylexStyles.rule34, sx]}>
      <div sx={stylexStyles.rule35}>
        <button type="button" onClick={goBack} sx={stylexStyles.rule36}>
          <LuArrowLeft sx={stylexStyles.rule37} />
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
