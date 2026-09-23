import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { setImmediate } from 'node:timers/promises'
import type { StandardSchemaV1 } from '@standard-schema/spec'
import { createServer } from '../../core/sync/server.js'
import { openNodeEngine } from '../../core/sync/node/host.js'

import { AppQueries } from '../../sdk/sync/live-query.js'
import { defineQuery } from '../../sdk/sync/query.js'
import { defineApp } from '../../sdk/sync/app.js'
import { ApplicationTransaction } from '../../sdk/sync/transaction.js'
import { Application } from '../../core/sync/application.js'
import { defineSchema, type Principal } from '../../sdk/sync/schema.js'
import { SyncError } from '../../sdk/sync/errors.js'
import { collectionKey } from '../../sdk/sync/keys.js'
import { getObjectType, setObjectType } from '../../sdk/world/types/types.js'
import {
  executeAppOperation,
  readAppInstance,
  appLimits,
  type AppOperation,
} from '../../sdk/sync/instance.js'

function validator<T>(
  check: (value: unknown) => value is T,
): StandardSchemaV1<T> {
  return {
    '~standard': {
      version: 1,
      vendor: 'test',
      validate: (value) =>
        check(value) ? { value } : { issues: [{ message: 'invalid' }] },
    },
  }
}

const number = validator(
  (value): value is number =>
    typeof value === 'number' && Number.isFinite(value),
)
const nullable = validator(
  (value): value is string | null =>
    value === null || typeof value === 'string',
)
const schema = defineSchema({
  id: 'sync-acceptance',
  version: 1,
  collections: { counts: number, audit: number, notes: nullable },
  mutations: {
    increment: { input: number, output: number },
    fail: { input: number, output: number },
  },
})
const principal: Principal = { subject: 'alice', scope: 'team-a' }

