import { describe, expect, it, vi } from 'vitest'

import {
  Job,
  State as JobState,
} from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { ChangeFeed } from '@s4wave/sdk/world/change-feed.js'
import type { Engine } from '@s4wave/sdk/world/engine.js'
import {
  readKeyedSet,
  watchKeyedSet,
  type KeyedSetReader,
} from '@s4wave/sdk/world/entity-set.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { buildTypeObjectKey, TypePred } from '@s4wave/sdk/world/types/types.js'
import type { WatchChangesResponse } from '@s4wave/sdk/world/world.pb.js'
import {
  buildForgeDashboardActivityEntries,
  forgeDashboardSource,
} from './useForgeDashboardActivity.js'

describe('forgeDashboardSource', () => {
  it('routes a type quad to its subject only when that is an IRI', () => {
    const source = forgeDashboardSource('forge/dashboard')
    const members = new Set(['notes/b'])
    const typeQuad = (subject: string) => ({
      seqno: 2n,
      quads: [
        {
          subject,
          predicate: TypePred,
          obj: keyToIRI(buildTypeObjectKey('forge/job')),
        },
      ],
    })
    expect(source.route(typeQuad(keyToIRI('notes/b')), members).keys).toEqual([
      'notes/b',
    ])
    expect(source.route(typeQuad('_:b0'), members)).toEqual({
      list: false,
      keys: [],
    })
  })

  it('reads the bodies of only the linked objects that have activity', async () => {
    const signal = new AbortController().signal
    const jobData = Job.toBinary({
      jobState: JobState.JobState_RUNNING,
      timestamp: new Date('2026-05-11T12:01:00Z'),
    })
    const members = {
      'gizmo/dogfood/worklist/job/task/agent-instance/dormant':
        'gizmo/agent-instance',
      'gizmo/dogfood/worklist/evidence/old-history': 'gizmo/evidence',
      'forge/job/active': 'forge/job',
    }
    const reader = {
      getSeqno: vi.fn(() => Promise.resolve({ seqno: 7n })),
      listObjectsWithType: vi.fn(),
      listGraphEdgeBuckets: vi.fn((keys: string[], _limit: number) => {
        if (keys[0] === 'forge/dashboard') {
          return Promise.resolve({
            buckets: [
              {
                originObjectKey: 'forge/dashboard',
                outgoing: Object.keys(members).map((key) => ({
                  obj: keyToIRI(key),
                })),
              },
            ],
          })
        }
        return Promise.resolve({
          buckets: keys.map((key) => ({
            originObjectKey: key,
            outgoing: [
              {
                obj: keyToIRI(
                  buildTypeObjectKey(members[key as keyof typeof members]),
                ),
              },
            ],
          })),
        })
      }),
      getObjectBodyPages: vi.fn(async function* (keys: string[]) {
        await Promise.resolve()
        yield {
          worldSeqno: 7n,
          bodies: keys.map((objectKey) => ({
            objectKey,
            exists: true,
            body: jobData,
          })),
        }
      }),
    }

    const set = await readKeyedSet(
      reader as unknown as KeyedSetReader,
      forgeDashboardSource('forge/dashboard'),
      signal,
    )

    expect(reader.getObjectBodyPages).toHaveBeenCalledTimes(1)
    expect(reader.getObjectBodyPages).toHaveBeenCalledWith(
      ['forge/job/active'],
      signal,
    )
    expect(set.keys).toHaveLength(3)
    const entries = buildForgeDashboardActivityEntries(
      { name: 'Worklist', createdAt: new Date('2026-05-11T12:00:00Z') },
      set.keys.flatMap((key) => set.rows.get(key)?.source ?? []),
    )
    expect(entries.map((entry) => entry.id)).toEqual([
      'forge/job/active:snapshot',
      'dashboard-created',
    ])
    expect(
      set.rows.get('gizmo/dogfood/worklist/evidence/old-history')?.typeId,
    ).toBe('gizmo/evidence')
  })

  it('reads a member again when a commit only changes its type', async () => {
    const abort = new AbortController()
    let seqno = 1n
    const types: Record<string, string> = {
      'forge/job/a': 'forge/job',
      'notes/b': 'gizmo/evidence',
    }
    const jobData = Job.toBinary({
      jobState: JobState.JobState_RUNNING,
      timestamp: new Date('2026-05-11T12:01:00Z'),
    })
    let push: ((resp: WatchChangesResponse) => void) | undefined
    const feed = new ChangeFeed((reqs, signal) => {
      void (async () => {
        for await (const _ of reqs) {
          // The fake server follows every name.
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
      getSeqno: () => Promise.resolve({ seqno }),
      listGraphEdgeBuckets: (keys: string[]) =>
        Promise.resolve({
          buckets:
            keys[0] === 'forge/dashboard'
              ? [
                  {
                    originObjectKey: 'forge/dashboard',
                    outgoing: Object.keys(types).map((key) => ({
                      obj: keyToIRI(key),
                    })),
                  },
                ]
              : keys.map((key) => ({
                  originObjectKey: key,
                  outgoing: [{ obj: keyToIRI(buildTypeObjectKey(types[key])) }],
                })),
        }),
      getObjectBodyPages: async function* (keys: string[]) {
        await Promise.resolve()
        yield {
          worldSeqno: seqno,
          bodies: keys.map((objectKey) => ({
            objectKey,
            exists: true,
            body: jobData,
          })),
        }
      },
      discard: () => Promise.resolve(),
      release: () => {},
    }
    const engine = {
      newTransaction: () => Promise.resolve(tx),
      watchChanges: feed.subscribe.bind(feed),
    } as unknown as Engine
    const it = watchKeyedSet(
      engine,
      forgeDashboardSource('forge/dashboard'),
      abort.signal,
    )
    const settled = async () => {
      for (;;) {
        const r = await it.next()
        if (r.done) throw new Error('set ended')
        if (r.value.complete) return r.value
      }
    }

    const first = await settled()
    expect(first.rows.get('notes/b')).toEqual({
      typeId: 'gizmo/evidence',
      source: null,
    })
    push?.({
      body: { case: 'interestAck', value: { version: 1n, afterSeqno: seqno } },
    })

    // The retype commits only the type quad: no key names notes/b.
    types['notes/b'] = 'forge/job'
    seqno++
    push?.({
      body: {
        case: 'change',
        value: {
          seqno,
          quads: [
            {
              subject: keyToIRI('notes/b'),
              predicate: TypePred,
              obj: keyToIRI(buildTypeObjectKey('forge/job')),
            },
          ],
        },
      },
    })
    const retyped = await settled()
    expect(retyped.seqno).toBe(seqno)
    expect(retyped.rows.get('notes/b')?.typeId).toBe('forge/job')
    expect(retyped.rows.get('notes/b')?.source).not.toBeNull()
    expect(retyped.rows.get('forge/job/a')).toBe(first.rows.get('forge/job/a'))
    abort.abort()
  })
})
