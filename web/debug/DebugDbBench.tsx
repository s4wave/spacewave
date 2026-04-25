import { useCallback, useState } from 'react'
import { LuArrowLeft, LuDownload, LuPlay } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { DebugDb } from '@s4wave/sdk/debugdb/debugdb.js'
import type {
  BenchmarkResults,
  BenchmarkSuite,
  BenchmarkMetric,
  StorageInfo,
  WatchProgressResponse,
} from '@s4wave/sdk/debugdb/debugdb.pb.js'
import { BenchmarkResults as BenchmarkResultsType } from '@s4wave/sdk/debugdb/debugdb.pb.js'

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
    <tr className="border-b border-white/5">
      <td className="py-1.5 pr-4 font-mono text-xs">{metric.name}</td>
      <td className="text-text-muted py-1.5 pr-4 text-right font-mono text-xs">
        {metric.count?.toString() ?? '-'}
      </td>
      <td className="text-text-muted py-1.5 pr-4 text-right font-mono text-xs">
        {fmtMs(metric.totalMs)}
      </td>
      <td
        className={cn(
          'py-1.5 pr-4 text-right font-mono text-xs',
          latencyColor(metric.minMs ?? 0),
        )}
      >
        {fmtMs(metric.minMs)}
      </td>
      <td
        className={cn(
          'py-1.5 pr-4 text-right font-mono text-xs',
          latencyColor(metric.p50Ms ?? 0),
        )}
      >
        {fmtMs(metric.p50Ms)}
      </td>
      <td
        className={cn(
          'py-1.5 pr-4 text-right font-mono text-xs',
          latencyColor(metric.p99Ms ?? 0),
        )}
      >
        {fmtMs(metric.p99Ms)}
      </td>
      <td
        className={cn(
          'py-1.5 text-right font-mono text-xs',
          latencyColor(metric.maxMs ?? 0),
        )}
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
    <div className="flex flex-col gap-2">
      <h3 className="text-text-primary font-mono text-sm font-semibold">
        {suite.name}
      </h3>
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="text-text-muted border-b border-white/10">
              <th className="pr-4 pb-1 text-left font-mono text-[10px] font-normal uppercase">
                Metric
              </th>
              <th className="pr-4 pb-1 text-right font-mono text-[10px] font-normal uppercase">
                Count
              </th>
              <th className="pr-4 pb-1 text-right font-mono text-[10px] font-normal uppercase">
                Total (ms)
              </th>
              <th className="pr-4 pb-1 text-right font-mono text-[10px] font-normal uppercase">
                Min
              </th>
              <th className="pr-4 pb-1 text-right font-mono text-[10px] font-normal uppercase">
                P50
              </th>
              <th className="pr-4 pb-1 text-right font-mono text-[10px] font-normal uppercase">
                P99
              </th>
              <th className="pb-1 text-right font-mono text-[10px] font-normal uppercase">
                Max
              </th>
            </tr>
          </thead>
          <tbody>
            {metrics.map((m, i) => (
              <MetricRow key={m.name || i} metric={m} />
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
  const info = results.info
  const downloadResults = useCallback(() => {
    const json = BenchmarkResultsType.toJsonString(results, { prettySpaces: 2 })
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `debugdb-bench-${new Date().toISOString().slice(0, 19).replace(/:/g, '')}.json`
    a.click()
    URL.revokeObjectURL(url)
  }, [results])

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div className="text-text-muted text-xs">
          {suites.length} suites,{' '}
          {results.totalDurationMillis?.toString() ?? '0'}ms total, mode{' '}
          {info?.asyncIo ? 'async' : 'sync'}
        </div>
        <button
          onClick={downloadResults}
          className="text-text-secondary hover:text-text-primary flex cursor-pointer items-center gap-1.5 text-xs transition-colors"
        >
          <LuDownload className="h-3.5 w-3.5" />
          Download JSON
        </button>
      </div>
      {suites.map((suite, i) => (
        <SuiteTable key={suite.name || i} suite={suite} />
      ))}
    </div>
  )
}

// StorageInfoPanel displays current storage backend info.
function StorageInfoPanel({ info }: { info: StorageInfo }) {
  return (
    <div className="bg-background-card rounded-lg p-4">
      <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
        Current Backend
      </h3>
      <p className="text-text-muted mb-3 text-xs">
        These values describe the app&apos;s current storage backend, not the
        temporary benchmark run configuration below.
      </p>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 font-mono text-xs">
        <span className="text-text-muted">GOOS</span>
        <span className="text-text-primary">{info.goos || '-'}</span>
        <span className="text-text-muted">GOARCH</span>
        <span className="text-text-primary">{info.goarch || '-'}</span>
        {info.blockShardCount ?
          <>
            <span className="text-text-muted">Block Shards</span>
            <span className="text-text-primary">
              {info.blockShardCount.toString()}
            </span>
          </>
        : null}
        {info.blockFlushThreshold ?
          <>
            <span className="text-text-muted">Flush Threshold</span>
            <span className="text-text-primary">
              {info.blockFlushThreshold.toString()}
            </span>
          </>
        : null}
        {info.blockFlushMaxAgeMillis ?
          <>
            <span className="text-text-muted">Flush Max Age</span>
            <span className="text-text-primary">
              {info.blockFlushMaxAgeMillis.toString()}ms
            </span>
          </>
        : null}
        {info.pageSize ?
          <>
            <span className="text-text-muted">Page Size</span>
            <span className="text-text-primary">
              {info.pageSize.toString()}
            </span>
          </>
        : null}
        <span className="text-text-muted">Async I/O</span>
        <span className="text-text-primary">{info.asyncIo ? 'on' : 'off'}</span>
      </div>
    </div>
  )
}

// ProgressBar renders the benchmark progress.
function ProgressBar({ progress }: { progress: WatchProgressResponse }) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between text-xs">
        <span className="text-text-primary font-mono">
          {progress.suiteName || 'Starting...'}
        </span>
        <span className="text-text-muted">
          {progress.suiteIndex?.toString() ?? '0'}/
          {progress.suiteCount?.toString() ?? '0'}
        </span>
      </div>
      <div className="bg-background-dark h-2 overflow-hidden rounded-full">
        <div
          className="bg-primary h-full rounded-full transition-all duration-300"
          style={{ width: `${progress.percentComplete ?? 0}%` }}
        />
      </div>
      {progress.metricName ?
        <span className="text-text-muted font-mono text-[10px]">
          {progress.metricName}
        </span>
      : null}
    </div>
  )
}

