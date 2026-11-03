import { useMemo } from 'react'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { PromQuery, PromTargets } from '@s4wave/sdk/prom/prom.js'
import { PanelKind, type Panel } from '@s4wave/sdk/prom/prom.pb.js'
import { useContainerDensity } from '@s4wave/web/hooks/useContainerDensity.js'

import {
  PromPanelView,
  type PanelActions,
  type PanelAnswer,
  type StaleTarget,
} from './PromPanelView.js'
import {
  minScrapeIntervalMs,
  panelStepMs,
  staleTargets,
  type TimeRange,
} from './prom-view.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({ rule1: { minWidth: 0 } })

// widthBucket rounds the measured width so a resize reruns the query only
// when the step would change noticeably.
const widthBucket = 100

export interface PromPanelProps {
  query: PromQuery
  panel: Panel
  range: TimeRange
  // targets is the latest target set; each new set reruns the query, since a
  // target changes on each scrape commit.
  targets: PromTargets | null
  // nowMs is the wall clock, for naming targets that stopped scraping.
  nowMs: number
  hoverMs: number | null
  onHover: (ms: number | null) => void
  onOpenTarget?: (objectKey: string) => void
  actions?: PanelActions
}

// PromPanel runs a panel's expression over the time range, again after each
// target commit, and shows the answer. The previous answer stays on screen
// while the next one runs.
export function PromPanel({
  query,
  panel,
  range,
  targets,
  nowMs,
  hoverMs,
  onHover,
  onOpenTarget,
  actions,
}: PromPanelProps) {
  const { ref, width, measured } = useContainerDensity()
  const bucket = Math.ceil(width / widthBucket) * widthBucket
  const isValue = panel.kind === PanelKind.VALUE
  const minStep = minScrapeIntervalMs(targets)
  const expr = panel.expr ?? ''

  // Rerun on each change of the expression, range, width or targets.
  const result = useResource(
    async (signal) => {
      if (!measured || !expr) return null
      const endMs = Date.now()
      const startMs = endMs - range.ms
      const stepMs = isValue ? 0 : panelStepMs(range.ms, bucket, minStep)
      const resp = await query.query(
        isValue
          ? { expr }
          : {
              expr,
              startMs: BigInt(startMs),
              endMs: BigInt(endMs),
              stepMs: BigInt(stepMs),
            },
        signal,
      )
      return { resp, startMs, endMs, stepMs }
    },
    [query, expr, isValue, range.ms, bucket, minStep, measured, targets],
  )

  // Map the resource to the panel's answer state.
  const answer = useMemo<PanelAnswer>(() => {
    if (result.error) return { state: 'error', message: result.error.message }
    if (!result.value) return { state: 'loading' }
    return { state: 'answered', ...result.value, refreshing: result.loading }
  }, [result.error, result.value, result.loading])

  // Name the stale targets the answer's series came from.
  const stale = useMemo<StaleTarget[]>(() => {
    if (!result.value) return []
    return staleTargets(targets, result.value.resp.series ?? [], nowMs).map(
      (objectKey) => ({
        objectKey,
        lastScrapeMs: Number(
          targets?.get(objectKey)?.status?.lastScrapeMs ?? 0n,
        ),
      }),
    )
  }, [targets, result.value, nowMs])

  return (
    <div ref={ref} sx={stylexStyles.rule1}>
      <PromPanelView
        panel={panel}
        answer={expr ? answer : { state: 'error', message: 'empty expression' }}
        rangeLabel={range.id}
        stale={stale}
        nowMs={nowMs}
        width={Math.max(width - 10, 0)}
        hoverMs={hoverMs}
        onHover={onHover}
        onOpenTarget={onOpenTarget}
        actions={actions}
      />
    </div>
  )
}
