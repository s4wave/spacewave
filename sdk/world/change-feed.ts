import type { Quad } from '../../db/block/quad/quad.pb.js'
import { canonicalGraphValue } from './graph-utils.js'
import type {
  ChangeInterestDelta,
  WatchChangesRequest,
  WatchChangesResponse,
  WorldChange,
} from './world.pb.js'

// ChangeNames names the changes a subscriber follows: changes to its object
// keys, and changes setting or deleting a graph quad that matches one of its
// patterns.
export interface ChangeNames {
  keys?: readonly string[]
  patterns?: readonly ChangePattern[]
}

declare const changePatternBrand: unique symbol

// ChangePattern is a graph pattern spelled as the World stores quads, so the
// feed matches it against changed quads exactly as the World does. Only
// changePattern builds one.
export type ChangePattern = Readonly<Quad> & {
  readonly [changePatternBrand]: true
}

// changePattern builds the pattern matching quads with the given fields. An
// empty field matches any value; the others are spelled as the World stores
// them, so a key is named as an IRI, keyToIRI(key), and a bare predicate
// matches the quoted string the World stores for it.
export function changePattern(fields: Quad): ChangePattern {
  return {
    subject: canonicalGraphValue(fields.subject ?? ''),
    predicate: canonicalGraphValue(fields.predicate ?? ''),
    obj: canonicalGraphValue(fields.obj ?? ''),
    label: canonicalGraphValue(fields.label ?? ''),
  } as ChangePattern
}

// ChangeFeedEvent is one event of a subscription, in stream order.
//
// change: a change after the subscription's seqno naming one of its names, or
// an unknown change, after which any read may have changed.
//
// followed: every name added before it is followed after afterSeqno. A change
// at or before afterSeqno to a name added since the last followed event may
// not have been sent, so a name read at a revision below afterSeqno is read
// again.
export type ChangeFeedEvent =
  | { case: 'change'; change: WorldChange }
  | { case: 'followed'; afterSeqno: bigint }

// ChangeFeedOpen opens a WatchChanges stream with requests.
export type ChangeFeedOpen = (
  requests: AsyncIterable<WatchChangesRequest>,
  signal: AbortSignal,
) => AsyncIterable<WatchChangesResponse>

// maxQueued bounds the events a subscription holds unread. Past it they
// collapse into one unknown change, which a reader answers with one full read.
const maxQueued = 256

// patternKey identifies a pattern by its four fields.
function patternKey(q: ChangePattern): string {
  return JSON.stringify([
    q.subject ?? '',
    q.predicate ?? '',
    q.obj ?? '',
    q.label ?? '',
  ])
}

// patternMatches reports whether quad matches pattern.
function patternMatches(pattern: ChangePattern, quad: Quad): boolean {
  return (
    (!pattern.subject || pattern.subject === quad.subject) &&
    (!pattern.predicate || pattern.predicate === quad.predicate) &&
    (!pattern.obj || pattern.obj === quad.obj) &&
    (!pattern.label || pattern.label === quad.label)
  )
}

// RequestQueue is the request side of one stream: pushed requests in order
// until it ends.
class RequestQueue implements AsyncIterable<WatchChangesRequest> {
  private readonly queue: WatchChangesRequest[] = []
  private wake?: () => void
  private ended = false

  push(req: WatchChangesRequest) {
    this.queue.push(req)
    this.wake?.()
  }

  end() {
    this.ended = true
    this.wake?.()
  }

  async *[Symbol.asyncIterator](): AsyncIterator<WatchChangesRequest> {
    for (;;) {
      const req = this.queue.shift()
      if (req) {
        yield req
        continue
      }
      if (this.ended) return
      await new Promise<void>((resolve) => {
        this.wake = resolve
      })
      this.wake = undefined
    }
  }
}

// ChangeFeedSubscription is one subscriber's view of a shared change stream.
// Iterate it for events; update changes its names; close ends it.
export class ChangeFeedSubscription implements AsyncIterable<ChangeFeedEvent> {
  // after is the last seqno the subscription has passed.
  after: bigint
  readonly keys = new Set<string>()
  readonly patterns = new Map<string, ChangePattern>()
  // pending is the version of the delta carrying its latest added names, or
  // 0n when every added name is followed.
  pending = 0n
  private queue: ChangeFeedEvent[] = []
  private error?: unknown
  private done = false
  private wake?: () => void
  // detach removes the abort listener subscribe added, if any.
  detach?: () => void

  constructor(
    private readonly feed: ChangeFeed,
    afterSeqno: bigint,
  ) {
    this.after = afterSeqno
  }

  // update adds and removes names. Each added name is followed once the next
  // followed event arrives.
  update(add: ChangeNames, remove: ChangeNames = {}) {
    if (!this.done) this.feed.update(this, add, remove)
  }

  // close ends the subscription and stops following its names.
  close() {
    if (this.done) return
    this.done = true
    this.detach?.()
    this.feed.remove(this)
    this.wake?.()
  }

