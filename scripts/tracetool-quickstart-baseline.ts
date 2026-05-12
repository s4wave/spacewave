#!/usr/bin/env bun

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'fs'
import { dirname, join, resolve } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const rootDir = resolve(__dirname, '..')
const defaultArtifactDir = join(
  rootDir,
  '.bldr',
  'e2e-releasewasm',
  'artifacts',
)
const defaultTestName = 'TestQuickstartPrerenderAutoBootsProductionWasmBundle'

interface CliOptions {
  smokePath: string
  tracePath: string
  outPath: string
  markdownPath: string
  topCount: number
}

interface StartupMark {
  label?: string
  name?: string
  startTimeMs?: number
}

interface StartupSegment {
  name?: string
  startMs?: number | null
  endMs?: number | null
  elapsedMs?: number | null
  attribution?: string
  evidence?: string[]
}

interface AcceptedOperationTiming {
  ordinal?: number
  startedMs?: number
  finishedMs?: number
  elapsedMs?: number
  seqno?: string
}

interface QuickstartPhase {
  name?: string
  startedMs?: number
  finishedMs?: number
  elapsedMs?: number
}

interface PostLoadSharedObjectWorkload {
  scenario?: string
  skipped?: boolean
  skippedReason?: string
  operationSemantics?: string
  operationTypeId?: string
  opCount?: number
  startedMs?: number
  finishedMs?: number
  totalMs?: number
  opAvgMs?: number
  opMinMs?: number
  opMaxMs?: number
  opsPerSec?: number
  startingSeqno?: string
  endingSeqno?: string
  acceptedOperationTimings?: AcceptedOperationTiming[]
}

interface QuickstartSmokeArtifact {
  schemaVersion?: number
  scenario?: string
  collectedAt?: string
  finalURL?: string
  source?: {
    head?: string
    dirty?: boolean
    statusShort?: string[]
  }
  timing?: {
    driveFrameReadyMs?: number
    driveContentReadyMs?: number | null
    quickstart?: {
      startedMs?: number
      progressReadyMs?: number
      contentReadyMs?: number
      finishedMs?: number
      state?: string
      phases?: QuickstartPhase[]
    } | null
  }
  readiness?: {
    frameReadyMs?: number
    contentReadyMs?: number | null
    progressReadyMs?: number | null
    quickstartContentReadyMs?: number | null
  }
  runtimeTrace?: {
    captured?: boolean
    path?: string
    bytes?: number
    skippedReason?: string
  }
  postLoadSharedObjectWorkload?: PostLoadSharedObjectWorkload
  startupMarks?: StartupMark[]
  startupAttribution?: {
    longestSegment?: StartupSegment | null
    segments?: StartupSegment[]
  }
}

interface TraceEvent {
  name?: string
  cat?: string
  ph?: string
  ts?: number
  dur?: number
  tdur?: number
  pid?: number
  tid?: number
  args?: unknown
}

interface TraceTerm {
  name: string
  category: string
  eventCount: number
  totalMs: number
  selfMs: number
  maxMs: number
  threadCount: number
}

interface TraceWindowSummary {
  name: string
  startMs: number | null
  endMs: number | null
  elapsedMs: number | null
  traceAligned: boolean
  totalMatchedMs: number
  topTerms: TraceTerm[]
}

interface TraceRegionSummary {
  name: string
  startMs: number | null
  endMs: number | null
  elapsedMs: number | null
  attribution?: string
  evidence?: string[]
  traceWindow: TraceWindowSummary
}