// BenchmarkPanel manages the benchmark lifecycle.
function BenchmarkPanel({ debugDb }: { debugDb: Resource<DebugDb> }) {
  const [duration, setDuration] = useState(10)
  const [includeWorld, setIncludeWorld] = useState(false)
  const [asyncIo, setAsyncIo] = useState(false)
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState<WatchProgressResponse | null>(null)
  const [results, setResults] = useState<BenchmarkResults | null>(null)
  const [error, setError] = useState<string | null>(null)

  const runBenchmark = useCallback(async () => {
    const db = debugDb.value
    if (!db) return

    setRunning(true)
    setProgress(null)
    setResults(null)
    setError(null)

    try {
      using bench = await db.startBenchmark({
        durationSeconds: duration,
        includeWorldSuite: includeWorld,
        asyncIo,
      })

      // Watch progress in the background.
      const progressStream = bench.watchProgress()
      for await (const p of progressStream) {
        setProgress(p)
        if (p.done) break
      }

      const r = await bench.getResults()
      setResults(r)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setRunning(false)
    }
  }, [debugDb.value, duration, includeWorld, asyncIo])

  return (
    <div className="flex flex-col gap-6">
      <div className="bg-background-card flex flex-col gap-4 rounded-lg p-4">
        <h3 className="text-text-secondary text-xs font-semibold tracking-wider uppercase">
          Benchmark Run
        </h3>
        <p className="text-text-muted text-xs">
          These options apply only to the throwaway benchmark volume and engine
          created for this run. They do not change the app&apos;s current
          storage backend shown above.
        </p>
        <div className="flex flex-wrap items-center gap-6">
          <label className="flex items-center gap-2 text-xs">
            <span className="text-text-muted">Duration</span>
            <input
              type="range"
              min={5}
              max={60}
              step={5}
              value={duration}
              onChange={(e) => setDuration(Number(e.target.value))}
              disabled={running}
              className="w-24"
            />
            <span className="text-text-primary w-8 font-mono">{duration}s</span>
          </label>
          <label className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={includeWorld}
              onChange={(e) => setIncludeWorld(e.target.checked)}
              disabled={running}
            />
            <span className="text-text-muted">World Transaction Suite</span>
          </label>
          <label className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={asyncIo}
              onChange={(e) => setAsyncIo(e.target.checked)}
              disabled={running}
            />
            <span className="text-text-muted">Async I/O</span>
          </label>
          <button
            onClick={() => {
              void runBenchmark()
            }}
            disabled={running || !debugDb.value}
            className={cn(
              'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
              running ?
                'text-text-muted cursor-not-allowed bg-white/5'
              : 'bg-primary text-primary-foreground hover:bg-primary/90 cursor-pointer',
            )}
          >
            <LuPlay className="h-3.5 w-3.5" />
            {running ? 'Running...' : 'Run Benchmark'}
          </button>
        </div>
      </div>

      {running && progress ?
        <ProgressBar progress={progress} />
      : null}

      {error ?
        <div className="rounded-lg bg-red-500/10 p-3 font-mono text-xs text-red-400">
          {error}
        </div>
      : null}

      {results ?
        <ResultsDisplay results={results} />
      : null}
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
    <div className="bg-background @container flex w-full flex-1 flex-col overflow-y-auto">
      <div className="mx-auto w-full max-w-5xl px-4 py-6 @lg:px-8">
        <button
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground mb-6 flex cursor-pointer items-center gap-2 transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back to home
        </button>

        <h1 className="text-foreground mb-2 text-3xl font-bold">
          Storage Benchmark
        </h1>
        <p className="text-foreground-alt mb-8 text-sm">
          Benchmark storage layer performance: blockshard engine, block store,
          GC flush, and meta store operations.
        </p>

        {loading ?
          <div className="text-text-muted text-sm">Loading...</div>
        : err ?
          <div className="rounded-lg bg-red-500/10 p-3 font-mono text-xs text-red-400">
            {err.message}
          </div>
        : <div className="flex flex-col gap-8">
            {storageInfo.data ?
              <StorageInfoPanel info={storageInfo.data} />
            : null}
            <BenchmarkPanel debugDb={debugDbResource} />
          </div>
        }
      </div>
    </div>
  )
}