  // follows reports whether change names one of the subscription's names.
  follows(change: WorldChange): boolean {
    if (change.unknown) return true
    if (change.keys?.some((key) => this.keys.has(key))) return true
    if (this.patterns.size === 0) return false
    return !!change.quads?.some((quad) => {
      for (const pattern of this.patterns.values()) {
        if (patternMatches(pattern, quad)) return true
      }
      return false
    })
  }

  // push queues an event, collapsing the queue into one unknown change when
  // the reader falls behind. The unknown change reaches every seqno the
  // collapsed events and the subscription reached, a dropped followed
  // event's included.
  push(event: ChangeFeedEvent) {
    if (this.queue.length >= maxQueued) {
      let seqno = this.after
      for (const queued of [...this.queue, event]) {
        const reached =
          queued.case === 'change'
            ? (queued.change.seqno ?? 0n)
            : queued.afterSeqno
        if (reached > seqno) seqno = reached
      }
      this.queue = [{ case: 'change', change: { seqno, unknown: true } }]
    } else {
      this.queue.push(event)
    }
    this.wake?.()
  }

  // drain takes the events queued unread, without waiting. A reader that
  // woke on one event takes the rest of the burst with it.
  drain(): ChangeFeedEvent[] {
    const queued = this.queue
    this.queue = []
    return queued
  }

  // fail ends the subscription with err once its queued events are read.
  fail(err: unknown) {
    this.error = err
    this.done = true
    this.wake?.()
  }

  async *[Symbol.asyncIterator](): AsyncIterator<ChangeFeedEvent> {
    try {
      for (;;) {
        const event = this.queue.shift()
        if (event) {
          yield event
          continue
        }
        if (this.error !== undefined) throw this.error
        if (this.done) return
        await new Promise<void>((resolve) => {
          this.wake = resolve
        })
        this.wake = undefined
      }
    } finally {
      this.close()
    }
  }
}

// ChangeFeed shares one WatchChanges stream among its subscribers. The stream
// carries the union of their names, changed by one delta per burst of
// updates; each change goes to the subscribers it names. The stream opens
// with the first subscriber and closes with the last. When it fails, every
// subscriber fails with it; a subscriber reopens by subscribing again with
// the seqno it reached and its whole set of names, and reads again every name
// it added without a followed event.
export class ChangeFeed {
  // keys and patterns count the subscribers following each name.
  private readonly keys = new Map<string, number>()
  private readonly patterns = new Map<
    string,
    { quad: ChangePattern; count: number }
  >()
  // sentKeys and sentPatterns are the names the stream holds.
  private sentKeys = new Set<string>()
  private sentPatterns = new Map<string, ChangePattern>()
  private readonly subs = new Set<ChangeFeedSubscription>()
  private stream?: { requests: RequestQueue; abort: AbortController }
  // version is the version of the last delta sent, and acked the version of
  // the last delta the stream acknowledged; the first interest needs none.
  private version = 0n
  private acked = 0n
  // position is the last seqno the stream passed, by a change or an ack.
  private position = 0n
  private flushQueued = false

  constructor(private readonly open: ChangeFeedOpen) {}

  // subscribe follows names after afterSeqno until the subscription closes
  // or signal aborts.
  subscribe(
    afterSeqno: bigint,
    names: ChangeNames,
    signal?: AbortSignal,
  ): ChangeFeedSubscription {
    const sub = new ChangeFeedSubscription(this, afterSeqno)
    if (signal?.aborted) {
      sub.close()
      return sub
    }
    if (signal) {
      const onAbort = () => sub.close()
      signal.addEventListener('abort', onAbort, { once: true })
      sub.detach = () => signal.removeEventListener('abort', onAbort)
    }
    this.subs.add(sub)
    this.add(sub, names)
    if (this.stream) {
      sub.pending = this.version + 1n
      this.queueFlush()
    } else {
      this.start(afterSeqno)
    }
    return sub
  }

  // update changes sub's names.
  update(sub: ChangeFeedSubscription, add: ChangeNames, remove: ChangeNames) {
    this.drop(sub, remove)
    if (this.add(sub, add)) sub.pending = this.version + 1n
    this.queueFlush()
  }

  // remove stops following sub's names, closing the stream after the last.
  remove(sub: ChangeFeedSubscription) {
    if (!this.subs.delete(sub)) return
    this.drop(sub, {
      keys: [...sub.keys],
      patterns: [...sub.patterns.values()],
    })
    if (this.subs.size === 0) {
      this.reset()
      return
    }
    this.queueFlush()
  }

  // add adds names to sub and the union, reporting whether any was new to sub.
  private add(sub: ChangeFeedSubscription, names: ChangeNames): boolean {
    let added = false
    for (const key of names.keys ?? []) {
      if (sub.keys.has(key)) continue
      sub.keys.add(key)
      this.keys.set(key, (this.keys.get(key) ?? 0) + 1)
      added = true
    }
    for (const quad of names.patterns ?? []) {
      const id = patternKey(quad)
      if (sub.patterns.has(id)) continue
      sub.patterns.set(id, quad)
      const entry = this.patterns.get(id)
      if (entry) entry.count++
      else this.patterns.set(id, { quad, count: 1 })
      added = true
    }
    return added
  }