test(
  'public Node app queries keep one snapshot, share producers, and recheck authority',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-node-query-'))
    let allowed = true
    const server = await createServer({
      directory,
      schema,
      authenticate: async () => principal,
      authorize: () => allowed,
      mutations: {
        increment: async ({ collection }, amount) => {
          const counts = collection('counts')
          const value = ((await counts.get('total')) ?? 0) + amount
          await counts.put('total', value)
          await collection('audit').put('total', value)
          return value
        },
        fail: async () => {
          throw new Error('unused')
        },
      },
    })
    const entered = Promise.withResolvers<void>()
    const resume = Promise.withResolvers<void>()
    let evaluations = 0
    const query = defineQuery(schema, {
      name: 'consistent-total',
      input: number,
      async evaluate({ collection }) {
        const count = (await collection('counts').get('total')) ?? 0
        if (++evaluations === 1) {
          entered.resolve()
          await resume.promise
        }
        const audit = (await collection('audit').get('total')) ?? 0
        return [count, audit]
      },
    })
    const source = server.as(principal)
    const first = source.watch(query, 0)[Symbol.asyncIterator]()
    const second = source.watch(query, 0)[Symbol.asyncIterator]()
    try {
      const initial = first.next()
      await entered.promise
      await source.mutate('increment', 1, { requestId: 'during-evaluation' })
      resume.resolve()
      assert.deepEqual((await initial).value.value, [0, 0])
      assert.deepEqual((await first.next()).value.value, [1, 1])
      assert.deepEqual((await second.next()).value.value, [1, 1])
      assert.equal(evaluations, 2, 'second subscriber reran the same query')
      await first.return?.()
      await source.mutate('increment', 2)
      assert.deepEqual((await second.next()).value.value, [3, 3])

      // A cached snapshot is not disclosed after the caller loses read access.
      allowed = false
      const denied = source.watch(query, 0)[Symbol.asyncIterator]()
      await assert.rejects(denied.next(), { code: 'DENIED' })
      await denied.return?.()
      allowed = true

      // Closing the host joins query work even when a consumer awaits its next value.
      const pending = second.next()
      await server.close()
      assert.equal((await pending).done, true)
      await assert.rejects(source.mutate('increment', 1), { code: 'CLOSED' })
    } finally {
      resume.resolve()
      await server.close()
      await first.return?.()
      await second.return?.()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'plugin app instances accept records and receipts only with their supplied World writer',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-plugin-app-'))
    const owner = await openNodeEngine(directory)
    const app = defineApp(
      {
        ...schema,
        mutations: {
          ...schema.mutations,
          unawaited: { input: number, output: number },
        },
      },
      {
        increment: async ({ collection }, amount) => {
          const counts = collection('counts')
          const value = ((await counts.get('total')) ?? 0) + amount
          await counts.put('total', value)
          return value
        },
        fail: async ({ collection }, amount) => {
          await collection('counts').put('total', amount)
          throw new Error('operation rejected')
        },
        unawaited: async ({ collection }, amount) => {
          void collection('counts').put('total', amount)
          return amount
        },
      },
    )
    const revision = {
      pluginId: 'colors',
      manifestRoot: 'immutable-test-artifact',
    }
    const signal = AbortSignal.timeout(55_000)
    const initial = {
      objectKey: 'colors/one',
      requestId: 'create-one',
      mutation: { kind: 'mutate' as const, name: 'increment', input: 2 },
    }
    const run = async (
      create: boolean,
      request: AppOperation,
      commit = true,
    ) => {
      const tx = await owner.engine.newTransaction(true, signal)
      try {
        const result = await executeAppOperation(
          app,
          revision,
          tx,
          'alice',
          create,
          request,
          signal,
        )
        if (commit) await tx.commit(signal)
        return result
      } finally {
        try {
          await tx.discard()
        } finally {
          tx.release()
        }
      }
    }
    try {
      // Abandoning the caller's writer leaves neither the app nor its initialized records.
      assert.equal(await run(true, initial, false), 2)
      let read = await owner.engine.newTransaction(false, signal)
      try {
        assert.equal(await read.getObject(initial.objectKey, signal), null)
      } finally {
        await read.discard()
        read.release()
      }
      assert.equal(await run(true, initial), 2)
      assert.equal(await run(true, initial), 2)
      await assert.rejects(run(true, { ...initial, mutation: undefined }), {
        code: 'CONFLICT',
      })
      assert.equal(
        await run(true, {
          ...initial,
          objectKey: 'colors/two',
          requestId: 'create-two',
        }),
        2,
      )

      // A duplicate mutation has the same retained result and cannot increment twice.
      const increment = { ...initial, requestId: 'like-one' }
      assert.equal(await run(false, increment), 4)
      assert.equal(await run(false, increment), 4)
      await assert.rejects(
        run(false, {
          ...increment,
          mutation: { ...increment.mutation, input: 10 },
        }),
        { code: 'CONFLICT' },
      )
      await assert.rejects(
        run(false, {
          ...initial,
          requestId: 'fail',
          mutation: { ...initial.mutation, name: 'fail', input: 99 },
        }),
      )
      await assert.rejects(
        run(false, {
          ...initial,
          requestId: 'unawaited',
          mutation: { kind: 'mutate', name: 'unawaited', input: 99 },
        }),
        { code: 'VALIDATION' },
      )

      // Receipt recovery reads the same revision-bound contract without executing code.
      read = await owner.engine.newTransaction(false, signal)
      try {
        const one = await readAppInstance(read, 'colors/one', schema, signal)
        const two = await readAppInstance(read, 'colors/two', schema, signal)
        assert.notEqual(one.instance, two.instance)
        for (const [binding, expected] of [
          [one, 4],
          [two, 2],
        ] as const) {
          const unit = new ApplicationTransaction(read, {
            schema,
            mutations: app.mutations,
            instance: binding.instance,
            revision: binding.manifestRoot,
            limits: appLimits,
            write: false,
            signal,
            authorize: async () => {},
          })
          try {
            assert.equal(
              await unit
                .collections({ subject: 'alice', scope: 'shared' })
                .collection('counts')
                .get('total'),
              expected,
            )
            if (binding === one) {
              assert.deepEqual(
                await unit.readAcceptance(
                  { subject: 'alice', scope: 'shared' },
                  increment.mutation,
                  increment.requestId,
                ),
                { value: 4 },
              )
            }
          } finally {
            await unit.release()
          }
        }
        await assert.rejects(
          executeAppOperation(
            app,
            revision,
            read,
            '',
            false,
            increment,
            signal,
          ),
          { code: 'AUTHENTICATION' },
        )
        await assert.rejects(
          executeAppOperation(
            app,
            { ...revision, manifestRoot: 'new-artifact' },
            read,
            'alice',
            false,
            increment,
            signal,
          ),
          { code: 'SCHEMA_MISMATCH' },
        )
      } finally {
        await read.discard()
        read.release()
      }
      assert.ok(await owner.engine.getSeqno(signal))
    } finally {
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'internal server attachment preserves a supplied engine and existing ObjectTypes',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-supplied-'))
    const owner = await openNodeEngine(directory)
    try {
      const publicSchema = defineSchema({
        id: 'public-api',
        version: 1,
        collections: { notes: nullable },
        mutations: {},
      })
      const server = await createServer({
        schema: publicSchema,
        engine: owner.engine,
        mutations: {},
        authenticate: async () => principal,
        authorize: ({ principal }) => principal.subject === 'alice',
      })
      try {
        await server
          .as(principal)
          .collection('notes')
          .put('\ufeffkey', null, { requestId: 'null' })
        assert.equal(
          await server.as(principal).collection('notes').get('\ufeffkey'),
          null,
        )
        assert.deepEqual(
          await server.as(principal).collection('notes').scan(),
          [{ key: '\ufeffkey', value: null }],
        )
        await assert.rejects(
          server
            .as({ subject: 'denied', scope: 'team-a' })
            .collection('notes')
            .put('x', 'secret'),
          { code: 'DENIED' },
        )
        await server.admin('team-b').collection('notes').put('admin', 'allowed')
        assert.equal(
          await server
            .as({ ...principal, scope: 'team-b' })
            .collection('notes')
            .get('admin'),
          'allowed',
        )
        const key = collectionKey(publicSchema.id, 'wrong-type', 'notes')
        const tx = await owner.engine.newTransaction(true)
        try {
          const object = await tx.createObject(key, {})
          object.release()
          await setObjectType(tx, key, 'different/type')
          await tx.commit()
        } finally {
          await tx.discard()
          tx.release()
        }
        await assert.rejects(
          server.admin('wrong-type').collection('notes').put('key', 'value'),
          { code: 'SCHEMA_MISMATCH' },
        )
        const read = await owner.engine.newTransaction(false)
        try {
          assert.equal(await getObjectType(read, key), 'different/type')
        } finally {
          await read.discard()
          read.release()
        }
      } finally {
        await server.close()
      }
      assert.ok(await owner.engine.getSeqno())
    } finally {
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

async function attach(
  directory: string,
  version = 1,
  migration?: Parameters<
    Application<typeof schema, Principal>['initialize']
  >[0],
) {
  const owner = await openNodeEngine(directory)
  let handlerRuns = 0
  const app = new Application({
    schema: { ...schema, version },
    engine: owner.engine,
    authorize: ({ principal }) => principal.subject !== 'denied',
    mutations: {
      increment: async (tx, amount) => {
        handlerRuns++
        const counts = tx.collection('counts')
        const value = ((await counts.get('total')) ?? 0) + amount
        await counts.put('total', value)
        await tx.collection('audit').put('total', value)
        return value
      },
      fail: async (tx, amount) => {
        await tx.collection('counts').put('total', amount)
        await tx.collection('audit').put('total', amount)
        throw new SyncError(
          'VALIDATION',
          'Business rule rejected the operation',
        )
      },
    },
  })
  try {
    await app.initialize(migration)
  } catch (error) {
    await app.close()
    await owner.close()
    throw error
  }
  return {
    app,
    owner,
    runs: () => handlerRuns,
    close: async () => {
      await app.close()
      await owner.close()
    },
  }
}

test(
  'compiled application atomically changes two ordinary collections and retains receipts',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-application-'))
    try {
      let server = await attach(directory)
      try {
        const request = { kind: 'mutate', name: 'increment', input: 2 } as const
        assert.equal(
          await server.app.execute(principal, request, { requestId: 'once' }),
          2,
        )
        assert.equal(
          await server.app.execute(principal, request, { requestId: 'once' }),
          2,
        )
        assert.equal(server.runs(), 1)
        await assert.rejects(
          server.app.execute(
            principal,
            { ...request, input: 3 },
            { requestId: 'once' },
          ),
          { code: 'CONFLICT' },
        )
        await assert.rejects(
          server.app.execute(principal, {
            kind: 'mutate',
            name: 'fail',
            input: 99,
          }),
          { code: 'VALIDATION' },
        )
        for (const collection of ['counts', 'audit']) {
          assert.deepEqual(
            await server.app.execute(principal, {
              kind: 'get',
              collection,
              key: 'total',
            }),
            { found: true, value: 2 },
          )
        }
        const accepted = await Promise.all([
          server.app.execute(principal, {
            kind: 'mutate',
            name: 'increment',
            input: 1,
          }),
          server.app.execute(principal, {
            kind: 'mutate',
            name: 'increment',
            input: 1,
          }),
        ])
        assert.deepEqual(accepted.sort(), [3, 4])
        await server.app.close()
        assert.ok(await server.owner.engine.getSeqno())
      } finally {
        await server.close()
      }
      server = await attach(directory)
      try {
        assert.equal(
          await server.app.execute(
            principal,
            { kind: 'mutate', name: 'increment', input: 2 },
            { requestId: 'once' },
          ),
          2,
        )
        assert.equal(server.runs(), 0)
        assert.deepEqual(
          await server.app.execute(principal, {
            kind: 'get',
            collection: 'counts',
            key: 'total',
          }),
          { found: true, value: 4 },
        )
      } finally {
        await server.close()
      }
    } finally {
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'compiled application distinguishes null, validates writes, and isolates authorized scopes',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-policy-'))
    try {
      const server = await attach(directory)
      try {
        await server.app.execute(principal, {
          kind: 'put',
          collection: 'notes',
          key: 'nullable',
          value: null,
        })
        assert.deepEqual(
          await server.app.execute(principal, {
            kind: 'get',
            collection: 'notes',
            key: 'nullable',
          }),
          { found: true, value: null },
        )
        assert.deepEqual(
          await server.app.execute(principal, {
            kind: 'get',
            collection: 'notes',
            key: 'missing',
          }),
          { found: false },
        )
        assert.deepEqual(
          await server.app.execute(
            { ...principal, scope: 'team-b' },
            { kind: 'scan', collection: 'notes', prefix: '' },
          ),
          [],
        )
        await assert.rejects(
          server.app.execute(
            { ...principal, subject: 'denied' },
            { kind: 'get', collection: 'notes', key: 'nullable' },
          ),
          { code: 'DENIED' },
        )
        await assert.rejects(
          server.app.execute(principal, {
            kind: 'put',
            collection: 'notes',
            key: 'bad',
            value: 42,
          }),
          { code: 'VALIDATION' },
        )
        await assert.rejects(
          server.app.execute(principal, {
            kind: 'get',
            collection: 'unknown',
            key: 'x',
          }),
          { code: 'MISSING_COLLECTION' },
        )
        const expired = { ...principal, expiresAt: Date.now() - 1 }
        await assert.rejects(
          server.app.execute(expired, {
            kind: 'scan',
            collection: 'notes',
            prefix: '',
          }),
          { code: 'AUTHENTICATION' },
        )
        const revoke = new AbortController()
        revoke.abort()
        await assert.rejects(
          server.app.execute(
            { ...principal, signal: revoke.signal },
            { kind: 'scan', collection: 'notes', prefix: '' },
          ),
          { code: 'AUTHENTICATION' },
        )
      } finally {
        await server.close()
      }
    } finally {
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'maintenance migration commits data and version together and recovers after failure',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-migrate-'))
    try {
      const first = await attach(directory)
      try {
        await first.app.execute(principal, {
          kind: 'put',
          collection: 'counts',
          key: 'total',
          value: 1,
        })
      } finally {
        await first.close()
      }
      await assert.rejects(attach(directory, 2), { code: 'SCHEMA_MISMATCH' })
      await assert.rejects(
        attach(directory, 2, {
          from: 1,
          run: async (tx) => {
            await tx
              .scope(principal.scope)
              .collection('counts')
              .put('total', 99)
            throw new Error('interrupted migration')
          },
        }),
        /interrupted migration/,
      )
      const afterFailure = await attach(directory)
      try {
        assert.deepEqual(
          await afterFailure.app.execute(principal, {
            kind: 'get',
            collection: 'counts',
            key: 'total',
          }),
          { found: true, value: 1 },
        )
      } finally {
        await afterFailure.close()
      }
      const migrated = await attach(directory, 2, {
        from: 1,
        run: async (tx) => {
          assert.deepEqual(await tx.scopes(), [principal.scope])
          await tx.scope(principal.scope).collection('counts').put('total', 10)
        },
      })
      try {
        assert.deepEqual(
          await migrated.app.execute(principal, {
            kind: 'get',
            collection: 'counts',
            key: 'total',
          }),
          { found: true, value: 10 },
        )
      } finally {
        await migrated.close()
      }
      const reopened = await attach(directory, 2)
      await reopened.close()
    } finally {
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'application instances share one World without sharing records or receipts',
  { timeout: 60_000 },
  async () => {
    // Keep both application instances attached to one real Resource connection.
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-instances-'))
    const owner = await openNodeEngine(directory)
    const apps = ['colors/first', 'colors/second'].map(
      (instance) =>
        new Application({
          schema,
          instance,
          engine: owner.engine,
          authorize: () => true,
          mutations: {
            increment: async (tx, amount) => {
              const counts = tx.collection('counts')
              const next = ((await counts.get('total')) ?? 0) + amount
              await counts.put('total', next)
              return next
            },
            fail: async () => {
              throw new Error('unused')
            },
          },
        }),
    )
    try {
      for (const app of apps) await app.initialize()

      // Equal request IDs belong to separate datasets, including their receipts.
      const operation = { kind: 'mutate', name: 'increment', input: 1 } as const
      assert.equal(
        await apps[0].execute(principal, operation, { requestId: 'same' }),
        1,
      )
      assert.equal(
        await apps[1].execute(principal, operation, { requestId: 'same' }),
        1,
      )
      assert.equal(
        await apps[0].execute(principal, operation, { requestId: 'next' }),
        2,
      )
      assert.equal(
        await apps[1].execute(principal, operation, { requestId: 'same' }),
        1,
      )
      assert.deepEqual(
        await apps[1].execute(principal, {
          kind: 'get',
          collection: 'counts',
          key: 'total',
        }),
        { found: true, value: 1 },
      )

      // Releasing both attachments leaves their supplied World usable.
      await Promise.all(apps.map((app) => app.close()))
      assert.ok(await owner.engine.getSeqno())
    } finally {
      await Promise.all(apps.map((app) => app.close()))
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'supplied WorldState stages collection changes and receipts until its caller commits',
  { timeout: 60_000 },
  async () => {
    // Use the same app definition through the Node host and the supplied-state path.
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-supplied-state-'))
    const owner = await openNodeEngine(directory)
    const definition = defineApp(schema, {
      increment: async (tx, amount) => {
        const counts = tx.collection('counts')
        const next = ((await counts.get('total')) ?? 0) + amount
        await counts.put('total', next)
        await tx.collection('audit').put('total', next)
        return next
      },
      fail: async () => {
        throw new Error('unused')
      },
    })
    const app = new Application({
      ...definition,
      engine: owner.engine,
      authorize: () => true,
    })
    const request = { kind: 'mutate', name: 'increment', input: 1 } as const
    try {
      await app.initialize()
      for (const commit of [false, true]) {
        const tx = await owner.engine.newTransaction(true)
        const unit = new ApplicationTransaction(tx, {
          ...definition,
          instance: schema.id,
          write: true,
          signal: new AbortController().signal,
          limits: app.limits,
          authorize: async () => {},
        })
        try {
          assert.deepEqual(await unit.execute(principal, request, 'supplied'), {
            value: 1,
            duplicate: false,
          })
          await unit.flush()
          await unit.release()

          // The collection attachment cannot dispose its caller's World handle.
          const sibling = await tx.createObject('other/caller-owned', {})
          sibling.release()
          if (commit) await tx.commit()
        } finally {
          await unit.release()
          await tx.discard()
          tx.release()
        }
        assert.deepEqual(
          await app.execute(principal, {
            kind: 'get',
            collection: 'counts',
            key: 'total',
          }),
          commit ? { found: true, value: 1 } : { found: false },
        )
      }

      // A retry through the other host finds the same receipt and does not increment.
      assert.equal(
        await app.execute(principal, request, { requestId: 'supplied' }),
        1,
      )
      assert.equal(
        await app.execute(principal, request, { requestId: 'new' }),
        2,
      )
    } finally {
      await app.close()
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'function queries share snapshots, filter changes, and follow external World writes',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-live-query-'))
    const owner = await openNodeEngine(directory)
    const definition = defineApp(schema, {
      increment: async (tx, amount) => {
        const counts = tx.collection('counts')
        const value = ((await counts.get('total')) ?? 0) + amount
        await counts.put('total', value)
        return value
      },
      fail: async () => {
        throw new Error('unused')
      },
    })
    const app = new Application({
      ...definition,
      engine: owner.engine,
      authorize: () => true,
    })
    const queries = new AppQueries({
      ...definition,
      instance: schema.id,
      engine: owner.engine,
      principal,
      authorize: async () => {},
      limits: app.limits,
    })
    const lifetime = new AbortController()
    let evaluations = 0
    const query = defineQuery(schema, {
      name: 'counts',
      input: number,
      evaluate: async (tx, minimum) => {
        evaluations++
        return (await tx.collection('counts').scan()).filter(
          ({ value }) => value >= minimum,
        )
      },
    })
    const first = queries
      .watch(query, 1, lifetime.signal)
      [Symbol.asyncIterator]()
    const second = queries
      .watch(query, 1, lifetime.signal)
      [Symbol.asyncIterator]()
    try {
      await app.initialize()
      const [a, b] = await Promise.all([first.next(), second.next()])
      assert.equal(a.value.status, 'current')
      assert.deepEqual(a.value.value, [])
      assert.deepEqual(a.value, b.value)
      assert.equal(evaluations, 1)

      // Another collection advances the World without evaluating this query.
      await app.execute(principal, {
        kind: 'put',
        collection: 'audit',
        key: 'other',
        value: 100,
      })
      await app.execute(principal, {
        kind: 'mutate',
        name: 'increment',
        input: 1,
      })
      const [nextA, nextB] = await Promise.all([first.next(), second.next()])
      assert.deepEqual(nextA.value.value, [{ key: 'total', value: 1 }])
      assert.deepEqual(nextA.value, nextB.value)
      assert.equal(evaluations, 2)

      // A direct supplied-state writer is observed without any application event bus.
      const tx = await owner.engine.newTransaction(true)
      const unit = new ApplicationTransaction(tx, {
        ...definition,
        instance: schema.id,
        limits: app.limits,
        write: true,
        signal: lifetime.signal,
        authorize: async () => {},
      })
      try {
        await unit
          .collections(principal)
          .collection('counts')
          .put('external', 2)
        await unit.flush()
        await tx.commit()
      } finally {
        await unit.release()
        await tx.discard()
        tx.release()
      }
      const external = await first.next()
      assert.deepEqual(external.value.value, [
        { key: 'external', value: 2 },
        { key: 'total', value: 1 },
      ])
      assert.equal(evaluations, 3)
      lifetime.abort()
      await Promise.all([first.return?.(), second.return?.()])
      await queries.close()
      assert.ok(await owner.engine.getSeqno())
    } finally {
      lifetime.abort()
      await Promise.all([first.return?.(), second.return?.()])
      await queries.close()
      await app.close()
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'immutable record comparison skips unrelated keys before query scans',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-key-filter-'))
    const owner = await openNodeEngine(directory)
    const definition = defineApp(schema, {
      increment: async () => 0,
      fail: async () => 0,
    })
    const app = new Application({
      ...definition,
      engine: owner.engine,
      authorize: () => true,
    })
    const queries = new AppQueries({
      ...definition,
      instance: schema.id,
      engine: owner.engine,
      principal,
      authorize: async () => {},
      limits: app.limits,
    })
    const controller = new AbortController()
    const query = defineQuery(schema, {
      name: 'watched',
      input: number,
      evaluate: (context) =>
        context.collection('counts').scan({ prefix: 'watched/' }),
    })
    const iterator = queries
      .watch(query, 0, controller.signal)
      [Symbol.asyncIterator]()
    try {
      await app.initialize()
      await app.execute(principal, {
        kind: 'put',
        collection: 'counts',
        key: 'watched/one',
        value: 1,
      })
      assert.deepEqual((await iterator.next()).value.value, [
        { key: 'watched/one', value: 1 },
      ])
      const before = queries.diagnostics()
      const baseSnapshot = await owner.engine.newTransaction(false)
      const [base] = await baseSnapshot.getObjectRootRefs([
        collectionKey(schema.id, principal.scope, 'counts'),
      ])
      await baseSnapshot.discard()
      baseSnapshot.release()
      await app.execute(principal, {
        kind: 'put',
        collection: 'counts',
        key: 'unwatched/other',
        value: 2,
      })
      const acceptedSeqno = (await owner.engine.getSeqno()).seqno ?? 0n

      // Inspect the real snapshot's complete metadata, including the exact record key.
      const snapshot = await owner.engine.newTransaction(false)
      try {
        const metadata = await snapshot.compareObjectRecords([
          { objectKey: base.objectKey, rootRef: base.rootRef },
        ])
        assert.equal(metadata.changes?.[0].unknown ?? false, false)
        assert.deepEqual(
          metadata.changes?.[0].keys?.map((key) =>
            new TextDecoder().decode(key),
          ),
          ['unwatched/other'],
        )
      } finally {
        await snapshot.discard()
        snapshot.release()
      }

      // Wait for the source to finish filtering this accepted revision, not a timing guess.
      const deadline = Date.now() + 5000
      while (queries.diagnostics().processedSeqno < acceptedSeqno) {
        assert.ok(
          Date.now() < deadline,
          'query did not process the accepted revision',
        )
        await setImmediate()
      }
      const after = queries.diagnostics()
      assert.equal(after.evaluations, before.evaluations)
      assert.equal(after.rangeScans, before.rangeScans)
      assert.equal(after.keyReads, before.keyReads)
      await app.execute(principal, {
        kind: 'put',
        collection: 'counts',
        key: 'watched/two',
        value: 3,
      })
      assert.deepEqual((await iterator.next()).value.value, [
        { key: 'watched/one', value: 1 },
        { key: 'watched/two', value: 3 },
      ])
    } finally {
      controller.abort()
      await iterator.return?.()
      await queries.close()
      await app.close()
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)