interface BaselineReport {
  schemaVersion: 2
  generatedAt: string
  inputs: {
    smokePath: string
    tracePath: string
    traceFound: boolean
    traceAligned: boolean
    traceOffsetMs: number | null
  }
  source: QuickstartSmokeArtifact['source']
  scenario?: string
  collectedAt?: string
  finalURL?: string
  coldStartDriveSeed: {
    segment: StartupSegment | null
    adjacentDriveRenderSegment: StartupSegment | null
    quickstartSeedPhases: QuickstartPhase[]
    longestStartupSegments: StartupSegment[]
    runtimeTraceRegions: TraceRegionSummary[]
    traceWindow: TraceWindowSummary
  }
  postLoadAcceptedOpThroughput: {
    scenario?: string
    operationSemantics?: string
    operationTypeId?: string
    opCount?: number
    totalMs?: number
    opAvgMs?: number
    opMinMs?: number
    opMaxMs?: number
    opP50Ms?: number | null
    opP95Ms?: number | null
    opsPerSec?: number
    startingSeqno?: string
    endingSeqno?: string
    dominantOperationTerms: string[]
    slowestOps: AcceptedOperationTiming[]
    operationTraceRegions: TraceRegionSummary[]
    traceWindow: TraceWindowSummary
  }
}

function usage(): never {
  console.error(`Usage:
  bun scripts/tracetool-quickstart-baseline.ts [options]

Options:
  --smoke <path>      Quickstart smoke JSON artifact.
                      Default: .bldr/e2e-releasewasm/artifacts/${defaultTestName}.json
  --trace <path>      Chromium trace JSON artifact.
                      Default: .bldr/e2e-releasewasm/artifacts/${defaultTestName}.chromium-trace.json
  --out <path>        JSON report path.
                      Default: .bldr/e2e-releasewasm/artifacts/quickstart-tracetool-baseline.json
  --markdown <path>   Markdown report path.
                      Default: .bldr/e2e-releasewasm/artifacts/quickstart-tracetool-baseline.md
  --top <count>       Number of dominant terms to keep per trace window. Default: 12`)
  process.exit(2)
}

function parseArgs(args: string[]): CliOptions {
  const defaults = {
    smokePath: join(defaultArtifactDir, `${defaultTestName}.json`),
    tracePath: join(
      defaultArtifactDir,
      `${defaultTestName}.chromium-trace.json`,
    ),
    outPath: join(defaultArtifactDir, 'quickstart-tracetool-baseline.json'),
    markdownPath: join(defaultArtifactDir, 'quickstart-tracetool-baseline.md'),
    topCount: 12,
  }
  const opts = { ...defaults }
  for (let i = 0; i < args.length; i++) {
    const arg = args[i]
    const next = args[i + 1]
    switch (arg) {
      case '--smoke':
        if (!next) usage()
        opts.smokePath = next
        i++
        break
      case '--trace':
        if (!next) usage()
        opts.tracePath = next
        i++
        break
      case '--out':
        if (!next) usage()
        opts.outPath = next
        i++
        break
      case '--markdown':
        if (!next) usage()
        opts.markdownPath = next
        i++
        break
      case '--top':
        if (!next) usage()
        opts.topCount = Number(next)
        if (!Number.isInteger(opts.topCount) || opts.topCount <= 0) usage()
        i++
        break
      case '--help':
      case '-h':
        usage()
        break
      default:
        console.error(`unknown argument: ${arg}`)
        usage()
    }
  }
  return {
    smokePath: resolve(rootDir, opts.smokePath),
    tracePath: resolve(rootDir, opts.tracePath),
    outPath: resolve(rootDir, opts.outPath),
    markdownPath: resolve(rootDir, opts.markdownPath),
    topCount: opts.topCount,
  }
}

function readJson<T>(path: string): T {
  try {
    return JSON.parse(readFileSync(path, 'utf8')) as T
  } catch (err) {
    throw new Error(`read JSON ${path}: ${String(err)}`)
  }
}

function numberOrNull(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function roundMs(value: number): number {
  return Math.round(value * 1000) / 1000
}

function percentile(values: number[], p: number): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.min(
    sorted.length - 1,
    Math.ceil((p / 100) * sorted.length) - 1,
  )
  return roundMs(sorted[idx])
}

function segmentByName(
  smoke: QuickstartSmokeArtifact,
  name: string,
): StartupSegment | null {
  return (
    smoke.startupAttribution?.segments?.find((s) => s.name === name) ?? null
  )
}

