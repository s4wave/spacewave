import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { z } from 'zod'

import { openNodeEngine } from '../../core/sync/node/host.js'
import {
  defineApp,
  type AppDefinition,
  type AppMigrationContext,
} from '../../sdk/sync/app.js'
import {
  appLimits,
  executeAppOperation,
  readAppInstance,
  type AppInstance,
} from '../../sdk/sync/instance.js'
import { executeAppUpgrade } from '../../sdk/sync/upgrade.js'
import { ApplicationTransaction } from '../../sdk/sync/transaction.js'
import { collectionKey } from '../../sdk/sync/keys.js'
import type { Schema } from '../../sdk/sync/schema.js'

/** oldApp stores numeric votes before the explicit record-schema migration. */
const oldApp = defineApp(
  {
    id: 'upgrade-colors',
    version: 1,
    collections: { counts: z.number().int(), retired: z.string() },
    mutations: { like: { input: z.string(), output: z.number() } },
  },
  {
    async like({ collection }, key) {
      const counts = collection('counts')
      const total = ((await counts.get(key)) ?? 0) + 1
      await counts.put(key, total)
      await collection('retired').put('label', 'Blue')
      return total
    },
  },
)

const nextSchema = {
  id: oldApp.schema.id,
  version: 2,
  collections: {
    colors: z.object({ name: z.string(), likes: z.number().int() }),
  },
  mutations: {},
} as const

test(
  'app upgrades validate, migrate atomically, retain identity, and reject stale revisions',
  { timeout: 60_000 },
  async () => {
    // Use the compiled World and production collection Resources for every acceptance.
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-upgrade-'))
    let owner = await openNodeEngine(directory)
    const signal = AbortSignal.timeout(55_000)
    const revision = { pluginId: 'colors', manifestRoot: 'first' }
    const key = 'colors/one'
    const principal = { subject: 'alice', scope: 'shared' }
    let migrations = 0
    let escaped: AppMigrationContext<typeof nextSchema> | undefined
    const nextApp = defineApp(
      nextSchema,
      {},
      {
        1: async (context) => {
          migrations++
          escaped = context
          const old = context.previous.collection('counts')
          const label = z
            .string()
            .parse(await context.previous.collection('retired').get('label'))
          for (const entry of await old.scan()) {
            await context
              .collection('colors')
              .put(entry.key, {
                name: label,
                likes: z.number().parse(entry.value),
              })
          }
        },
      },
    )
    const binding = async () => {
      const tx = await owner.engine.newTransaction(false, signal)
      try {
        return await readAppInstance(tx, key, { id: oldApp.schema.id }, signal)
      } finally {
        await tx.discard()
        tx.release()
      }
    }
    const upgrade = async <S extends Schema>(
      app: AppDefinition<S>,
      root: string,
      from: AppInstance,
      requestId: string,
      commit = true,
    ) => {
      const tx = await owner.engine.newTransaction(true, signal)
      try {
        await executeAppUpgrade(
          app,
          { ...revision, manifestRoot: root },
          tx,
          'alice',
          { objectKey: key, requestId, from },
          signal,
        )
        if (commit) await tx.commit(signal)
      } finally {
        await tx.discard()
        tx.release()
      }
    }
    try {
      // A compatible module revision keeps records and instance identity.
      const create = await owner.engine.newTransaction(true, signal)
      try {
        await executeAppOperation(
          oldApp,
          revision,
          create,
          'alice',
          true,
          {
            objectKey: key,
            requestId: 'create',
            mutation: { kind: 'mutate', name: 'like', input: 'blue' },
          },
          signal,
        )
        await create.commit(signal)
      } finally {
        await create.discard()
        create.release()
      }
      const original = await binding()
      await upgrade(oldApp, 'compatible', original, 'compatible')
      const compatible = await binding()
      assert.equal(compatible.instance, original.instance)
      assert.equal(compatible.manifestRoot, 'compatible')
      await upgrade(oldApp, 'compatible', original, 'compatible')
      assert.deepEqual(await binding(), compatible)
      await assert.rejects(upgrade(oldApp, 'different', original, 'stale'), {
        code: 'CONFLICT',
      })
      const invalid = defineApp(
        {
          ...oldApp.schema,
          collections: { counts: z.string(), retired: z.string() },
        },
        { like: async () => 0 },
      )
      await assert.rejects(upgrade(invalid, 'invalid', compatible, 'invalid'), {
        code: 'VALIDATION',
      })
      assert.deepEqual(await binding(), compatible)

      // Missing, failing, and unaccepted migrations preserve the old data and binding.
      await assert.rejects(
        upgrade(defineApp(nextSchema, {}), 'missing', compatible, 'missing'),
        { code: 'SCHEMA_MISMATCH' },
      )
      const failure = defineApp(
        nextSchema,
        {},
        {
          1: async ({ collection }) => {
            await collection('colors').put('blue', {
              name: 'Wrong',
              likes: 999,
            })
            throw new Error('migration failed')
          },
        },
      )
      await assert.rejects(
        upgrade(failure, 'failure', compatible, 'failure'),
        /migration failed/,
      )
      assert.deepEqual(await binding(), compatible)
      await upgrade(nextApp, 'second', compatible, 'migrate', false)
      assert.deepEqual(await binding(), compatible)
      await upgrade(nextApp, 'second', compatible, 'migrate')
      await upgrade(nextApp, 'second', compatible, 'migrate')
      assert.equal(migrations, 2)
      const migrated = await binding()
      assert.equal(migrated.instance, original.instance)
      assert.equal(migrated.version, 2)
      assert.equal(migrated.manifestRoot, 'second')
      await assert.rejects(
        escaped!
          .collection('colors')
          .put('blue', { name: 'Escaped', likes: 99 }),
        { code: 'CLOSED' },
      )
      await assert.rejects(escaped!.previous.collection('counts').get('blue'), {
        code: 'CLOSED',
      })
      await assert.rejects(
        upgrade(oldApp, 'compatible', migrated, 'downgrade'),
        { code: 'SCHEMA_MISMATCH' },
      )

      // Reopening retains the migrated values, removes retired collections, and leaves old calls conflicted.
      await owner.close()
      owner = await openNodeEngine(directory)
      assert.deepEqual(await binding(), migrated)
      const read = await owner.engine.newTransaction(false, signal)
      const unit = new ApplicationTransaction(read, {
        schema: nextSchema,
        mutations: {},
        instance: migrated.instance,
        limits: appLimits,
        write: false,
        signal,
        authorize: async () => {},
      })
      try {
        assert.deepEqual(
          await unit.collections(principal).collection('colors').get('blue'),
          { name: 'Blue', likes: 1 },
        )
        assert.equal(
          await read.getObject(
            collectionKey(original.instance, 'shared', 'retired'),
            signal,
          ),
          null,
        )
        assert.equal(
          await read.getObject(
            collectionKey(original.instance, 'shared', 'counts'),
            signal,
          ),
          null,
        )
        await assert.rejects(
          executeAppOperation(
            oldApp,
            revision,
            read,
            'alice',
            false,
            {
              objectKey: key,
              requestId: 'late-old-call',
              mutation: { kind: 'mutate', name: 'like', input: 'blue' },
            },
            signal,
          ),
          { code: 'SCHEMA_MISMATCH' },
        )
      } finally {
        await unit.release()
        await read.discard()
        read.release()
      }
    } finally {
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)
