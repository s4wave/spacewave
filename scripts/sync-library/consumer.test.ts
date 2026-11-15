import assert from 'node:assert/strict'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { Worker } from 'node:worker_threads'
import { spawnSync } from 'node:child_process'
import { defineSchema, SyncError } from 'spacewave'
import { createServer } from 'spacewave/server'
import type { StandardSchemaV1 } from '@standard-schema/spec'

const text: StandardSchemaV1<string> = {
  '~standard': {
    version: 1,
    vendor: 'acceptance',
    validate: (value) =>
      typeof value === 'string'
        ? { value }
        : { issues: [{ message: 'Expected text' }] },
  },
}
const schema = defineSchema({
  id: 'packed-process',
  version: 1,
  collections: { notes: text },
  mutations: {},
})
const principal = { subject: 'alice', scope: 'team' }
const open = (directory: string) =>
  createServer({
    directory,
    schema,
    mutations: {},
    authenticate: async () => principal,
    authorize: () => true,
  })

test(
  'installed package durably reopens without Go or install scripts and excludes another writer',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-packed-'))
    assert.equal(spawnSync('go', ['version']).error?.code, 'ENOENT')
    try {
      const first = await open(directory)
      try {
        await first
          .as(principal)
          .collection('notes')
          .put('first', 'ship the RC', { requestId: 'first' })
        await assert.rejects(
          open(directory),
          (error: unknown) =>
            error instanceof SyncError && error.code === 'STORAGE',
        )
      } finally {
        await first.close()
      }
      const reopened = await open(directory)
      try {
        assert.equal(
          await reopened.as(principal).collection('notes').get('first'),
          'ship the RC',
        )
        await reopened
          .as(principal)
          .collection('notes')
          .put('first', 'ship the RC', { requestId: 'first' })
        await assert.rejects(
          reopened
            .as(principal)
            .collection('notes')
            .put('first', 'different', { requestId: 'first' }),
          { code: 'CONFLICT' },
        )
      } finally {
        await reopened.close()
        await reopened.close()
      }
    } finally {
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'installed servers isolate independent directories',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-isolated-'))
    const servers: Awaited<ReturnType<typeof open>>[] = []
    try {
      servers.push(await open(join(directory, 'a')))
      servers.push(await open(join(directory, 'b')))
      await Promise.all(
        servers.map((server, index) =>
          server.as(principal).collection('notes').put('same', String(index)),
        ),
      )
      assert.deepEqual(
        await Promise.all(
          servers.map((server) =>
            server.as(principal).collection('notes').get('same'),
          ),
        ),
        ['0', '1'],
      )
    } finally {
      await Promise.all(servers.map((server) => server.close()))
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'Worker failure rejects pending work and preserves an acknowledged write',
  { timeout: 60_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-worker-loss-'))
    let worker: Worker | undefined
    const originalPost = Worker.prototype.postMessage
    Worker.prototype.postMessage = function (...args) {
      // oxlint-disable-next-line no-this-alias -- the test captures the Worker the server posts to.
      worker = this
      return originalPost.apply(this, args)
    }
    let server: Awaited<ReturnType<typeof open>>
    try {
      server = await open(directory)
    } finally {
      Worker.prototype.postMessage = originalPost
    }
    try {
      assert.ok(worker)
      await server
        .as(principal)
        .collection('notes')
        .put('first', 'accepted before failure')
      const pending = assert.rejects(
        server.as(principal).collection('notes').get('first'),
      )
      await worker.terminate()
      await pending
      await assert.rejects(server.close(), /Worker exited unexpectedly/)
      const reopened = await open(directory)
      try {
        assert.equal(
          await reopened.as(principal).collection('notes').get('first'),
          'accepted before failure',
        )
      } finally {
        await reopened.close()
      }
    } finally {
      await server.close().catch(() => {})
      await rm(directory, { recursive: true, force: true })
    }
  },
)

test(
  'failed startup releases its Worker and returns a safe error',
  { timeout: 15_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'spacewave-startup-'))
    try {
      const file = join(directory, 'file')
      await writeFile(file, 'not a directory')
      await assert.rejects(
        open(file),
        (error: unknown) =>
          error instanceof SyncError &&
          error.code === 'STORAGE' &&
          !error.message.includes(directory),
      )
    } finally {
      await rm(directory, { recursive: true, force: true })
    }
  },
)