function concreteSegment(segment: StartupSegment): StartupSegment {
  return {
    name: segment.name,
    startMs: numberOrNull(segment.startMs),
    endMs: numberOrNull(segment.endMs),
    elapsedMs: numberOrNull(segment.elapsedMs),
    attribution: segment.attribution,
    evidence: segment.evidence,
  }
}

function longestSegments(
  smoke: QuickstartSmokeArtifact,
  count: number,
): StartupSegment[] {
  return (smoke.startupAttribution?.segments ?? [])
    .filter((s) => typeof s.elapsedMs === 'number')
    .map(concreteSegment)
    .sort((a, b) => (b.elapsedMs ?? 0) - (a.elapsedMs ?? 0))
    .slice(0, count)
}

function seedPhases(
  smoke: QuickstartSmokeArtifact,
  startMs: number | null,
  endMs: number | null,
): QuickstartPhase[] {
  if (startMs === null || endMs === null) return []
  return (smoke.timing?.quickstart?.phases ?? [])
    .filter((phase) => {
      if (
        typeof phase.startedMs !== 'number' ||
        typeof phase.finishedMs !== 'number'
      ) {
        return false
      }
      return phase.finishedMs > startMs && phase.startedMs < endMs
    })
    .map((phase) => ({
      name: phase.name,
      startedMs: phase.startedMs,
      finishedMs: phase.finishedMs,
      elapsedMs: phase.elapsedMs,
    }))
    .sort((a, b) => (b.elapsedMs ?? 0) - (a.elapsedMs ?? 0))
}

function extractTraceEvents(tracePath: string): TraceEvent[] | null {
  if (!existsSync(tracePath)) return null
  const raw = readJson<{ traceEvents?: TraceEvent[] } | TraceEvent[]>(tracePath)
  if (Array.isArray(raw)) return raw
  return Array.isArray(raw.traceEvents) ? raw.traceEvents : []
}

function eventText(event: TraceEvent): string {
  const parts = [event.name ?? '', event.cat ?? '']
  if (event.args && typeof event.args === 'object') {
    const args = event.args as Record<string, unknown>
    const data = args.data
    if (data && typeof data === 'object') {
      const d = data as Record<string, unknown>
      parts.push(
        String(d.name ?? ''),
        String(d.message ?? ''),
        String(d.url ?? ''),
      )
    }
    parts.push(String(args.name ?? ''), String(args.message ?? ''))
  }
  return parts.filter(Boolean).join(' ')
}

function findTraceOffsetMs(
  events: TraceEvent[],
  startupMarks: StartupMark[],
): number | null {
  const markByFullName = new Map<string, number>()
  for (const mark of startupMarks) {
    if (typeof mark.startTimeMs !== 'number') continue
    if (mark.name) markByFullName.set(mark.name, mark.startTimeMs)
    if (mark.label)
      markByFullName.set(`spacewave.startup.${mark.label}`, mark.startTimeMs)
  }

  const offsets: number[] = []
  for (const event of events) {
    if (typeof event.ts !== 'number') continue
    const text = eventText(event)
    for (const [markName, startMs] of markByFullName) {
      if (text.includes(markName)) {
        offsets.push(event.ts / 1000 - startMs)
        break
      }
    }
  }
  if (offsets.length === 0) return null
  offsets.sort((a, b) => a - b)
  return roundMs(offsets[Math.floor(offsets.length / 2)])
}

function eventTermName(event: TraceEvent): string {
  const text = eventText(event)
  const runtimeTask = text.match(/\b(?:alpha|hydra)\/[a-z0-9/_-]+/i)
  if (runtimeTask) return runtimeTask[0]
  const startupMark = text.match(/spacewave\.startup\.[a-z0-9._-]+/i)
  if (startupMark) return startupMark[0]
  const quickstart = text.match(/\bquickstart[-./a-z0-9_]*/i)
  if (quickstart) return quickstart[0]
  return event.name || '(unnamed)'
}

