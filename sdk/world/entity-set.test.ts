import { describe, expect, it } from 'vitest'

import { ChangeFeed } from './change-feed.js'
import type { Engine } from './engine.js'
import {
  entitySetSource,
  messageDecoder,
  watchKeyedSet,
  type EntitySetSpec,
  type KeyedSet,
} from './entity-set.js'
import { keyToIRI } from './graph-utils.js'
import { WatchChangesResponse } from './world.pb.js'

const spec: EntitySetSpec = {
  case: 'link',
  origin: 'job',
  predicate: '<job-task>',
  direction: 'out',
}

const text = {
  name: 'text',
  decode: (body: Uint8Array) => new TextDecoder().decode(body),
}

// fakeWorld is a World with one link origin: it counts every call a wake
// makes and sends the change stream the test pushes.
function fakeWorld() {
  let seqno = 1n
  const members: string[] = []
  const bodies = new Map<string, string>()
  const calls: string[] = []
  let push: ((resp: WatchChangesResponse) => void) | undefined
  const versions: bigint[] = []

  const feed = new ChangeFeed((reqs, signal) => {
    void (async () => {
      for await (const req of reqs) {
        if (req.interest?.version) versions.push(req.interest.version)
      }
    })()
    const queue: WatchChangesResponse[] = []
    let wake: (() => void) | undefined
    push = (resp) => {
      queue.push(resp)
      wake?.()
    }
    return (async function* () {
      for (;;) {
        const next = queue.shift()
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

  const tx = {
    getSeqno: () => {
      calls.push('getSeqno')
      return Promise.resolve({ seqno })
    },
    listGraphEdgeBuckets: () => {
      calls.push('list')
      return Promise.resolve({
        buckets: [
          {
            originObjectKey: 'job',
            outgoing: members.map((key) => ({ obj: keyToIRI(key) })),
          },
        ],
      })
    },
    getObjectBodyPages: async function* (keys: string[]) {
      calls.push('bodies')
      await Promise.resolve()
      yield {
        worldSeqno: seqno,
        bodies: keys.map((objectKey) => {
          const body = bodies.get(objectKey)
          return {
            objectKey,
            exists: body !== undefined,
            body: new TextEncoder().encode(body ?? ''),
          }
        }),
      }
    },
    discard: () => {
      calls.push('discard')
      return Promise.resolve()
    },
    release: () => {},
  }

  const engine = {
    newTransaction: () => {
      calls.push('tx')
      return Promise.resolve(tx)
    },
    watchChanges: feed.subscribe.bind(feed),
  } as unknown as Engine

  const send = (body: WatchChangesResponse['body']) => push?.({ body })
  return {
    engine,
    calls,
    versions,
    get seqno() {
      return seqno
    },
    // put writes a member body in one commit and sends its change.
    put(key: string, body: string) {
      bodies.set(key, body)
      seqno++
      send({ case: 'change', value: { seqno, keys: [key] } })
    },
    // link adds or removes a member in one commit and sends its quad.
    link(key: string, add: boolean, body?: string) {
      if (add) members.push(key)
      else members.splice(members.indexOf(key), 1)
      if (body !== undefined) bodies.set(key, body)
      seqno++
      send({
        case: 'change',
        value: {
          seqno,
          quads: [
            {
              subject: keyToIRI('job'),
              predicate: '<job-task>',
              obj: keyToIRI(key),
            },
          ],
        },
      })
    },
    // seed sets members and bodies without a change.
    seed(rows: Record<string, string>) {
      for (const [key, body] of Object.entries(rows)) {
        members.push(key)
        bodies.set(key, body)
      }
    },
    // join links a member in a commit the stream does not send, as for a
    // pattern the stream does not follow yet.
    join(key: string, body: string) {
      members.push(key)
      bodies.set(key, body)
      seqno++
    },
    // write changes a body in a commit the stream does not send, as for a
    // name the stream does not follow yet.
    write(key: string, body: string) {
      bodies.set(key, body)
      seqno++
    },
    ack: (version: bigint, afterSeqno: bigint) =>
      send({ case: 'interestAck', value: { version, afterSeqno } }),
    unknown: () =>
      send({ case: 'change', value: { seqno: ++seqno, unknown: true } }),
  }
}

// follow starts watchKeyedSet over world and returns a reader that takes
// the next set it yields.
function follow(world: ReturnType<typeof fakeWorld>) {
  const abort = new AbortController()
  const it = watchKeyedSet(
    world.engine,
    entitySetSource(spec, text),
    abort.signal,
  )
  return {
    next: async (): Promise<KeyedSet<string>> => {
      const r = await it.next()
      if (r.done) throw new Error('set ended')
      return r.value
    },
    // settled takes sets until one is complete.
    settled: async (): Promise<KeyedSet<string>> => {
      for (;;) {
        const r = await it.next()
        if (r.done) throw new Error('set ended')
        if (r.value.complete) return r.value
      }
    },
    close: () => abort.abort(),
  }
}

const rowsOf = (set: KeyedSet<string>) =>
  Object.fromEntries(set.keys.map((key) => [key, set.rows.get(key)]))

describe('watchKeyedSet', () => {
  it('reads one body per entity change in three calls', async () => {
    const w = fakeWorld()
    w.seed({ a: 'a1', b: 'b1' })
    const set = follow(w)
    const first = await set.settled()
    expect(rowsOf(first)).toEqual({ a: 'a1', b: 'b1' })
    expect(w.calls).toEqual(['tx', 'list', 'bodies', 'discard'])
    w.ack(1n, w.seqno)

    w.calls.length = 0
    w.put('a', 'a2')
    const next = await set.settled()
    expect(rowsOf(next)).toEqual({ a: 'a2', b: 'b1' })
    expect(w.calls).toEqual(['tx', 'bodies', 'discard'])
    // The unchanged row keeps its value, so its memoized row skips a draw,
    // and the yielded set was not changed under its reader.
    expect(next.rows.get('b')).toBe(first.rows.get('b'))
    expect(first.rows.get('a')).toBe('a1')
    set.close()
  })

  it('adds and removes members without blanking the set', async () => {
    const w = fakeWorld()
    w.seed({ a: 'a1' })
    const set = follow(w)
    await set.settled()
    w.ack(1n, w.seqno)

    w.calls.length = 0
    w.link('b', true, 'b1')
    const added = await set.settled()
    expect(rowsOf(added)).toEqual({ a: 'a1', b: 'b1' })
    expect(w.calls).toEqual(['tx', 'list', 'bodies', 'discard'])

    w.link('a', false)
    const removed = await set.settled()
    expect(rowsOf(removed)).toEqual({ b: 'b1' })
    expect(removed.rows.has('a')).toBe(false)
    set.close()
  })

  it('reads a new member again when it is followed past its read', async () => {
    const w = fakeWorld()
    w.seed({ a: 'a1' })
    const set = follow(w)
    await set.settled()
    w.ack(1n, w.seqno)

    // b is linked and read at R; its own names are followed only from the
    // ack's L, so a change to b between R and L reaches the set only
    // through the re-read the ack causes.
    w.link('b', true, 'b1')
    const added = await set.settled()
    expect(added.rows.get('b')).toBe('b1')
    w.write('b', 'b2')
    w.ack(2n, w.seqno)
    const caught = await set.settled()
    expect(caught.rows.get('b')).toBe('b2')
    set.close()
  })

  it('lists again when it joins a stream past its listing', async () => {
    const w = fakeWorld()
    w.seed({ a: 'a1' })
    // Another subscriber opened the Engine's stream, so the set joins it
    // and its link pattern is followed only from the ack of its names.
    const other = new AbortController()
    w.engine.watchChanges(w.seqno, { keys: ['other'] }, other.signal)
    w.ack(1n, w.seqno)
    const set = follow(w)
    expect(rowsOf(await set.settled())).toEqual({ a: 'a1' })

    // b is linked after the listing, in a commit the stream passes before
    // it follows the pattern; the ack past the listing lists again.
    w.join('b', 'b1')
    w.ack(2n, w.seqno)
    expect(rowsOf(await set.settled())).toEqual({ a: 'a1', b: 'b1' })
    set.close()
    other.abort()
  })

  it('reads the whole set again after an unknown change', async () => {
    const w = fakeWorld()
    w.seed({ a: 'a1' })
    const set = follow(w)
    await set.settled()
    w.ack(1n, w.seqno)

    w.calls.length = 0
    w.seed({ c: 'c1' })
    w.unknown()
    const reread = await set.settled()
    expect(rowsOf(reread)).toEqual({ a: 'a1', c: 'c1' })
    expect(reread.seqno).toBe(w.seqno)
    expect(w.calls).toEqual(['tx', 'list', 'bodies', 'discard'])
    set.close()
  })
})

describe('messageDecoder', () => {
  it('fails only the row of a body it can not decode', () => {
    const decoder = messageDecoder(WatchChangesResponse)
    expect(decoder.decode(new Uint8Array([0xff])).error).toBeTruthy()
    const body = WatchChangesResponse.toBinary({
      body: { case: 'interestAck', value: { version: 3n } },
    })
    expect(decoder.decode(body).value?.body?.case).toBe('interestAck')
  })
})
