import { describe, expect, it, vi } from 'vitest'

import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { BlockCursor } from '../block/cursor/cursor.js'
import { BlockTransaction } from '../block/transaction/transaction.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import { accessObject } from './utils.js'

interface Deferred<T> {
  promise: Promise<T>
  resolve(value: T): void
  reject(error: unknown): void
}

type LifecycleEvent =
  | 'getRef:start'
  | 'getRef:done'
  | 'buildTransaction'
  | 'markDirty'
  | 'callback'
  | 'write'
  | `release:${number}`

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

function createResourceRef(
  resourceId: number,
  events: LifecycleEvent[],
): ClientResourceRef {
  let released = false
  const ref: ClientResourceRef = {
    resourceId,
    get released() {
      return released
    },
    client: {
      request() {
        throw new Error('unexpected RPC request from unit-test resource')
      },
    } as unknown as ClientResourceRef['client'],
    createRef(id) {
      return createResourceRef(id, events)
    },
    createResource(id, ResourceClass, ...args) {
      return new ResourceClass(createResourceRef(id, events), ...args)
    },
    release() {
      if (released) return
      released = true
      events.push(`release:${resourceId}`)
    },
    [Symbol.dispose]() {
      this.release()
    },
  }
  return ref
}

function blockRef(bytes: number[]) {
  return {
    hash: {
      hashType: 1,
      hash: new Uint8Array(bytes),
    },
  }
}

describe('accessObject', () => {
  it('sequences ref lookup before writes and releases transaction resources on success', async () => {
    const events: LifecycleEvent[] = []
    const refLookup = createDeferred<{ ref?: ObjectRef }>()
    const worldState = new BucketLookupCursor(createResourceRef(1, events))
    const transaction = new BlockTransaction(createResourceRef(2, events))
    const cursor = new BlockCursor(createResourceRef(3, events))
    const writtenRootRef = blockRef([9, 9, 9])

    const getRef = vi
      .spyOn(worldState, 'getRef')
      .mockImplementation(async () => {
        events.push('getRef:start')
        const value = await refLookup.promise
        events.push('getRef:done')
        return value
      })
    const buildTransaction = vi
      .spyOn(worldState, 'buildTransaction')
      .mockImplementation(() => {
        events.push('buildTransaction')
        return Promise.resolve({ transaction, cursor })
      })
    const write = vi.spyOn(transaction, 'write').mockImplementation(() => {
      events.push('write')
      return Promise.resolve({ rootRef: writtenRootRef })
    })
    const callback = vi.fn((actualCursor: BlockCursor) => {
      events.push('callback')
      expect(actualCursor).toBe(cursor)
      return Promise.resolve()
    })

    const resultPromise = accessObject(
      worldState,
      { bucketId: 'input-bucket', rootRef: blockRef([1, 2, 3]) },
      callback,
    )

    await Promise.resolve()

    expect(getRef).toHaveBeenCalledOnce()
    expect(buildTransaction).not.toHaveBeenCalled()
    expect(write).not.toHaveBeenCalled()
    expect(events).toEqual(['getRef:start'])

    refLookup.resolve({
      ref: { bucketId: 'current-bucket', rootRef: blockRef([4, 5, 6]) },
    })

    await expect(resultPromise).resolves.toEqual({
      bucketId: 'current-bucket',
      rootRef: writtenRootRef,
    })

    expect(events).toEqual([
      'getRef:start',
      'getRef:done',
      'buildTransaction',
      'callback',
      'write',
      'release:3',
      'release:2',
    ])
    expect(callback).toHaveBeenCalledOnce()
    expect(write).toHaveBeenCalledWith({ clearTree: true }, undefined)
    expect(cursor.released).toBe(true)
    expect(transaction.released).toBe(true)
  })
  it('creates an undefined ref without looking up an existing bucket and releases resources after write', async () => {
    const events: LifecycleEvent[] = []
    const worldState = new BucketLookupCursor(createResourceRef(1, events))
    const transaction = new BlockTransaction(createResourceRef(2, events))
    const cursor = new BlockCursor(createResourceRef(3, events))
    const writtenRootRef = blockRef([7, 8, 9])

    const getRef = vi.spyOn(worldState, 'getRef').mockImplementation(() => {
      throw new Error('new object path must not look up an existing ref')
    })
    const buildTransaction = vi
      .spyOn(worldState, 'buildTransaction')
      .mockImplementation(() => {
        events.push('buildTransaction')
        return Promise.resolve({ transaction, cursor })
      })
    const markDirty = vi.spyOn(cursor, 'markDirty').mockImplementation(() => {
      events.push('markDirty')
      return Promise.resolve()
    })
    const write = vi.spyOn(transaction, 'write').mockImplementation(() => {
      events.push('write')
      return Promise.resolve({ rootRef: writtenRootRef })
    })
    const callback = vi.fn((actualCursor: BlockCursor) => {
      events.push('callback')
      expect(actualCursor).toBe(cursor)
      return Promise.resolve()
    })

    await expect(
      accessObject(worldState, undefined, callback),
    ).resolves.toEqual({
      bucketId: '',
      rootRef: writtenRootRef,
    })

    expect(getRef).not.toHaveBeenCalled()
    expect(buildTransaction).toHaveBeenCalledWith({}, undefined)
    expect(markDirty).toHaveBeenCalledWith(undefined)
    expect(callback).toHaveBeenCalledOnce()
    expect(write).toHaveBeenCalledWith({ clearTree: true }, undefined)
    expect(events).toEqual([
      'buildTransaction',
      'markDirty',
      'callback',
      'write',
      'release:3',
      'release:2',
    ])
    expect(cursor.released).toBe(true)
    expect(transaction.released).toBe(true)
  })
})