function summarizeTraceWindow(
  name: string,
  events: TraceEvent[] | null,
  traceOffsetMs: number | null,
  startMs: number | null,
  endMs: number | null,
  topCount: number,
): TraceWindowSummary {
  const elapsedMs =
    typeof startMs === 'number' && typeof endMs === 'number' ?
      roundMs(Math.max(0, endMs - startMs))
    : null
  if (!events || traceOffsetMs === null || startMs === null || endMs === null) {
    return {
      name,
      startMs,
      endMs,
      elapsedMs,
      traceAligned: false,
      totalMatchedMs: 0,
      topTerms: [],
    }
  }

  const terms = new Map<string, TraceTerm & { tids: Set<string> }>()
  for (const event of events) {
    if (typeof event.ts !== 'number' || typeof event.dur !== 'number') continue
    if (event.dur <= 0) continue

    const eventStartMs = event.ts / 1000 - traceOffsetMs
    const eventEndMs = eventStartMs + event.dur / 1000
    const overlap =
      Math.min(eventEndMs, endMs) - Math.max(eventStartMs, startMs)
    if (overlap <= 0) continue

    const termName = eventTermName(event)
    const category = event.cat || ''
    const key = `${termName}\u0000${category}`
    let term = terms.get(key)
    if (!term) {
      term = {
        name: termName,
        category,
        eventCount: 0,
        totalMs: 0,
        selfMs: 0,
        maxMs: 0,
        threadCount: 0,
        tids: new Set<string>(),
      }
      terms.set(key, term)
    }
    const tid = `${event.pid ?? ''}:${event.tid ?? ''}`
    term.tids.add(tid)
    term.eventCount++
    term.totalMs += overlap
    term.selfMs +=
      typeof event.tdur === 'number' ?
        Math.min(overlap, event.tdur / 1000)
      : overlap
    term.maxMs = Math.max(term.maxMs, overlap)
  }

  const topTerms = [...terms.values()]
    .map((term) => ({
      name: term.name,
      category: term.category,
      eventCount: term.eventCount,
      totalMs: roundMs(term.totalMs),
      selfMs: roundMs(term.selfMs),
      maxMs: roundMs(term.maxMs),
      threadCount: term.tids.size,
    }))
    .sort(
      (a, b) =>
        b.totalMs - a.totalMs ||
        b.eventCount - a.eventCount ||
        a.name.localeCompare(b.name),
    )
    .slice(0, topCount)

  return {
    name,
    startMs,
    endMs,
    elapsedMs,
    traceAligned: true,
    totalMatchedMs: roundMs(
      topTerms.reduce((sum, term) => sum + term.totalMs, 0),
    ),
    topTerms,
  }
}

function traceRegionFromSegment(
  segment: StartupSegment,
  events: TraceEvent[] | null,
  traceOffsetMs: number | null,
  topCount: number,
): TraceRegionSummary {
  const concrete = concreteSegment(segment)
  const name = concrete.name ?? '(unnamed)'
  return {
    name,
    startMs: concrete.startMs ?? null,
    endMs: concrete.endMs ?? null,
    elapsedMs: concrete.elapsedMs ?? null,
    attribution: concrete.attribution,
    evidence: concrete.evidence,
    traceWindow: summarizeTraceWindow(
      name,
      events,
      traceOffsetMs,
      concrete.startMs ?? null,
      concrete.endMs ?? null,
      topCount,
    ),
  }
}

function traceRegionFromAcceptedOp(
  op: AcceptedOperationTiming,
  events: TraceEvent[] | null,
  traceOffsetMs: number | null,
  topCount: number,
): TraceRegionSummary {
  const name = `accepted-op-${op.ordinal ?? 'unknown'}`
  const startMs = numberOrNull(op.startedMs)
  const endMs = numberOrNull(op.finishedMs)
  const elapsedMs =
    typeof op.elapsedMs === 'number' ? roundMs(op.elapsedMs)
    : startMs !== null && endMs !== null ? roundMs(Math.max(0, endMs - startMs))
    : null
  return {
    name,
    startMs,
    endMs,
    elapsedMs,
    evidence: [`acceptedOperationTimings.${op.ordinal ?? 'unknown'}`],
    traceWindow: summarizeTraceWindow(
      name,
      events,
      traceOffsetMs,
      startMs,
      endMs,
      topCount,
    ),
  }
}

