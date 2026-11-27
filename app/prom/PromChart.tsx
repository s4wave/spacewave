import { useCallback, useMemo, type PointerEvent } from 'react'

import type { PanelUnit, Series } from '@s4wave/sdk/prom/prom.pb.js'

import {
  formatClock,
  formatValue,
  niceTicks,
  seriesColor,
} from './prom-view.js'

// chartHeight is the height of every chart, so panels never shift as they
// load. The panel's placeholders match it with h-40.
const chartHeight = 160

// marginLeft, marginBottom and marginTop are the margins around the plot area
// for axis labels.
const marginLeft = 56
const marginBottom = 18
const marginTop = 6

export interface PromChartProps {
  // series holds the range query's series in legend order.
  series: readonly Series[]
  // hidden holds the indexes of series the legend turned off.
  hidden: ReadonlySet<number>
  startMs: number
  endMs: number
  // stepMs breaks a line where samples are more than one step apart.
  stepMs: number
  unit: PanelUnit
  width: number
  // hoverMs is the Dashboard's shared hover time, or null.
  hoverMs: number | null
  onHover: (ms: number | null) => void
}

// PromChart draws a range query as SVG lines with one y axis, a time axis and
// the Dashboard's shared hover crosshair. A step of the range over the width
// gives at most one point per pixel, so SVG paths stay small.
export function PromChart({
  series,
  hidden,
  startMs,
  endMs,
  stepMs,
  unit,
  width,
  hoverMs,
  onHover,
}: PromChartProps) {
  const plotWidth = Math.max(width - marginLeft, 1)
  const plotHeight = chartHeight - marginBottom - marginTop

  // Fit the y axis to the shown series, starting at zero for positive data.
  const yTicks = useMemo(() => {
    let min = 0
    let max = 0
    series.forEach((s, i) => {
      if (hidden.has(i)) return
      for (const sample of s.samples ?? []) {
        const v = sample.value ?? 0
        if (!Number.isFinite(v)) continue
        min = Math.min(min, v)
        max = Math.max(max, v)
      }
    })
    if (max === min) max = min + 1
    return niceTicks(min, max, 4)
  }, [series, hidden])
  const yMin = yTicks[0]
  const yMax = Math.max(yTicks[yTicks.length - 1], yMin + 1)
  const x = useCallback(
    (ms: number) =>
      marginLeft + ((ms - startMs) / (endMs - startMs)) * plotWidth,
    [startMs, endMs, plotWidth],
  )
  const y = useCallback(
    (v: number) => marginTop + (1 - (v - yMin) / (yMax - yMin)) * plotHeight,
    [yMin, yMax, plotHeight],
  )

  // Draw each series as one path, broken where samples are missing.
  const paths = useMemo(
    () =>
      series.map((s) => {
        let d = ''
        let prev = -Infinity
        for (const sample of s.samples ?? []) {
          const t = Number(sample.timeMs ?? 0n)
          const v = sample.value ?? 0
          if (!Number.isFinite(v)) {
            prev = -Infinity
            continue
          }
          const cmd = t - prev > stepMs * 1.5 ? 'M' : 'L'
          d += `${cmd}${x(t).toFixed(1)},${y(v).toFixed(1)}`
          prev = t
        }
        return d
      }),
    [series, stepMs, x, y],
  )

  // Label the time axis at round intervals.
  const withDate = endMs - startMs > 24 * 60 * 60_000
  const xTicks = useMemo(() => {
    const count = Math.max(2, Math.floor(plotWidth / 110))
    const span = (endMs - startMs) / count
    return Array.from({ length: count }, (_, i) => startMs + span * (i + 0.5))
  }, [startMs, endMs, plotWidth])

  // Report the hovered time to the Dashboard.
  const handleMove = useCallback(
    (e: PointerEvent<SVGSVGElement>) => {
      const rect = e.currentTarget.getBoundingClientRect()
      const px = e.clientX - rect.left - marginLeft
      if (px < 0) {
        onHover(null)
        return
      }
      onHover(startMs + (px / plotWidth) * (endMs - startMs))
    },
    [onHover, startMs, endMs, plotWidth],
  )
  const handleLeave = useCallback(() => onHover(null), [onHover])

  const showHover = hoverMs !== null && hoverMs >= startMs && hoverMs <= endMs
  return (
    <svg
      width={width}
      height={chartHeight}
      className="block touch-none select-none"
      onPointerMove={handleMove}
      onPointerLeave={handleLeave}
      role="img"
    >
      {yTicks.map((v) => (
        <g key={v}>
          <line
            x1={marginLeft}
            x2={width}
            y1={y(v)}
            y2={y(v)}
            className="stroke-foreground/8"
          />
          <text
            x={marginLeft - 6}
            y={y(v)}
            dy="0.32em"
            textAnchor="end"
            className="fill-foreground-alt text-metadata tabular-nums"
          >
            {formatValue(v, unit)}
          </text>
        </g>
      ))}
      {xTicks.map((ms) => (
        <text
          key={ms}
          x={x(ms)}
          y={chartHeight - 4}
          textAnchor="middle"
          className="fill-foreground-alt text-metadata tabular-nums"
        >
          {formatClock(ms, withDate)}
        </text>
      ))}
      {paths.map((d, i) =>
        hidden.has(i) ? null : (
          <path
            key={i}
            d={d}
            fill="none"
            stroke={seriesColor(i)}
            strokeWidth={2}
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        ),
      )}
      {showHover ? (
        <line
          x1={x(hoverMs)}
          x2={x(hoverMs)}
          y1={marginTop}
          y2={marginTop + plotHeight}
          className="stroke-foreground/40"
        />
      ) : null}
    </svg>
  )
}
