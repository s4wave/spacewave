import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { setImmediate } from 'node:timers/promises'
import { z } from 'zod'

import { openNodeEngine } from '../../core/sync/node/host.js'
import { Application } from '../../core/sync/application.js'
import { defineApp } from '../../sdk/sync/app.js'
import { AppQueries } from '../../sdk/sync/live-query.js'
import type { Operation } from '../../sdk/sync/operation.js'
import { defineQuery } from '../../sdk/sync/query.js'
import { ApplicationTransaction } from '../../sdk/sync/transaction.js'

const color = z.object({
  name: z.string(),
  likes: z.number(),
  visible: z.boolean(),
  note: z.string(),
})
const definition = defineApp(
  {
    id: 'query-workload',
    version: 1,
    collections: { colors: color },
    mutations: {
      seed: { input: z.number().int(), output: z.null() },
      like: { input: z.string(), output: z.null() },
    },
  },
  {
    seed: async ({ collection }, size) => {
      const colors = collection('colors')
      for (let index = 0; index < size; index++) {
        await colors.put(`color/${index.toString().padStart(5, '0')}`, {
          name: `Color ${index}`,
          likes: index,
          visible: index % 10 === 0,
          note: '',
        })
      }
      return null
    },
    like: async ({ collection }, key) => {
      const colors = collection('colors')
      const value = await colors.get(key)
      assert.ok(value)
      await colors.put(key, { ...value, likes: value.likes + 1 })
      return null
    },
  },
)
const ranking = defineQuery(definition.schema, {
  name: 'ranked-colors',
  input: z.null(),
  evaluate: async ({ collection }) =>
    (await collection('colors').scan({ prefix: 'color/' }))
      .filter(({ value }) => value.visible)
      .toSorted((a, b) => b.value.likes - a.value.likes)
      .slice(0, 10)
      .map(({ key, value }) => ({ key, name: value.name, likes: value.likes })),
})