function runtimeTraceRegions(
  smoke: QuickstartSmokeArtifact,
  events: TraceEvent[] | null,
  traceOffsetMs: number | null,
  topCount: number,
): TraceRegionSummary[] {
  return (smoke.startupAttribution?.segments ?? [])
    .filter(
      (segment) =>
        typeof segment.startMs === 'number' &&
        typeof segment.endMs === 'number' &&
        typeof segment.elapsedMs === 'number',
    )
    .map((segment) =>
      traceRegionFromSegment(segment, events, traceOffsetMs, topCount),
    )
}

function operationTraceRegions(
  acceptedTimings: AcceptedOperationTiming[],
  events: TraceEvent[] | null,
  traceOffsetMs: number | null,
  topCount: number,
): TraceRegionSummary[] {
  return [...acceptedTimings]
    .filter(
      (op) =>
        typeof op.startedMs === 'number' && typeof op.finishedMs === 'number',
    )
    .sort((a, b) => (b.elapsedMs ?? 0) - (a.elapsedMs ?? 0))
    .slice(0, topCount)
    .sort((a, b) => (a.startedMs ?? 0) - (b.startedMs ?? 0))
    .map((op) => traceRegionFromAcceptedOp(op, events, traceOffsetMs, topCount))
}

function buildReport(opts: CliOptions): BaselineReport {
  if (!existsSync(opts.smokePath)) {
    throw new Error(`missing smoke artifact: ${opts.smokePath}`)
  }
  const smoke = readJson<QuickstartSmokeArtifact>(opts.smokePath)
  const traceEvents = extractTraceEvents(opts.tracePath)
  const traceOffsetMs =
    traceEvents ?
      findTraceOffsetMs(traceEvents, smoke.startupMarks ?? [])
    : null
  const traceAligned = traceEvents !== null && traceOffsetMs !== null

  const seedSegment = segmentByName(smoke, 'quickstart-content-seed')
  const renderSegment = segmentByName(smoke, 'frame-ready-to-content-ready')
  const postLoad = smoke.postLoadSharedObjectWorkload ?? {}
  const acceptedTimings = postLoad.acceptedOperationTimings ?? []
  const elapsedOps = acceptedTimings
    .map((op) => op.elapsedMs)
    .filter((value): value is number => typeof value === 'number')

  const seedStartMs = numberOrNull(seedSegment?.startMs)
  const seedEndMs = numberOrNull(seedSegment?.endMs)
  const postLoadStartMs = numberOrNull(postLoad.startedMs)
  const postLoadEndMs = numberOrNull(postLoad.finishedMs)

  return {
    schemaVersion: 2,
    generatedAt: new Date().toISOString(),
    inputs: {
      smokePath: opts.smokePath,
      tracePath: opts.tracePath,
      traceFound: traceEvents !== null,
      traceAligned,
      traceOffsetMs,
    },
    source: smoke.source,
    scenario: smoke.scenario,
    collectedAt: smoke.collectedAt,
    finalURL: smoke.finalURL,
    coldStartDriveSeed: {
      segment: seedSegment ? concreteSegment(seedSegment) : null,
      adjacentDriveRenderSegment:
        renderSegment ? concreteSegment(renderSegment) : null,
      quickstartSeedPhases: seedPhases(smoke, seedStartMs, seedEndMs),
      longestStartupSegments: longestSegments(smoke, 6),
      runtimeTraceRegions: runtimeTraceRegions(
        smoke,
        traceEvents,
        traceOffsetMs,
        opts.topCount,
      ),
      traceWindow: summarizeTraceWindow(
        'quickstart-content-seed',
        traceEvents,
        traceOffsetMs,
        seedStartMs,
        seedEndMs,
        opts.topCount,
      ),
    },
    postLoadAcceptedOpThroughput: {
      scenario: postLoad.scenario,
      operationSemantics: postLoad.operationSemantics,
      operationTypeId: postLoad.operationTypeId,
      opCount: postLoad.opCount,
      totalMs: postLoad.totalMs,
      opAvgMs: postLoad.opAvgMs,
      opMinMs: postLoad.opMinMs,
      opMaxMs: postLoad.opMaxMs,
      opP50Ms: percentile(elapsedOps, 50),
      opP95Ms: percentile(elapsedOps, 95),
      opsPerSec: postLoad.opsPerSec,
      startingSeqno: postLoad.startingSeqno,
      endingSeqno: postLoad.endingSeqno,
      dominantOperationTerms: [
        postLoad.operationTypeId,
        postLoad.operationSemantics,
        postLoad.scenario,
        'acceptedOperationTimings',
      ].filter(
        (term): term is string => typeof term === 'string' && term !== '',
      ),
      slowestOps: [...acceptedTimings]
        .sort((a, b) => (b.elapsedMs ?? 0) - (a.elapsedMs ?? 0))
        .slice(0, 5),
      operationTraceRegions: operationTraceRegions(
        acceptedTimings,
        traceEvents,
        traceOffsetMs,
        opts.topCount,
      ),
      traceWindow: summarizeTraceWindow(
        'post-load-accepted-op-throughput',
        traceEvents,
        traceOffsetMs,
        postLoadStartMs,
        postLoadEndMs,
        opts.topCount,
      ),
    },
  }
}

