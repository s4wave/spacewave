import {
  PanelUnit,
  type LabelSet,
  type Series,
} from '@s4wave/sdk/prom/prom.pb.js'
import type { PromTargets } from '@s4wave/sdk/prom/prom.js'
import { isTargetStale, labelValue } from '@s4wave/sdk/prom/prom.js'

// TimeRange is one of the Dashboard's time range presets.
export interface TimeRange {
  id: string
  ms: number
}

// timeRanges are the presets in display order.
export const timeRanges: readonly TimeRange[] = [
  { id: '15m', ms: 15 * 60_000 },
  { id: '1h', ms: 60 * 60_000 },
  { id: '6h', ms: 6 * 60 * 60_000 },
  { id: '24h', ms: 24 * 60 * 60_000 },
  { id: '7d', ms: 7 * 24 * 60 * 60_000 },
]

// defaultTimeRange is the range of a Dashboard path that names none.
export const defaultTimeRange = timeRanges[1]

// clockPeriodMs is how often the viewers advance the wall clock that moves a
// silent target to stale and ages "scraped ago" text.
export const clockPeriodMs = 5_000

// parseTimeRange reads the time range from a viewer path such as /6h.
export function parseTimeRange(path: string): TimeRange {
  const id = path.replace(/^\/+|\/+$/g, '')
  return timeRanges.find((r) => r.id === id) ?? defaultTimeRange
}

// seriesPalette is the categorical palette of chart lines, validated against
// the app's dark surface. Series take colors in this order and never cycle.
export const seriesPalette: readonly string[] = [
  '#3987e5',
  '#d95926',
  '#199e70',
  '#c98500',
  '#d55181',
  '#008300',
  '#9085e9',
  '#e66767',
]

// overflowColor draws series past the palette's end.
export const overflowColor = '#6b6b6b'

// seriesColor returns the color of the series at index.
export function seriesColor(index: number): string {
  return seriesPalette[index] ?? overflowColor
}

// panelStepMs returns the step of a range query over rangeMs drawn widthPx
// wide: one point per pixel, but no finer than minStepMs.
export function panelStepMs(
  rangeMs: number,
  widthPx: number,
  minStepMs: number,
): number {
  const perPixel = Math.ceil(rangeMs / Math.max(widthPx, 1))
  return Math.max(perPixel, minStepMs, 1000)
}

// minScrapeIntervalMs returns the shortest scrape interval among targets,
// or 15 seconds when there are none.
export function minScrapeIntervalMs(targets: PromTargets | null): number {
  let min = 0
  for (const info of targets?.values() ?? []) {
    const interval = info.meta?.scrape?.intervalMs || 15_000
    if (min === 0 || interval < min) min = interval
  }
  return min || 15_000
}

// seriesNames returns a name for each series: the labels whose values differ
// among them, or the metric name when only one series is shown.
export function seriesNames(series: readonly Series[]): string[] {
  // Find the labels that tell the series apart.
  const values = new Map<string, Set<string>>()
  for (const s of series) {
    for (const l of s.labels?.labels ?? []) {
      const name = l.name ?? ''
      let set = values.get(name)
      if (!set) {
        set = new Set()
        values.set(name, set)
      }
      set.add(l.value ?? '')
    }
  }
  const distinct = [...values.entries()]
    .filter(([, set]) => set.size > 1)
    .map(([name]) => name)

  // Name each series by those labels.
  return series.map((s) => {
    if (distinct.length === 0) {
      return labelValue(s.labels, '__name__') || formatLabels(s.labels)
    }
    return distinct
      .map((name) => labelMatcher(name, labelValue(s.labels, name)))
      .join(', ')
  })
}

// formatLabels writes a label set in PromQL selector form.
export function formatLabels(set: LabelSet | undefined): string {
  const labels = set?.labels ?? []
  if (labels.length === 0) return '{}'
  return `{${labels.map((l) => labelMatcher(l.name ?? '', l.value ?? '')).join(', ')}}`
}

// labelMatcher writes name="value" with the value quoted as a PromQL string:
// backslash, double quote and newline escaped, so the selector parses back to
// the same value.
function labelMatcher(name: string, value: string): string {
  const quoted = value
    .replaceAll('\\', '\\\\')
    .replaceAll('"', '\\"')
    .replaceAll('\n', '\\n')
  return `${name}="${quoted}"`
}

