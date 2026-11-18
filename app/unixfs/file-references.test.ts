import { describe, expect, it } from 'vitest'

import type { KeyedSetReader } from '@s4wave/sdk/world/entity-set.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { buildTypeObjectKey } from '@s4wave/sdk/world/types/types.js'

import {
  fileReferencesSource,
  fileReferencesView,
  listReferrers,
} from './file-references.js'

const file = 'design.md'

// fakeWorld serves incoming edges into file sorted by subject, as cayley_kv
// does, and the types of objects. limits records each listing's limit.
function fakeWorld(
  edges: [subject: string, predicate: string][],
  types: Record<string, string>,
) {
  const limits: number[] = []
  const sorted = [...edges].sort((a, b) =>
    (a[0] + a[1]).localeCompare(b[0] + b[1]),
  )
  const tx = {
    listGraphEdgeBuckets: (keys: string[], limit: number, opts?: unknown) => {
      const options = opts as { predicate?: string }
      if (options.predicate) {
        return Promise.resolve({
          buckets: keys.map((key) => ({
            originObjectKey: key,
            outgoing: types[key]
              ? [{ obj: keyToIRI(buildTypeObjectKey(types[key])) }]
              : [],
          })),
        })
      }
      limits.push(limit)
      return Promise.resolve({
        buckets: [
          {
            originObjectKey: keys[0],
            incoming: sorted.slice(0, limit).map(([subject, predicate]) => ({
              subject: keyToIRI(subject),
              predicate: keyToIRI(predicate),
              obj: keyToIRI(file),
            })),
            incomingTruncated: sorted.length > limit,
          },
        ],
      })
    },
  } as unknown as KeyedSetReader
  return { tx, limits }
}

describe('listReferrers', () => {
  it('reads past an object that points at the file by several predicates', async () => {
    const { tx, limits } = fakeWorld(
      [
        ['a', 'ref/input'],
        ['a', 'ref/output'],
        ['a', 'ref/source'],
        [file, 'ref/self'],
        ['b', 'ref/input'],
        ['c', 'ref/input'],
      ],
      {},
    )
    const signal = new AbortController().signal
    expect(await listReferrers(tx, file, 2, signal)).toEqual(['a', 'b', 'c'])
    expect(limits).toEqual([3, 6])
    expect(await listReferrers(tx, file, 5, signal)).toEqual(['a', 'b', 'c'])
  })
})

describe('fileReferencesView', () => {
  it('lists typed and untyped referrers and shows More past the page', async () => {
    const { tx } = fakeWorld(
      [
        ['obs-1', 'ref/output'],
        ['a-untyped', 'ref/output'],
        ['obs-2', 'ref/output'],
      ],
      { 'obs-1': 'glados/observation', 'obs-2': 'glados/observation' },
    )
    const signal = new AbortController().signal
    const source = fileReferencesSource(file, 2)
    const keys = await source.list(tx, signal)
    const rows = new Map()
    for await (const page of source.read(tx, keys, signal)) {
      for (const [key, value] of page.rows) rows.set(key, value)
    }
    const retry = () => {}
    const value = { seqno: 1n, keys, rows, complete: true }
    expect(fileReferencesView({ value, retry }, 1)).toEqual({
      load: 'ready',
      refs: [['a-untyped', { typeId: '' }]],
      hasMore: true,
    })
    expect(fileReferencesView({ value, retry }, 3)).toEqual({
      load: 'ready',
      refs: [
        ['a-untyped', { typeId: '' }],
        ['obs-1', { typeId: 'glados/observation' }],
        ['obs-2', { typeId: 'glados/observation' }],
      ],
      hasMore: false,
    })
    expect(
      fileReferencesView({ value: { ...value, keys: [] }, retry }, 2),
    ).toEqual({ load: 'ready', refs: [], hasMore: false })
    expect(fileReferencesView({ value: null, retry }, 2)).toEqual({
      load: 'loading',
    })
    expect(
      fileReferencesView({ value, error: new Error('gone'), retry }, 2),
    ).toMatchObject({ load: 'failed', retry })
  })
})

describe('fileReferencesSource', () => {
  it('re-lists on an edge into the file and re-reads a retyped referrer', () => {
    const source = fileReferencesSource(file, 2)
    const members = new Set(['obs-1'])
    const into = {
      subject: keyToIRI('obs-2'),
      predicate: keyToIRI('ref/output'),
      obj: keyToIRI(file),
    }
    expect(source.route({ quads: [into] }, members)).toEqual({
      list: true,
      keys: [],
    })
    const retype = {
      subject: keyToIRI('obs-1'),
      predicate: source.names(['obs-1']).patterns?.[0]?.predicate,
      obj: keyToIRI(buildTypeObjectKey('glados/observation')),
    }
    expect(source.route({ quads: [retype] }, members)).toEqual({
      list: false,
      keys: ['obs-1'],
    })
    const elsewhere = { ...into, obj: keyToIRI('other.md') }
    expect(source.route({ quads: [elsewhere] }, members).list).toBe(false)
  })
})