  // drop removes names from sub and the union.
  private drop(sub: ChangeFeedSubscription, names: ChangeNames) {
    for (const key of names.keys ?? []) {
      if (!sub.keys.delete(key)) continue
      const count = (this.keys.get(key) ?? 1) - 1
      if (count === 0) this.keys.delete(key)
      else this.keys.set(key, count)
    }
    for (const quad of names.patterns ?? []) {
      const id = patternKey(quad)
      if (!sub.patterns.delete(id)) continue
      const entry = this.patterns.get(id)
      if (entry && --entry.count === 0) this.patterns.delete(id)
    }
  }

  // start opens the stream after afterSeqno with the whole union.
  private start(afterSeqno: bigint) {
    const requests = new RequestQueue()
    const abort = new AbortController()
    this.stream = { requests, abort }
    this.version = 1n
    this.acked = 1n
    this.position = afterSeqno
    this.sentKeys = new Set(this.keys.keys())
    this.sentPatterns = new Map(
      [...this.patterns].map(([id, entry]) => [id, entry.quad]),
    )
    requests.push({
      afterSeqno,
      interest: {
        version: 1n,
        addKeys: [...this.sentKeys],
        addPatterns: [...this.sentPatterns.values()],
      },
    })
    void this.read(this.stream)
  }

  // read hands each response to the subscribers until the stream ends.
  private async read(stream: {
    requests: RequestQueue
    abort: AbortController
  }) {
    let err: unknown = new Error('change stream ended')
    try {
      for await (const resp of this.open(
        stream.requests,
        stream.abort.signal,
      )) {
        if (this.stream !== stream) return
        this.receive(resp)
      }
    } catch (e) {
      err = e
    }
    if (this.stream !== stream) return
    for (const sub of this.subs) sub.fail(err)
    this.subs.clear()
    this.reset()
  }

  // receive routes one response.
  private receive(resp: WatchChangesResponse) {
    const body = resp.body
    if (body?.case === 'change') {
      const change = body.value
      const seqno = change.seqno ?? 0n
      if (seqno > this.position) this.position = seqno
      for (const sub of this.subs) {
        if (seqno <= sub.after) continue
        sub.after = seqno
        if (sub.follows(change)) sub.push({ case: 'change', change })
      }
      return
    }
    if (body?.case === 'interestAck') {
      const version = body.value.version ?? 0n
      const afterSeqno = body.value.afterSeqno ?? 0n
      this.acked = version
      if (afterSeqno > this.position) this.position = afterSeqno
      for (const sub of this.subs) {
        if (sub.pending === 0n || sub.pending > version) continue
        sub.pending = 0n
        sub.push({ case: 'followed', afterSeqno })
      }
    }
  }

  // queueFlush sends one delta for every update made in this task.
  private queueFlush() {
    if (this.flushQueued || !this.stream) return
    this.flushQueued = true
    queueMicrotask(() => {
      this.flushQueued = false
      this.flush()
    })
  }

  // flush sends the delta from the names the stream holds to the union. A
  // subscriber that added only names the stream already holds needs no
  // delta: once the stream's last delta is acknowledged they are followed
  // after every seqno the stream passed, so it gets its followed event here;
  // otherwise it waits for that delta's ack.
  private flush() {
    const stream = this.stream
    if (!stream) return
    const delta: ChangeInterestDelta = {
      addKeys: [...this.keys.keys()].filter((key) => !this.sentKeys.has(key)),
      removeKeys: [...this.sentKeys].filter((key) => !this.keys.has(key)),
      addPatterns: [...this.patterns]
        .filter(([id]) => !this.sentPatterns.has(id))
        .map(([, entry]) => entry.quad),
      removePatterns: [...this.sentPatterns]
        .filter(([id]) => !this.patterns.has(id))
        .map(([, quad]) => quad),
    }
    const changed =
      delta.addKeys!.length +
      delta.removeKeys!.length +
      delta.addPatterns!.length +
      delta.removePatterns!.length
    if (!changed) {
      for (const sub of this.subs) {
        if (sub.pending <= this.version) continue
        if (this.acked < this.version) {
          sub.pending = this.version
          continue
        }
        sub.pending = 0n
        sub.push({ case: 'followed', afterSeqno: this.position })
      }
      return
    }
    this.version++
    delta.version = this.version
    this.sentKeys = new Set(this.keys.keys())
    this.sentPatterns = new Map(
      [...this.patterns].map(([id, entry]) => [id, entry.quad]),
    )
    stream.requests.push({ interest: delta })
  }

  // reset closes the stream and forgets the names it held.
  private reset() {
    const stream = this.stream
    this.stream = undefined
    this.version = 0n
    this.acked = 0n
    this.position = 0n
    this.sentKeys = new Set()
    this.sentPatterns = new Map()
    this.keys.clear()
    this.patterns.clear()
    stream?.requests.end()
    stream?.abort.abort()
  }
}
