import { describe, expect, it, vi } from 'vitest'

import { Client } from 'starpc'

describe('patched starpc client stream backpressure', () => {
  it('does not drain upload sources while the outbound stream is blocked', async () => {
    const abort = new AbortController()
    const yielded = { count: 0 }
    const firstYield = deferred()
    const secondYield = deferred()
    const sinkConsumed = { count: 0 }
    const sinkGate = deferred()
    const client = new Client(async () => ({
      close: vi.fn(async () => {}),
      abort: vi.fn(),
      source: pendingSource(),
      sink: async (source) => {
        await sinkGate.promise
        for await (const _packet of source) {
          sinkConsumed.count++
        }
      },
    }))

    const request = client
      .clientStreamingRequest(
        'test.Service',
        'Upload',
        chunkSource(yielded, firstYield, secondYield),
        abort.signal,
      )
      .catch((err: unknown) => err)

    await promiseWithTimeout(firstYield.promise, 'first upload chunk')
    expect(yielded.count).toBe(1)
    await expectPending(secondYield.promise, 'second upload chunk')
    expect(yielded.count).toBe(1)
    expect(sinkConsumed.count).toBe(0)
    await expectPending(request, 'client-stream request')

    abort.abort()
    sinkGate.resolve()
    await expect(
      promiseWithTimeout(request, 'client-stream abort'),
    ).resolves.toBeInstanceOf(Error)
  })
})

async function* chunkSource(
  yielded: { count: number },
  firstYield: Deferred,
  secondYield: Deferred,
) {
  for (const i of Array.from({ length: 32 }, (_, index) => index)) {
    yielded.count++
    if (i === 0) {
      firstYield.resolve()
    } else if (i === 1) {
      secondYield.resolve()
    }
    yield new Uint8Array([i])
  }
}

async function* pendingSource() {
  await new Promise(() => {})
  yield new Uint8Array()
}

function promiseWithTimeout<T>(promise: Promise<T>, label: string): Promise<T> {
  return Promise.race([
    promise,
    new Promise<T>((_, reject) => {
      setTimeout(() => reject(new Error(`timed out waiting for ${label}`)), 500)
    }),
  ])
}

async function expectPending<T>(promise: Promise<T>, label: string) {
  await expect(
    Promise.race([
      promise.then(() => 'settled'),
      new Promise<'pending'>((resolve) => {
        setTimeout(() => resolve('pending'), 50)
      }),
    ]),
    `${label} should not be requested while outbound packets are blocked`,
  ).resolves.toBe('pending')
}

type Deferred = ReturnType<typeof deferred>

function deferred(): {
  promise: Promise<void>
  resolve: () => void
} {
  const callbacks: {
    resolve?: () => void
  } = {}
  const promise = new Promise<void>((resolve) => {
    callbacks.resolve = resolve
  })
  return {
    promise,
    resolve() {
      callbacks.resolve?.()
    },
  }
}