function formatMs(value: number | null | undefined): string {
  return typeof value === 'number' ? `${roundMs(value).toFixed(3)} ms` : 'n/a'
}

function formatNumber(value: number | null | undefined): string {
  return typeof value === 'number' ? roundMs(value).toString() : 'n/a'
}

function markdownTerms(terms: TraceTerm[]): string {
  if (terms.length === 0)
    return 'No aligned Chromium trace terms were available for this window.\n'
  const rows = [
    '| term | category | total | events | max | threads |',
    '| --- | --- | ---: | ---: | ---: | ---: |',
    ...terms.map(
      (term) =>
        `| ${escapeCell(term.name)} | ${escapeCell(term.category)} | ${formatMs(term.totalMs)} | ${term.eventCount} | ${formatMs(term.maxMs)} | ${term.threadCount} |`,
    ),
  ]
  return `${rows.join('\n')}\n`
}

function markdownPhases(phases: QuickstartPhase[]): string {
  if (phases.length === 0) return 'No Quickstart seed phases were recorded.\n'
  const rows = [
    '| phase | elapsed | start | finish |',
    '| --- | ---: | ---: | ---: |',
    ...phases.map(
      (phase) =>
        `| ${escapeCell(phase.name ?? '')} | ${formatMs(phase.elapsedMs)} | ${formatMs(phase.startedMs)} | ${formatMs(phase.finishedMs)} |`,
    ),
  ]
  return `${rows.join('\n')}\n`
}

function markdownSegments(segments: StartupSegment[]): string {
  if (segments.length === 0)
    return 'No startup attribution segments were measured.\n'
  const rows = [
    '| segment | elapsed | attribution |',
    '| --- | ---: | --- |',
    ...segments.map(
      (segment) =>
        `| ${escapeCell(segment.name ?? '')} | ${formatMs(segment.elapsedMs)} | ${escapeCell(segment.attribution ?? '')} |`,
    ),
  ]
  return `${rows.join('\n')}\n`
}

function markdownTraceRegions(regions: TraceRegionSummary[]): string {
  if (regions.length === 0)
    return 'No narrowed trace regions were measured for this window.\n'
  const rows = [
    '| region | elapsed | top trace term | top term total | events | attribution |',
    '| --- | ---: | --- | ---: | ---: | --- |',
    ...regions.map((region) => {
      const topTerm = region.traceWindow.topTerms[0] ?? null
      return `| ${escapeCell(region.name)} | ${formatMs(region.elapsedMs)} | ${escapeCell(topTerm?.name ?? 'n/a')} | ${formatMs(topTerm?.totalMs)} | ${
        topTerm?.eventCount ?? 'n/a'
      } | ${escapeCell(region.attribution ?? '')} |`
    }),
  ]
  return `${rows.join('\n')}\n`
}

function escapeCell(value: string): string {
  return value.replaceAll('|', '\\|').replaceAll('\n', ' ')
}

