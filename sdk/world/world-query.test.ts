import { describe, expect, it, vi } from 'vitest'

import type { Engine } from './engine.js'
import { watchWorldQuery } from './world-query.js'
import type { IWorldState } from './world-state.js'

// fakeWorld is a live World whose seqno advances when commit is called.
function fakeWorld() {
  let seqno = 1n
  let wake: (() => void) | undefined
  const txs: Array<{
    discard: ReturnType<typeof vi.fn>
    release: ReturnType<typeof vi.fn>
  }> = []
  const waits: bigint[] = []
  const engine = {
    newTransaction: vi.fn(async () => {
      const pinned = seqno
      const tx = {
        getSeqno: vi.fn(async () => ({ seqno: pinned })),
        discard: vi.fn(async () => {}),
        release: vi.fn(),
      }
      txs.push(tx)
      return tx
    }),
    waitSeqno: vi.fn(async (target: bigint, signal?: AbortSignal) => {
      waits.push(target)
      while (seqno < target) {
        await new Promise<void>((resolve, reject) => {
          wake = resolve
          signal?.addEventListener('abort', () => reject(signal.reason), {
            once: true,
          })
        })
      }
      return { seqno }
    }),
    [Symbol.dispose]: vi.fn(),
  }
  const owner = {
    id: 7,
    resourceRef: { createRef: vi.fn(() => engine) },
  }
  return {
    engine,
    owner: owner as unknown as Engine,
    txs,
    waits,
    commit() {
      seqno++
      wake?.()
    },
  }
}

vi.mock('./engine.js', () => ({
  // The watch wraps its own reference to the owner's Engine.
  Engine: function (ref: unknown) {
    return ref
  },
}))

describe('watchWorldQuery', () => {
  it('re-runs the query on each World revision', async () => {
    const world = fakeWorld()
    const query = vi.fn(
      async (_tx: IWorldState) => `run-${query.mock.calls.length}`,
    )
    const controller = new AbortController()
    const it = watchWorldQuery(world.owner, query, controller.signal)[
      Symbol.asyncIterator
    ]()

    await expect(it.next()).resolves.toEqual({
      done: false,
      value: { seqno: 1n, value: 'run-1' },
    })

    // The next run waits for the revision after the one it read.
    const next = it.next()
    await vi.waitFor(() => expect(world.waits).toEqual([2n]))
    world.commit()
    await expect(next).resolves.toEqual({
      done: false,
      value: { seqno: 2n, value: 'run-2' },
    })

    // Every run's transaction is discarded and released.
    expect(world.txs).toHaveLength(2)
    for (const tx of world.txs) {
      expect(tx.discard).toHaveBeenCalledOnce()
      expect(tx.release).toHaveBeenCalledOnce()
    }

    // Aborting ends the wait and releases the Engine reference.
    const pending = it.next()
    await vi.waitFor(() => expect(world.waits).toEqual([2n, 3n]))
    controller.abort()
    await expect(pending).rejects.toBeDefined()
    expect(world.engine[Symbol.dispose]).toHaveBeenCalledOnce()
  })

  it('releases the transaction when the query fails', async () => {
    const world = fakeWorld()
    const failure = new Error('query failed')
    const it = watchWorldQuery(world.owner, async () => {
      throw failure
    })[Symbol.asyncIterator]()

    await expect(it.next()).rejects.toBe(failure)
    expect(world.txs[0].discard).toHaveBeenCalledOnce()
    expect(world.txs[0].release).toHaveBeenCalledOnce()
    expect(world.engine[Symbol.dispose]).toHaveBeenCalledOnce()
  })
})
