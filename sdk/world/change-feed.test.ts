import { describe, expect, it, vi } from 'vitest'

import {
  ChangeFeed,
  changePattern,
  type ChangeFeedEvent,
} from './change-feed.js'
import type { Quad } from '../../db/block/quad/quad.pb.js'
import type { WatchChangesRequest, WatchChangesResponse } from './world.pb.js'

// fakeServer stands in for the WatchChanges stream: it records each request
// and sends the responses the test pushes.
function fakeServer() {
  const requests: WatchChangesRequest[] = []
  const opens: AbortSignal[] = []
  let push: ((resp: WatchChangesResponse | Error) => void) | undefined
  const feed = new ChangeFeed((reqs, signal) => {
    opens.push(signal)
    void (async () => {
      for await (const req of reqs) requests.push(req)
    })()
    const queue: (WatchChangesResponse | Error)[] = []
    let wake: (() => void) | undefined
    push = (resp) => {
      queue.push(resp)
      wake?.()
    }
    return (async function* () {
      for (;;) {
        const next = queue.shift()
        if (next instanceof Error) throw next
        if (next) {
          yield next
          continue
        }
        if (signal.aborted) return
        await new Promise<void>((resolve) => {
          wake = resolve
          signal.addEventListener('abort', () => resolve(), { once: true })
        })
      }
    })()
  })
  return {
    feed,
    requests,
    opens,
    change: (seqno: bigint, keys: string[] = [], unknown = false) =>
      push?.({ body: { case: 'change', value: { seqno, keys, unknown } } }),
    link: (seqno: bigint, quad: Quad) =>
      push?.({ body: { case: 'change', value: { seqno, quads: [quad] } } }),
    ack: (version: bigint, afterSeqno: bigint) =>
      push?.({ body: { case: 'interestAck', value: { version, afterSeqno } } }),
    fail: (err: Error) => push?.(err),
  }
}

// settle lets queued microtasks and stream reads run.
const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

// reader collects a subscription's events as they arrive.
function reader(sub: AsyncIterable<ChangeFeedEvent>) {
  const events: ChangeFeedEvent[] = []
  const done = (async () => {
    for await (const event of sub) events.push(event)
  })()
  return { events, done }
}