function toMarkdown(report: BaselineReport): string {
  const seed = report.coldStartDriveSeed.segment
  const render = report.coldStartDriveSeed.adjacentDriveRenderSegment
  const post = report.postLoadAcceptedOpThroughput
  const traceState =
    report.inputs.traceFound ?
      report.inputs.traceAligned ?
        `aligned with offset ${formatMs(report.inputs.traceOffsetMs)}`
      : 'found but not aligned to performance marks'
    : 'not found'

  return `# Quickstart Tracetool Baseline

Generated: ${report.generatedAt}
Scenario: ${report.scenario ?? 'n/a'}
Source HEAD: ${report.source?.head ?? 'n/a'}
Source dirty: ${String(report.source?.dirty ?? false)}
Chromium trace: ${traceState}

## Cold-start Drive Seed

Seed segment: ${seed?.name ?? 'quickstart-content-seed'} (${formatMs(seed?.elapsedMs)}, ${formatMs(seed?.startMs)} to ${formatMs(seed?.endMs)})

Adjacent Drive render segment: ${render?.name ?? 'frame-ready-to-content-ready'} (${formatMs(render?.elapsedMs)})

Dominant startup segments:

${markdownSegments(report.coldStartDriveSeed.longestStartupSegments)}
Dominant Quickstart seed phases:

${markdownPhases(report.coldStartDriveSeed.quickstartSeedPhases)}
Dominant concrete trace terms during Drive seed:

${markdownTerms(report.coldStartDriveSeed.traceWindow.topTerms)}
Narrowed runtime trace regions:

${markdownTraceRegions(report.coldStartDriveSeed.runtimeTraceRegions)}
## Post-load Accepted-op Throughput

Scenario: ${post.scenario ?? 'n/a'}
Operation type: ${post.operationTypeId ?? 'n/a'}
Operation semantics: ${post.operationSemantics ?? 'n/a'}
Accepted operations: ${post.opCount ?? 'n/a'}
Total: ${formatMs(post.totalMs)}
Average: ${formatMs(post.opAvgMs)}
P50: ${formatMs(post.opP50Ms)}
P95: ${formatMs(post.opP95Ms)}
Min/Max: ${formatMs(post.opMinMs)} / ${formatMs(post.opMaxMs)}
Ops/sec: ${formatNumber(post.opsPerSec)}
Seqno: ${post.startingSeqno ?? 'n/a'} -> ${post.endingSeqno ?? 'n/a'}
Dominant operation terms: ${post.dominantOperationTerms.join(', ') || 'n/a'}

Slowest accepted operations:

${markdownOps(post.slowestOps)}
Dominant concrete trace terms during post-load accepted-op throughput:

${markdownTerms(post.traceWindow.topTerms)}
Narrowed accepted-op trace regions:

${markdownTraceRegions(post.operationTraceRegions)}
`
}

function markdownOps(ops: AcceptedOperationTiming[]): string {
  if (ops.length === 0) return 'No accepted operation timings were recorded.\n'
  const rows = [
    '| op | elapsed | start | finish | seqno |',
    '| ---: | ---: | ---: | ---: | --- |',
    ...ops.map(
      (op) =>
        `| ${op.ordinal ?? ''} | ${formatMs(op.elapsedMs)} | ${formatMs(op.startedMs)} | ${formatMs(op.finishedMs)} | ${escapeCell(op.seqno ?? '')} |`,
    ),
  ]
  return `${rows.join('\n')}\n`
}

async function main(): Promise<void> {
  const opts = parseArgs(process.argv.slice(2))
  const report = buildReport(opts)
  mkdirSync(dirname(opts.outPath), { recursive: true })
  writeFileSync(opts.outPath, `${JSON.stringify(report, null, 2)}\n`)
  mkdirSync(dirname(opts.markdownPath), { recursive: true })
  writeFileSync(opts.markdownPath, toMarkdown(report))
  console.log(`wrote ${opts.outPath}`)
  console.log(`wrote ${opts.markdownPath}`)
}

await main()
