import { mkdtemp, readdir, stat, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { performance } from 'node:perf_hooks'
import { connect, defineSchema } from 'spacewave'
import { createServer } from 'spacewave/server'
import type { StandardSchemaV1 } from '@standard-schema/spec'

const recordCount = 1000
const subscriberCount = 3
const writeCount = 10
const number: StandardSchemaV1<number> = {
  '~standard': {
    version: 1,
    vendor: 'workload',
    validate: (value) =>
      typeof value === 'number'
        ? { value }
        : { issues: [{ message: 'Expected a number' }] },
  },
}
const schema = defineSchema({
  id: 'workload',
  version: 1,
  collections: { values: number },
  mutations: {
    seed: { input: number, output: number },
    receiptProbe: { input: number, output: number },
  },
})
const directory = await mkdtemp(join(tmpdir(), 'sync-workload-'))
const before = process.memoryUsage().rss
const started = performance.now()
const server = await createServer({
  directory,
  schema,
  authenticate: async () => ({ subject: 'bench', scope: 'bench' }),
  authorize: () => true,
  mutations: {
    seed: async (context, count) => {
      for (let index = 0; index < count; index++)
        await context
          .collection('values')
          .put(String(index).padStart(6, '0'), index)
      return count
    },
    receiptProbe: async (_context, value) => value,
  },
})
const startupMs = performance.now() - started
const rssReady = process.memoryUsage().rss
const clients: Awaited<ReturnType<typeof connect<typeof schema>>>[] = []
const stops: (() => void)[] = []
const seen = Array.from({ length: subscriberCount }, () => -1)
async function wait(check: () => boolean): Promise<void> {
  const deadline = Date.now() + 60_000
  while (!check()) {
    if (Date.now() > deadline) throw new Error('Workload delivery timed out')
    await new Promise((resolve) => setTimeout(resolve, 5))
  }
}
async function storageBytes(path = directory): Promise<number> {
  const sizes = await Promise.all(
    (await readdir(path, { recursive: true })).map(async (file) => {
      const info = await stat(join(path, file))
      return info.isFile() ? info.size : 0
    }),
  )
  return sizes.reduce((sum, size) => sum + size, 0)
}
const quantiles = (values: number[]) => {
  const sorted = [...values].sort((a, b) => a - b)
  return {
    p50: sorted[Math.ceil(sorted.length * 0.5) - 1],
    p95: sorted[Math.ceil(sorted.length * 0.95) - 1],
    max: sorted.at(-1),
  }
}
try {
  const seedStarted = performance.now()
  await server.admin('bench').mutate('seed', recordCount)
  const seedMs = performance.now() - seedStarted
  const bytesAfterSeed = await storageBytes()
  const listener = await server.listen({ port: 0 })
  const initial = performance.now()
  for (let index = 0; index < subscriberCount; index++) {
    const client = await connect({
      url: listener.url,
      schema,
      getAccessToken: () => 'bench',
    })
    clients.push(client)
    stops.push(
      client.collection('values').subscribe(
        {},
        {
          next: (state) => {
            if (state.status === 'current')
              seen[index] =
                state.data.find((entry) => entry.key === '000000')?.value ?? -1
            if (state.status === 'error') throw state.error
          },
        },
      ),
    )
  }
  await wait(() => seen.every((value) => value === 0))
  const firstSnapshotMs = performance.now() - initial
  const acceptanceMs: number[] = []
  const deliveryMs: number[] = []
  for (let index = 1; index <= writeCount; index++) {
    const start = performance.now()
    await clients[0]
      .collection('values')
      .put('000000', recordCount + index, { requestId: `measure-${index}` })
    acceptanceMs.push(performance.now() - start)
    await wait(() => seen.every((value) => value === recordCount + index))
    deliveryMs.push(performance.now() - start)
  }
  const rssLoaded = process.memoryUsage().rss
  const bytesAfterWrites = await storageBytes()
  const receiptOnlyCount = 10
  const receiptDirectory = join(directory, 'receipt-only')
  const receiptServer = await createServer({
    directory: receiptDirectory,
    schema,
    authenticate: async () => ({ subject: 'bench', scope: 'bench' }),
    authorize: () => true,
    mutations: {
      seed: async (_context, count) => count,
      receiptProbe: async (_context, value) => value,
    },
  })
  let receiptOnlyBytesBefore: number
  let receiptOnlyBytesAfter: number
  try {
    receiptOnlyBytesBefore = await storageBytes(receiptDirectory)
    for (let index = 0; index < receiptOnlyCount; index++)
      await receiptServer.admin('bench').mutate('receiptProbe', index)
    receiptOnlyBytesAfter = await storageBytes(receiptDirectory)
  } finally {
    await receiptServer.close()
  }
  console.log(
    JSON.stringify({
      node: process.version,
      platform: process.platform,
      arch: process.arch,
      recordCount,
      subscriberCount,
      writeCount,
      retainedReceipts: writeCount + 1,
      startupMs,
      seedMs,
      firstSnapshotMs,
      acceptanceMs: quantiles(acceptanceMs),
      deliveryMs: quantiles(deliveryMs),
      rssBefore: before,
      rssReady,
      rssLoaded,
      bytesAfterSeed,
      bytesAfterWrites,
      receiptOnlyCount,
      receiptOnlyBytesBefore,
      receiptOnlyBytesAfter,
      receiptOnlyStorageGrowth: receiptOnlyBytesAfter - receiptOnlyBytesBefore,
      snapshotJSONBytes: Buffer.byteLength(
        JSON.stringify(await clients[0].collection('values').scan()),
      ),
    }),
  )
} finally {
  for (const stop of stops) stop()
  await Promise.all(clients.map((client) => client.close()))
  await server.close()
  await rm(directory, { recursive: true, force: true })
}