test(
  'three function-query subscribers match full reevaluation over 1000 records',
  { timeout: 110_000 },
  async (t) => {
    const size = 1000
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-query-workload-'))
    const owner = await openNodeEngine(directory)
    const principal = { subject: 'reader', scope: 'shared', signal: t.signal }
    const app = new Application({
      ...definition,
      engine: owner.engine,
      authorize: () => true,
    })
    const queries = new AppQueries({
      ...definition,
      engine: owner.engine,
      instance: definition.schema.id,
      principal,
      limits: app.limits,
      authorize: async () => {},
    })
    const controller = new AbortController()
    const signal = AbortSignal.any([controller.signal, t.signal])
    const iterators = Array.from({ length: 3 }, () =>
      queries.watch(ranking, null, signal)[Symbol.asyncIterator](),
    )
    try {
      await app.initialize()
      const seedStart = performance.now()
      await app.execute(principal, {
        kind: 'mutate',
        name: 'seed',
        input: size,
      })
      const seedMs = performance.now() - seedStart
      const fullStart = performance.now()
      const initial = await Promise.all(
        iterators.map((iterator) => iterator.next()),
      )
      let current = initial[0].value.value
      for (const result of initial) {
        assert.equal(result.value.status, 'current')
        assert.deepEqual(result.value.value, current)
      }
      assert.equal(queries.diagnostics().evaluations, 1)
      const fullMs = performance.now() - fullStart
      const measurements = []
      const value = {
        name: 'Changed',
        likes: size + 1,
        visible: false,
        note: 'edited',
      }
      const changes: {
        label: string
        operation?: Operation
        reads: number
        affected: boolean
      }[] = [
        { label: 'unrelated-world-object', reads: 0, affected: false },
        {
          label: 'outside-prefix',
          operation: { kind: 'put', collection: 'colors', key: 'other', value },
          reads: 0,
          affected: false,
        },
        {
          label: 'unused-field',
          operation: {
            kind: 'put',
            collection: 'colors',
            key: 'color/00001',
            value,
          },
          reads: 1,
          affected: false,
        },
        {
          label: 'like',
          operation: { kind: 'mutate', name: 'like', input: 'color/00990' },
          reads: 1,
          affected: true,
        },
        {
          label: 'insert',
          operation: {
            kind: 'put',
            collection: 'colors',
            key: 'color/01000',
            value: { ...value, visible: true },
          },
          reads: 1,
          affected: true,
        },
        {
          label: 'delete',
          operation: {
            kind: 'delete',
            collection: 'colors',
            key: 'color/00990',
          },
          reads: 1,
          affected: true,
        },
      ]

      // Measure changes and a full snapshot baseline through the same compiled Engine.
      for (const change of changes) {
        signal.throwIfAborted()
        const before = queries.diagnostics()
        const start = performance.now()
        if (change.operation) {
          await app.execute(principal, change.operation)
        } else {
          const tx = await owner.engine.newTransaction(true, signal)
          const unit = new ApplicationTransaction(tx, {
            ...definition,
            instance: 'unrelated-app',
            limits: app.limits,
            write: true,
            signal,
            authorize: async () => {},
          })
          try {
            await unit
              .collections(principal)
              .collection('colors')
              .put('other', value)
            await unit.flush()
            await tx.commit(signal)
          } finally {
            await unit.release()
            await tx.discard()
            tx.release()
          }
        }
        const accepted = (await owner.engine.getSeqno(signal)).seqno ?? 0n
        const acceptedMs = performance.now() - start
        while (queries.diagnostics().processedSeqno < accepted) {
          signal.throwIfAborted()
          assert.ok(
            performance.now() - start < 20_000,
            'query did not process the accepted revision',
          )
          await setImmediate(undefined, { signal })
        }
        const after = queries.diagnostics()
        assert.equal(
          after.evaluations - before.evaluations,
          change.affected ? 1 : 0,
        )
        assert.equal(
          after.rangeScans - before.rangeScans,
          change.affected ? 1 : 0,
        )
        assert.equal(after.keyReads - before.keyReads, change.reads)
        let deliveredBytes = 0
        if (change.affected) {
          const results = await Promise.all(
            iterators.map((iterator) => iterator.next()),
          )
          current = results[0].value.value
          for (const result of results) {
            assert.equal(result.value.status, 'current')
            assert.deepEqual(result.value.value, current)
            deliveredBytes += Buffer.byteLength(
              JSON.stringify(result.value.value),
            )
          }
        }
        const settledMs = performance.now() - start

        // Full reevaluation uses one snapshot and the identical query callback.
        const baselineStart = performance.now()
        const tx = await owner.engine.newTransaction(false, signal)
        const unit = new ApplicationTransaction(tx, {
          ...definition,
          instance: definition.schema.id,
          limits: app.limits,
          write: false,
          signal,
          authorize: async () => {},
        })
        let baselineRecords = 0
        try {
          const source = unit.collections(principal)
          const result = await ranking.evaluate(
            {
              collection: (name) => {
                const original = source.collection(name)
                return {
                  get: (key) => original.get(key),
                  scan: async (query) => {
                    const entries = await original.scan(query)
                    baselineRecords += entries.length
                    return entries
                  },
                }
              },
            },
            null,
          )
          assert.deepEqual(current, result)
        } finally {
          await unit.release()
          await tx.discard()
          tx.release()
        }
        measurements.push({
          change: change.label,
          acceptedMs,
          settledMs,
          fullReevaluationMs: performance.now() - baselineStart,
          fullReevaluationRecords: baselineRecords,
          evaluations: after.evaluations - before.evaluations,
          scans: after.rangeScans - before.rangeScans,
          reads: after.keyReads - before.keyReads,
          deliveredBytes,
        })
      }
      console.log(
        JSON.stringify({
          node: process.version,
          size,
          subscribers: iterators.length,
          seedMs,
          fullMs,
          measurements,
        }),
      )
    } finally {
      controller.abort()
      await Promise.all(iterators.map((iterator) => iterator.return?.()))
      await queries.close()
      await app.close()
      await owner.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)