// staleTargets returns the stale targets whose labels every series of
// series carries, so the panel can name them.
export function staleTargets(
  targets: PromTargets | null,
  series: readonly Series[],
  nowMs: number,
): string[] {
  const out: string[] = []
  for (const [key, info] of targets ?? []) {
    if (!isTargetStale(info, nowMs)) continue
    const own = info.meta?.labels?.labels ?? []
    const matches = series.some((s) =>
      own.every((l) => labelValue(s.labels, l.name ?? '') === l.value),
    )
    if (matches) out.push(key)
  }
  return out
}

// targetName returns a short name for a target object key.
export function targetName(objectKey: string): string {
  return objectKey.replace(/^prom\/target\//, '')
}

// formatValue formats value in unit for a label or readout.
export function formatValue(value: number, unit: PanelUnit): string {
  if (!Number.isFinite(value)) return String(value)
  switch (unit) {
    case PanelUnit.PERCENT:
      return `${trim(value)}%`
    case PanelUnit.BYTES:
      return formatBytes(value)
    case PanelUnit.BYTES_PER_SECOND:
      return `${formatBytes(value)}/s`
    case PanelUnit.SECONDS:
      return formatSeconds(value)
    case PanelUnit.CELSIUS:
      return `${trim(value)} °C`
    default:
      return formatPlain(value)
  }
}

// trim rounds value to at most three significant decimals.
function trim(value: number): string {
  const abs = Math.abs(value)
  const digits = abs >= 100 ? 0 : abs >= 10 ? 1 : 2
  return value.toFixed(digits).replace(/\.?0+$/, '')
}

// byteUnits are the binary size prefixes.
const byteUnits = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

// formatBytes formats a size with a binary prefix.
function formatBytes(value: number): string {
  let i = 0
  let v = value
  while (Math.abs(v) >= 1024 && i < byteUnits.length - 1) {
    v /= 1024
    i++
  }
  return `${trim(v)} ${byteUnits[i]}`
}

// formatSeconds formats a duration from microseconds to days.
function formatSeconds(value: number): string {
  const abs = Math.abs(value)
  if (abs === 0) return '0 s'
  if (abs < 1e-3) return `${trim(value * 1e6)} µs`
  if (abs < 1) return `${trim(value * 1e3)} ms`
  if (abs < 120) return `${trim(value)} s`
  if (abs < 7200) return `${trim(value / 60)} min`
  if (abs < 172800) return `${trim(value / 3600)} h`
  return `${trim(value / 86400)} d`
}

// formatPlain formats a unitless value with an SI suffix past a thousand.
function formatPlain(value: number): string {
  const abs = Math.abs(value)
  if (abs >= 1e9) return `${trim(value / 1e9)}G`
  if (abs >= 1e6) return `${trim(value / 1e6)}M`
  if (abs >= 1e3) return `${trim(value / 1e3)}k`
  return trim(value)
}

// formatClock formats a Unix millisecond time as a local time of day, with
// the date when the range spans more than a day.
export function formatClock(ms: number, withDate = false): string {
  const d = new Date(ms)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  if (!withDate) return time
  return `${d.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${time}`
}

// formatAgo formats the time since ms as a short age such as 41m.
export function formatAgo(ms: number, nowMs: number): string {
  const s = Math.max(0, Math.round((nowMs - ms) / 1000))
  if (s < 90) return `${s}s`
  const m = Math.round(s / 60)
  if (m < 90) return `${m}m`
  const h = Math.round(m / 60)
  if (h < 48) return `${h}h`
  return `${Math.round(h / 24)}d`
}

// niceTicks returns about count round values spanning min to max.
export function niceTicks(min: number, max: number, count: number): number[] {
  if (!(max > min)) return [min]
  const raw = (max - min) / count
  const mag = 10 ** Math.floor(Math.log10(raw))
  const step =
    [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? raw
  const ticks: number[] = []
  for (let v = Math.ceil(min / step) * step; v <= max + step / 1e6; v += step) {
    ticks.push(Math.abs(v) < step / 1e6 ? 0 : v)
  }
  return ticks
}

// shortPeer returns the last characters of a peer ID.
export function shortPeer(peerId: string): string {
  return peerId.length > 8 ? `…${peerId.slice(-8)}` : peerId
}
