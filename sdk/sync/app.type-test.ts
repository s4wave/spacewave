import type { StandardSchemaV1 } from '@standard-schema/spec'

import { createAccess } from './access.js'
import { defineApp } from './app.js'

declare const color: StandardSchemaV1<{ name: string; likes: number }>
declare const text: StandardSchemaV1<string>
declare const number: StandardSchemaV1<number>
const app = defineApp(
  {
    id: 'colors',
    version: 1,
    collections: { colors: color },
    mutations: { like: { input: text, output: number } },
  },
  {
    async like(tx, id) {
      // Read the accepted count before applying this named increment.
      const colors = tx.collection('colors')
      const previous = await colors.get(id)
      const likes = (previous?.likes ?? 0) + 1
      await colors.put(id, { name: previous?.name ?? id, likes })

      // @ts-expect-error A handler cannot address undeclared collections.
      tx.collection('missing')
      return likes
    },
  },
)
const access = createAccess<typeof app.schema>(async () => null)
const result: Promise<number> = access.mutate('like', 'red')
void result
// @ts-expect-error Named operation inputs retain their validator type.
void access.mutate('like', 1)
// @ts-expect-error Only declared operation names are accepted.
void access.mutate('missing', 'red')
