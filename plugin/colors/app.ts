import { z } from 'zod'

import { defineApp } from '../../sdk/sync/app.js'
import { defineQuery } from '../../sdk/sync/query.js'
import { SyncError } from '../../sdk/sync/errors.js'

/** colors defines one World-backed voting app with named, replayable operations. */
export const colors = defineApp(
  {
    id: 'colors/votes',
    version: 1,
    collections: {
      colors: z.object({
        name: z.string().min(1),
        hex: z.string().regex(/^#[0-9a-fA-F]{6}$/),
        likes: z.number().int().nonnegative(),
      }),
    },
    mutations: {
      initialize: { input: z.null(), output: z.null() },
      like: {
        input: z.object({ id: z.string().min(1) }),
        output: z.number().int().nonnegative(),
      },
    },
  },
  {
    async initialize({ collection }) {
      const records = collection('colors')
      for (const [id, name, hex] of [
        ['blue', 'Ocean blue', '#247ac5'],
        ['green', 'Sea glass', '#439477'],
        ['coral', 'Coral', '#d36b5e'],
      ]) {
        // eslint-disable-next-line react-doctor/async-await-in-loop -- Each seed read and write is ordered within one mutable KV transaction.
        if (!(await records.get(id)))
          await records.put(id, { name, hex, likes: 0 })
      }
      return null
    },
    async like({ collection }, { id }) {
      // Read and increment inside the supplied World transaction.
      const records = collection('colors')
      const color = await records.get(id)
      if (!color)
        throw new SyncError('VALIDATION', 'This color is no longer available')
      const likes = color.likes + 1
      await records.put(id, { ...color, likes })
      return likes
    },
  },
)

/** rankedColors derives its watch from the same predicate and comparator that return the list. */
export const rankedColors = defineQuery(colors.schema, {
  name: 'ranked-colors',
  input: z.object({ search: z.string() }),
  async evaluate({ collection }, { search }) {
    const matching = (await collection('colors').scan()).filter(({ value }) =>
      value.name.toLowerCase().includes(search.toLowerCase()),
    )
    return matching.toSorted(
      (a, b) =>
        b.value.likes - a.value.likes ||
        a.value.name.localeCompare(b.value.name) ||
        a.key.localeCompare(b.key),
    )
  },
})
