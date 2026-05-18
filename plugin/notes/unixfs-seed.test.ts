import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MknodType } from '@s4wave/sdk/unixfs/handle.pb.js'

type MockFn = ReturnType<typeof vi.fn>

interface MockHandle {
  mkdirAll: MockFn
  mknod: MockFn
  writeAt: MockFn
  release: MockFn
  lookupPath: MockFn
  lookup: MockFn
}

const h = vi.hoisted(() => ({
  handles: [] as MockHandle[],
}))

vi.mock('@s4wave/sdk/unixfs/handle.js', () => {
  class MockFSHandle implements MockHandle {
    mkdirAll = vi.fn().mockResolvedValue(undefined)
    mknod = vi.fn().mockResolvedValue(undefined)
    writeAt = vi.fn().mockResolvedValue(0n)
    release = vi.fn()

    constructor(readonly ref: unknown) {
      h.handles.push(this)
    }

    lookupPath = vi.fn((path: string) =>
      Promise.resolve({
        handle: new MockFSHandle({ path }),
        traversedPath: path.split('/'),
      }),
    )

    lookup = vi.fn((name: string) =>
      Promise.resolve(new MockFSHandle({ name })),
    )
  }

  Object.defineProperty(MockFSHandle.prototype, Symbol.dispose, {
    configurable: true,
    value(this: MockFSHandle) {
      this.release()
    },
  })

  return { FSHandle: MockFSHandle }
})

import { uploadSeedTree } from './unixfs-seed.js'

describe('uploadSeedTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.handles.length = 0
  })

  function buildWorldState() {
    const createRef = vi.fn().mockReturnValue({ resource: 'fs-root' })
    return {
      accessTypedObject: vi.fn().mockResolvedValue({ resourceId: 7 }),
      getResourceRef: () => ({
        createRef,
      }),
    }
  }

  it('creates directories and writes text files with unary filesystem calls', async () => {
    await uploadSeedTree(
      buildWorldState() as never,
      'docs-fs',
      [{ path: 'nested/index.md', content: 'hello' }],
      ['nested'],
    )

    const [root, parent, file] = h.handles
    expect(root.mkdirAll).toHaveBeenCalledWith(['nested'], 0o755, undefined)
    expect(root.lookupPath).toHaveBeenCalledWith('nested', undefined)
    expect(parent.mknod).toHaveBeenCalledWith(
      ['index.md'],
      MknodType.FILE,
      0o644,
      false,
      undefined,
    )
    expect(parent.lookup).toHaveBeenCalledWith('index.md', undefined)
    expect(file.writeAt).toHaveBeenCalledWith(
      0n,
      new TextEncoder().encode('hello'),
      undefined,
    )
    expect(file.release).toHaveBeenCalledTimes(1)
    expect(parent.release).toHaveBeenCalledTimes(1)
    expect(root.release).toHaveBeenCalledTimes(1)
  })

  it('rejects empty or parent-relative seed paths before mutating the filesystem', async () => {
    const worldState = buildWorldState()

    await expect(
      uploadSeedTree(worldState as never, 'blog-fs', [
        { path: '../hello.md', content: 'hello' },
      ]),
    ).rejects.toThrow('invalid seed path: ../hello.md')
    expect(h.handles[0]?.mknod).not.toHaveBeenCalled()
  })
})
