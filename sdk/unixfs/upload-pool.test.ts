import { describe, expect, it, vi } from 'vitest'

import type { IFSHandle, TreeUploadEntry } from './handle.js'
import { TreeUploadPool } from './upload-pool.js'

interface DeferredUpload {
  path: string
  resolve: () => void
  reject: (err: Error) => void
}

function fileEntry(path: string): TreeUploadEntry {
  return {
    kind: 'file',
    path,
    totalSize: 1n,
    stream: new ReadableStream<Uint8Array>(),
  }
}

function deferredHandle(
  active: Set<string>,
  maxActive: { value: number },
  uploads: DeferredUpload[],
  listing?: Set<string>,
): Pick<IFSHandle, 'uploadTree'> {
  return {
    uploadTree: vi.fn(async ([entry]) => {
      active.add(entry.path)
      maxActive.value = Math.max(maxActive.value, active.size)
      const deferred = Promise.withResolvers<void>()
      uploads.push({
        path: entry.path,
        resolve: () => {
          active.delete(entry.path)
          listing?.add(entry.path)
          deferred.resolve()
        },
        reject: (err) => {
          active.delete(entry.path)
          deferred.reject(err)
        },
      })
      await deferred.promise
      return {
        bytesWritten: entry.kind === 'file' ? entry.totalSize : 0n,
        filesWritten: entry.kind === 'file' ? 1n : 0n,
        directoriesWritten: entry.kind === 'directory' ? 1n : 0n,
      }
    }),
  }
}

async function nextTurn(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>()
  queueMicrotask(resolve)
  await promise
}

describe('TreeUploadPool', () => {
  it('limits in-flight uploads and starts queued files as slots open', async () => {
    const active = new Set<string>()
    const maxActive = { value: 0 }
    const uploads: DeferredUpload[] = []
    const handle = deferredHandle(active, maxActive, uploads)
    const pool = new TreeUploadPool(3)

    for (const name of ['a', 'b', 'c', 'd', 'e']) {
      pool.add(handle, fileEntry(name), {})
    }

    expect(active).toEqual(new Set(['a', 'b', 'c']))
    expect(maxActive.value).toBe(3)

    uploads.find((upload) => upload.path === 'a')?.resolve()
    await nextTurn()

    expect(active).toEqual(new Set(['b', 'c', 'd']))
    expect(maxActive.value).toBe(3)
  })

  it('publishes each completed file while later files remain in flight', async () => {
    const active = new Set<string>()
    const maxActive = { value: 0 }
    const uploads: DeferredUpload[] = []
    const listing = new Set<string>()
    const handle = deferredHandle(active, maxActive, uploads, listing)
    const pool = new TreeUploadPool(2)

    for (const name of ['a', 'b', 'c']) {
      pool.add(handle, fileEntry(name), {})
    }

    uploads.find((upload) => upload.path === 'a')?.resolve()
    await nextTurn()

    expect(listing).toEqual(new Set(['a']))
    expect(active).toEqual(new Set(['b', 'c']))
  })

  it('continues queued files after one upload fails', async () => {
    const active = new Set<string>()
    const maxActive = { value: 0 }
    const uploads: DeferredUpload[] = []
    const completed: string[] = []
    const failed: string[] = []
    const handle = deferredHandle(active, maxActive, uploads)
    const pool = new TreeUploadPool(1)

    for (const name of ['a', 'b']) {
      pool.add(handle, fileEntry(name), {
        onComplete: () => completed.push(name),
        onError: () => failed.push(name),
      })
    }

    uploads.find((upload) => upload.path === 'a')?.reject(new Error('broken'))
    await nextTurn()
    expect(active).toEqual(new Set(['b']))

    uploads.find((upload) => upload.path === 'b')?.resolve()
    await nextTurn()

    expect(failed).toEqual(['a'])
    expect(completed).toEqual(['b'])
  })
})