describe('ChangeFeed', () => {
  it('shares one stream and sends each subscriber only its changes', async () => {
    const s = fakeServer()
    const a = reader(s.feed.subscribe(5n, { keys: ['a'] }))
    const b = reader(s.feed.subscribe(5n, { keys: ['b'] }))
    await settle()

    // One stream: the first request opens it, the second adds b as a delta.
    expect(s.opens).toHaveLength(1)
    expect(s.requests).toEqual([
      {
        afterSeqno: 5n,
        interest: { version: 1n, addKeys: ['a'], addPatterns: [] },
      },
      {
        interest: {
          version: 2n,
          addKeys: ['b'],
          removeKeys: [],
          addPatterns: [],
          removePatterns: [],
        },
      },
    ])

    // b is followed after the ack. Each change goes only to the subscriber
    // the change references.
    s.ack(2n, 6n)
    s.change(7n, ['a'])
    s.change(8n, ['b'])
    s.change(9n, [], true)
    await settle()
    expect(a.events.map((e) => e.case)).toEqual(['change', 'change'])
    expect(b.events).toEqual([
      { case: 'followed', afterSeqno: 6n },
      { case: 'change', change: { seqno: 8n, keys: ['b'], unknown: false } },
      { case: 'change', change: { seqno: 9n, keys: [], unknown: true } },
    ])
  })

  it('sends one delta for a burst of updates and closes with the last', async () => {
    const s = fakeServer()
    const sub = s.feed.subscribe(1n, { keys: ['a'] })
    sub.update({ keys: ['b'] })
    sub.update({ keys: ['c'] }, { keys: ['a'] })
    await settle()
    expect(s.requests).toHaveLength(2)
    expect(s.requests[1].interest).toEqual({
      version: 2n,
      addKeys: ['b', 'c'],
      removeKeys: ['a'],
      addPatterns: [],
      removePatterns: [],
    })

    sub.close()
    expect(s.opens[0].aborted).toBe(true)
  })

  it('matches a member by pattern without naming it', async () => {
    const s = fakeServer()
    const pattern = changePattern({ subject: '<origin>', predicate: '<rel>' })
    const sub = reader(s.feed.subscribe(1n, { patterns: [pattern] }))
    await settle()
    expect(s.requests[0].interest?.addPatterns).toEqual([
      { subject: '<origin>', predicate: '<rel>', obj: '', label: '' },
    ])
    const link = { subject: '<origin>', predicate: '<rel>', obj: '<member>' }
    s.change(2n, ['member'])
    s.link(3n, { subject: '<other>', predicate: '<rel>', obj: '<member>' })
    s.link(4n, link)
    await settle()
    expect(sub.events).toEqual([
      { case: 'change', change: { seqno: 4n, quads: [link] } },
    ])
  })

  it('fails every subscriber when the stream fails', async () => {
    const s = fakeServer()
    const a = reader(s.feed.subscribe(1n, { keys: ['a'] }))
    const b = reader(s.feed.subscribe(1n, { keys: ['b'] }))
    await settle()
    const err = new Error('stream reset')
    s.fail(err)
    await expect(a.done).rejects.toBe(err)
    await expect(b.done).rejects.toBe(err)

    // The next subscriber opens a new stream with its whole interest.
    s.feed.subscribe(4n, { keys: ['a', 'b'] })
    await settle()
    expect(s.opens).toHaveLength(2)
    expect(s.requests.at(-1)).toEqual({
      afterSeqno: 4n,
      interest: { version: 1n, addKeys: ['a', 'b'], addPatterns: [] },
    })
  })

  it('collapses an unread backlog into one unknown change', async () => {
    const s = fakeServer()
    const sub = s.feed.subscribe(0n, { keys: ['a'] })
    await settle()
    for (let i = 1n; i <= 300n; i++) s.change(i, ['a'])
    await settle()
    const it = sub[Symbol.asyncIterator]()
    const first = await it.next()
    expect(first.value).toEqual({
      case: 'change',
      change: { seqno: 257n, unknown: true },
    })
    sub.close()
  })

  it('spells a pattern as the World stores it', () => {
    expect(changePattern({ subject: '<a>', predicate: 'rel' })).toEqual({
      subject: '<a>',
      predicate: '"rel"',
      obj: '',
      label: '',
    })
    expect(
      changePattern({
        subject: '_:b',
        predicate: '"say \\"hi\\""',
        obj: '"v"^^<t>',
        label: '"x"@en',
      }),
    ).toEqual({
      subject: '_:b',
      predicate: '"say \\\\\\"hi\\\\\\""',
      obj: '"v"^^<t>',
      label: '"x"@en',
    })
  })

  it("routes a change only past each subscriber's own seqno", async () => {
    const s = fakeServer()
    const a = reader(s.feed.subscribe(5n, { keys: ['k'] }))
    const b = reader(s.feed.subscribe(8n, { keys: ['k'] }))
    await settle()
    s.change(7n, ['k'])
    s.change(9n, ['k'])
    await settle()
    expect(a.events.map((e) => e.case === 'change' && e.change.seqno)).toEqual([
      7n,
      9n,
    ])
    expect(b.events).toEqual([
      { case: 'followed', afterSeqno: 5n },
      { case: 'change', change: { seqno: 9n, keys: ['k'], unknown: false } },
    ])
  })

  it('follows names the stream holds without a delta', async () => {
    const s = fakeServer()
    reader(s.feed.subscribe(5n, { keys: ['a'] }))
    await settle()
    s.change(6n, ['a'])
    await settle()

    // The stream follows a already: the followed event is at the seqno the
    // stream passed, and no delta is sent.
    const b = reader(s.feed.subscribe(5n, { keys: ['a'] }))
    await settle()
    expect(s.requests).toHaveLength(1)
    expect(b.events).toEqual([{ case: 'followed', afterSeqno: 6n }])

    // While a delta is unacknowledged, a name it holds is followed at its
    // ack.
    const c = reader(s.feed.subscribe(5n, { keys: ['c'] }))
    await settle()
    const d = reader(s.feed.subscribe(5n, { keys: ['c'] }))
    await settle()
    expect(s.requests).toHaveLength(2)
    expect(d.events).toEqual([])
    s.ack(2n, 8n)
    await settle()
    expect(c.events).toEqual([{ case: 'followed', afterSeqno: 8n }])
    expect(d.events).toEqual([{ case: 'followed', afterSeqno: 8n }])
  })

  it('reaches a dropped followed event with the collapsed change', async () => {
    const s = fakeServer()
    const sub = s.feed.subscribe(0n, { keys: ['a'] })
    await settle()
    sub.update({ keys: ['b'] })
    await settle()
    for (let i = 1n; i <= 256n; i++) s.change(i, ['a'])
    s.ack(2n, 260n)
    await settle()
    const it = sub[Symbol.asyncIterator]()
    expect((await it.next()).value).toEqual({
      case: 'change',
      change: { seqno: 260n, unknown: true },
    })
    sub.close()
  })

  it('stops listening to the abort signal on close', () => {
    const s = fakeServer()
    const abort = new AbortController()
    const remove = vi.spyOn(abort.signal, 'removeEventListener')
    const sub = s.feed.subscribe(0n, { keys: ['a'] }, abort.signal)
    sub.close()
    expect(remove).toHaveBeenCalledWith('abort', expect.any(Function))
  })
})
