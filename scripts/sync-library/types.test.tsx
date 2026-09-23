import {
  connect,
  defineSchema,
  defineQuery,
  type AppSource,
  type Database,
  type SubscriptionState,
} from 'spacewave'
import { createServer } from 'spacewave/server'
import { createSyncContext } from 'spacewave/react'
import { z } from 'zod'

const schema = defineSchema({
  id: 'types',
  version: 1,
  collections: { counts: z.string().transform(Number) },
  mutations: {
    increment: { input: z.object({ amount: z.number() }), output: z.number() },
  },
})
declare const db: Database<typeof schema>
const value: number | undefined = await db.collection('counts').get('a')
await db.collection('counts').put('a', '12')
// @ts-expect-error Writes accept the validator input, not its transformed output.
await db.collection('counts').put('a', 12)
// @ts-expect-error Collection names come from the shared schema.
db.collection('missing')
const count: number = await db.mutate('increment', { amount: 1 })
// @ts-expect-error Mutation input is inferred from its declared validator.
db.mutate('increment', { amount: 'one' })
// @ts-expect-error Mutation results retain their output type.
const invalid: string = await db.mutate('increment', { amount: 1 })
const { SyncProvider, useCollection } = createSyncContext(schema)
const state: SubscriptionState<number> = useCollection('counts')
const component = (
  <SyncProvider db={db}>
    <span>{state.status}</span>
  </SyncProvider>
)
const server = await createServer({
  directory: '/unused-types-only',
  schema,
  authenticate: async () => ({
    subject: 'a',
    scope: 'b',
    role: 'editor' as const,
  }),
  authorize: ({ principal }) => principal.role === 'editor',
  mutations: {
    increment: async (context, input) => {
      const previous: number | undefined = await context
        .collection('counts')
        .get('a')
      await context
        .collection('counts')
        .put('a', String((previous ?? 0) + input.amount))
      return (previous ?? 0) + input.amount
    },
  },
})
await server
  .as({ subject: 'a', scope: 'b', role: 'editor' })
  .collection('counts')
  .put('a', '2')
const source: AppSource<typeof schema> = server.as({
  subject: 'a',
  scope: 'b',
  role: 'editor',
})
const sum = defineQuery(schema, {
  name: 'sum',
  input: z.string(),
  evaluate: async ({ collection }, prefix) =>
    (await collection('counts').scan({ prefix })).reduce(
      (total, row) => total + row.value,
      0,
    ),
})
for await (const snapshot of source.watch(sum, 'a')) {
  if (snapshot.status === 'current') {
    const total: number = snapshot.value
    void total
  }
}
const connected: Database<typeof schema> = await connect({
  url: 'ws://localhost/sync',
  schema,
  getAccessToken: () => 'token',
})
void [value, count, invalid, component, connected]
