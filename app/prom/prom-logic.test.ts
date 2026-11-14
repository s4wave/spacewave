import { describe, expect, it } from 'vitest'

import type { TargetInfo } from '@s4wave/sdk/prom/prom.pb.js'

import { suggestContext } from './expr-suggest.js'
import { formatLabels, seriesNames } from './prom-view.js'
import { targetAction, targetState } from './target-state.js'

describe('suggestContext', () => {
  it('reads metric and label positions and skips values', () => {
    const at = (expr: string) => suggestContext(expr, expr.length)
    expect(at('rate(node_cp')).toEqual({
      kind: 'metric',
      prefix: 'node_cp',
      start: 5,
    })
    expect(at('rate(node_cpu{mo')).toEqual({
      kind: 'label',
      metric: 'node_cpu',
      prefix: 'mo',
      start: 14,
    })
    expect(at('node_cpu{mode="idle", cp')).toMatchObject({
      kind: 'label',
      metric: 'node_cpu',
      prefix: 'cp',
    })
    expect(at('node_cpu{mode="id')).toEqual({ kind: 'none' })
    expect(at('node_cpu{mode=id')).toEqual({ kind: 'none' })
    expect(at('node_cpu{mode="a{b}"} + up')).toMatchObject({
      kind: 'metric',
      prefix: 'up',
    })
    expect(at('rate(x[5')).toEqual({ kind: 'none' })
  })
})

describe('targetState', () => {
  it('moves a silent writer to stale and a standby to waiting', () => {
    const scrapedMs = 1_000_000
    const info: TargetInfo = {
      meta: { scrape: { intervalMs: 15_000 } },
      status: {
        up: true,
        lastScrapeMs: BigInt(scrapedMs),
        devicePeerId: 'writer',
        commitWindowMs: 60_000,
      },
    }
    const fresh = scrapedMs + 10_000
    const silent = scrapedMs + 2 * 15_000 + 60_000 + 1

    expect(targetState(undefined, false, 'me', fresh)).toEqual({
      state: 'unapproved',
    })
    expect(targetState(info, false, 'me', fresh).state).toBe('up')
    expect(
      targetAction(targetState(info, false, 'me', fresh), false, true),
    ).toBe(null)

    const stale = targetState(info, false, 'me', silent)
    expect(stale.state).toBe('stale')
    expect(targetAction(stale, false, true)).toBe('take-over')
    expect(targetAction(stale, false, false)).toBe(null)

    // An approved Device that is not the writer is the standby.
    const standby = targetState(info, true, 'me', silent)
    expect(standby).toEqual({ state: 'waiting' })
    expect(targetAction(standby, true, true)).toBe('stop')
    // The writer itself going silent stays stale.
    expect(targetState(info, true, 'writer', silent).state).toBe('stale')
  })
})

describe('formatLabels', () => {
  it('quotes each value as a PromQL string', () => {
    // A backslash, a double quote and a newline are escaped, so the selector
    // parses back to the same values.
    const labels = {
      labels: [
        { name: 'path', value: 'C:\\logs' },
        { name: 'msg', value: 'say "hi"\nbye' },
      ],
    }
    expect(formatLabels(labels)).toBe(
      '{path="C:\\\\logs", msg="say \\"hi\\"\\nbye"}',
    )
    expect(
      seriesNames([
        { labels },
        { labels: { labels: [{ name: 'path', value: 'a"b' }] } },
      ]),
    ).toEqual(['path="C:\\\\logs"', 'path="a\\"b"'])
  })
})
