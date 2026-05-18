import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { TreeUploadEntry } from '@s4wave/sdk/unixfs/handle.js'

const h = vi.hoisted(() => ({
  mockUploadTree: vi.fn<
    (
      entries: TreeUploadEntry[],
      options?: unknown,
      abortSignal?: AbortSignal,
    ) => Promise<{
      bytesWritten: bigint
      filesWritten: bigint
      directoriesWritten: bigint
    }>
  >(),
}))

vi.mock('@s4wave/sdk/unixfs/handle.js', () => {
  class MockFSHandle {
    constructor(_ref: unknown) {}

    uploadTree = h.mockUploadTree
  }

  Object.defineProperty(MockFSHandle.prototype, Symbol.dispose, {
    configurable: true,
    value: () => undefined,
  })

  return { FSHandle: MockFSHandle }
})

import { uploadSeedTree } from './unixfs-seed.js'

describe('uploadSeedTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  function buildWorldState() {
    return {
      accessTypedObject: vi.fn().mockResolvedValue({ resourceId: 7 }),
      getResourceRef: () => ({
        createRef: vi.fn().mockReturnValue({}),
      }),
    }
  }

  it('uploads directories and text files as one tree batch', async () => {
    h.mockUploadTree.mockResolvedValue({
      bytesWritten: 5n,
      filesWritten: 1n,
      directoriesWritten: 1n,
    })

    await uploadSeedTree(
      buildWorldState() as never,
      'docs-fs',
      [{ path: 'nested/index.md', content: 'hello' }],
      ['nested'],
    )

    expect(h.mockUploadTree).toHaveBeenCalledTimes(1)
    const entries = h.mockUploadTree.mock.calls[0][0]
    expect(entries).toHaveLength(2)
    expect(entries[0]).toMatchObject({ kind: 'directory', path: 'nested' })
    expect(entries[1]).toMatchObject({
      kind: 'file',
      path: 'nested/index.md',
      totalSize: 5n,
    })
  })

  it('builds file streams without requiring Blob', async () => {
    h.mockUploadTree.mockResolvedValue({
      bytesWritten: 5n,
      filesWritten: 1n,
      directoriesWritten: 0n,
    })
    const originalBlob = Object.getOwnPropertyDescriptor(globalThis, 'Blob')
    Object.defineProperty(globalThis, 'Blob', {
      configurable: true,
      value: undefined,
    })

    try {
      await uploadSeedTree(
        buildWorldState() as never,
        'blog-fs',
        [{ path: 'hello.md', content: 'hello' }],
      )
    } finally {
      if (originalBlob) {
        Object.defineProperty(globalThis, 'Blob', originalBlob)
      } else {
        delete (globalThis as { Blob?: unknown }).Blob
      }
    }

    const entry = h.mockUploadTree.mock.calls[0][0][0]
    expect(entry.kind).toBe('file')
    if (entry.kind !== 'file') {
      throw new Error('expected file entry')
    }

    const reader = entry.stream.getReader()
    const [first, second] = await Promise.all([reader.read(), reader.read()])
    reader.releaseLock()

    expect(first.done).toBe(false)
    expect(first.value).toEqual(new TextEncoder().encode('hello'))
    expect(second.done).toBe(true)
  })
})
