import { describe, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
  mockUploadTree: vi.fn(),
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
  it('uploads directories and text files as one tree batch', async () => {
    h.mockUploadTree.mockResolvedValue({
      bytesWritten: 5n,
      filesWritten: 1n,
      directoriesWritten: 1n,
    })

    const worldState = {
      accessTypedObject: vi.fn().mockResolvedValue({ resourceId: 7 }),
      getResourceRef: () => ({
        createRef: vi.fn().mockReturnValue({}),
      }),
    }

    await uploadSeedTree(
      worldState as never,
      'docs-fs',
      [{ path: 'nested/index.md', content: 'hello' }],
      ['nested'],
    )

    expect(h.mockUploadTree).toHaveBeenCalledTimes(1)
    const [entries] = h.mockUploadTree.mock.calls[0]
    expect(entries).toHaveLength(2)
    expect(entries[0]).toMatchObject({ kind: 'directory', path: 'nested' })
    expect(entries[1]).toMatchObject({
      kind: 'file',
      path: 'nested/index.md',
      totalSize: 5n,
    })
  })
})
