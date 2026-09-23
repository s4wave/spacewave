import type { StandardSchemaV1 } from '@standard-schema/spec'
import { describe, expect, test } from 'vitest'

import { evaluateQuery, QueryFilter, type QueryContext } from './query.js'
import { defineSchema } from './schema.js'

type Color = {
  name: string
  likes: number
  visible: boolean
  extra: { label: string } | null
}
const color: StandardSchemaV1<Color> = {
  '~standard': {
    version: 1,
    vendor: 'fixture',
    validate: (value) => ({ value: value as Color }),
  },
}
const schema = defineSchema({
  id: 'colors',
  version: 1,
  collections: { colors: color },
  mutations: {},
})
const red: Color = {
  name: 'Red',
  likes: 2,
  visible: true,
  extra: { label: 'warm' },
}
const blue: Color = { name: 'Blue', likes: 1, visible: false, extra: null }

/** source returns independent JSON values from an immutable fixture snapshot. */
function source(entries: Record<string, Color>): QueryContext<typeof schema> {
  return {
    collection: () => ({
      get: async (key) => entries[key] && structuredClone(entries[key]),
      scan: async ({ prefix = '' } = {}) =>
        Object.entries(entries)
          .filter(([key]) => key.startsWith(prefix))
          .map(([key, value]) => ({ key, value: structuredClone(value) })),
    }),
  }
}

describe('function-derived query dependencies', () => {
  test('tracks excluded predicates, sort keys, scan membership, and materialized output', async () => {
    const query = await evaluateQuery(source({ red, blue }), async (tx) =>
      (await tx.collection('colors').scan())
        .filter(({ value }) => value.visible)
        .toSorted((a, b) => b.value.likes - a.value.likes)
        .map(({ value }) => value),
    )
    expect(query.value).toEqual([red])
    const filter = new QueryFilter(query.dependencies)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'blue',
        before: blue,
        after: { ...blue, visible: true },
      }),
    ).toBe(true)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'blue',
        before: blue,
        after: { ...blue, name: 'Azure' },
      }),
    ).toBe(false)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: { ...red, extra: null },
      }),
    ).toBe(true)
    expect(
      filter.affected({ collection: 'colors', key: 'green', after: red }),
    ).toBe(true)
    expect(
      filter.affected({ collection: 'colors', key: 'blue', before: blue }),
    ).toBe(true)
    expect(
      filter.affected({
        collection: 'other',
        key: 'red',
        before: red,
        after: blue,
      }),
    ).toBe(false)
  })

  test('tracks absent lookups and replaces dependencies after a branch changes', async () => {
    const evaluate = async (tx: QueryContext<typeof schema>) => {
      const selected = await tx.collection('colors').get('selected')
      return selected?.visible
        ? ((await tx.collection('colors').get('red'))?.name ?? '')
        : ''
    }
    const absent = new QueryFilter(
      (await evaluateQuery(source({ red }), evaluate)).dependencies,
    )
    expect(
      absent.affected({ collection: 'colors', key: 'selected', after: red }),
    ).toBe(true)
    expect(
      absent.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: blue,
      }),
    ).toBe(false)
    const present = new QueryFilter(
      (await evaluateQuery(source({ red, selected: red }), evaluate))
        .dependencies,
    )
    expect(
      present.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: blue,
      }),
    ).toBe(true)
    expect(
      present.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: { ...red, likes: 100 },
      }),
    ).toBe(false)
  })

  test('tracks nested object presence and key enumeration', async () => {
    const truth = await evaluateQuery(
      source({ red }),
      async (tx) => !!(await tx.collection('colors').get('red'))?.extra,
    )
    const filter = new QueryFilter(truth.dependencies)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: { ...red, extra: null },
      }),
    ).toBe(true)
    const keys = await evaluateQuery(source({ red }), async (tx) =>
      Object.keys((await tx.collection('colors').get('red'))!),
    )
    expect(
      new QueryFilter(keys.dependencies).affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: { ...red, another: true },
      }),
    ).toBe(true)
  })

  test('reads frozen nested values without watching uninspected siblings', async () => {
    const value = Object.freeze({
      ...red,
      extra: Object.freeze({ label: 'warm', hidden: 1 }),
    })
    const query = await evaluateQuery(
      {
        collection: () => ({ get: async () => value, scan: async () => [] }),
      } as QueryContext<typeof schema>,
      async (tx) => (await tx.collection('colors').get('red'))?.extra?.label,
    )
    expect(query.value).toBe('warm')
    const filter = new QueryFilter(query.dependencies)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: value,
        after: { ...value, extra: { label: 'warm', hidden: 2 } },
      }),
    ).toBe(false)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: value,
        after: { ...value, extra: null },
      }),
    ).toBe(true)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: value,
        after: { ...value, extra: { label: 'cool', hidden: 1 } },
      }),
    ).toBe(true)
  })

  test('tracks absent own properties inspected through descriptors', async () => {
    const query = await evaluateQuery(source({ red }), async (tx) =>
      Object.hasOwn((await tx.collection('colors').get('red'))!, 'optional'),
    )
    expect(query.value).toBe(false)
    const filter = new QueryFilter(query.dependencies)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: { ...red, optional: true },
      }),
    ).toBe(true)
    expect(
      filter.affected({
        collection: 'colors',
        key: 'red',
        before: red,
        after: { ...red, likes: red.likes + 1 },
      }),
    ).toBe(false)
  })

  test('limits membership invalidation to the scanned prefix', async () => {
    const query = await evaluateQuery(
      source({ red }),
      async (tx) =>
        (await tx.collection('colors').scan({ prefix: 'r' })).length,
    )
    const filter = new QueryFilter(query.dependencies)
    expect(
      filter.affected({ collection: 'colors', key: 'blue', after: blue }),
    ).toBe(false)
    expect(
      filter.affected({ collection: 'colors', key: 'rose', after: red }),
    ).toBe(true)
    expect(filter.unknown('colors')).toBe(true)
    expect(filter.unknown('other')).toBe(false)
  })

  test('rejects snapshot mutation and reads after evaluation ends', async () => {
    await expect(
      evaluateQuery(source({ red }), async (tx) => {
        const value = (await tx.collection('colors').get('red'))!
        value.likes++
        return value
      }),
    ).rejects.toMatchObject({ code: 'VALIDATION' })
    let late: (() => Promise<unknown>) | undefined
    await evaluateQuery(source({ red }), (tx) => {
      const colors = tx.collection('colors')
      late = () => colors.get('red')
      return null
    })
    await expect(late!()).rejects.toMatchObject({ code: 'CLOSED' })
  })
})
