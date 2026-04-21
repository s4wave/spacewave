import { describe, it, expect, vi } from 'vitest'
import { FSHandle, FsInode } from './fs-handle.js'
import type {
  FSCursor,
  FSCursorChangeCb,
  FSCursorOps,
} from './fs-cursor.js'

// MockFSCursorOps implements FSCursorOps backed by in-memory state.
class MockFSCursorOps implements FSCursorOps {
  released = false
  private isDir: boolean
  private isFileVal: boolean
  private isSymlinkVal: boolean
  private nameVal: string
  private sizeVal: bigint
  private permissionsVal: number
  private modTimeVal: Date
  private readDataVal: Uint8Array

  constructor(opts?: {
    name?: string
    isDir?: boolean
    isFile?: boolean
    isSymlink?: boolean
    size?: bigint
    permissions?: number
    modTime?: Date
    readData?: Uint8Array
  }) {
    this.nameVal = opts?.name ?? 'test'
    this.isDir = opts?.isDir ?? false
    this.isFileVal = opts?.isFile ?? true
    this.isSymlinkVal = opts?.isSymlink ?? false
    this.sizeVal = opts?.size ?? 100n
    this.permissionsVal = opts?.permissions ?? 0o644
    this.modTimeVal = opts?.modTime ?? new Date(1000)
    this.readDataVal = opts?.readData ?? new Uint8Array([1, 2, 3, 4])
  }

  checkReleased(): boolean {
    return this.released
  }
  getName(): string {
    return this.nameVal
  }
  getIsDirectory(): boolean {
    return this.isDir
  }
  getIsFile(): boolean {
    return this.isFileVal
  }
  getIsSymlink(): boolean {
    return this.isSymlinkVal
  }
  async getPermissions(): Promise<number> {
    return this.permissionsVal
  }
  async setPermissions(): Promise<void> {}
  async getSize(): Promise<bigint> {
    return this.sizeVal
  }
  async getModTimestamp(): Promise<Date> {
    return this.modTimeVal
  }
  async setModTimestamp(): Promise<void> {}
  async readAt(
    _offset: bigint,
    _size: bigint,
  ): Promise<{ data: Uint8Array; n: bigint }> {
    return { data: this.readDataVal, n: BigInt(this.readDataVal.length) }
  }
  async readAtTo(
    offset: bigint,
    data: {
      readonly length: number
      set(source: ArrayLike<number>, offset?: number): void
    },
  ): Promise<bigint> {
    const len = Math.min(data.length, this.readDataVal.length)
    data.set(this.readDataVal.subarray(0, len))
    return BigInt(len)
  }
  async getOptimalWriteSize(): Promise<bigint> {
    return 4096n
  }
  async writeAt(): Promise<void> {}
  async truncate(): Promise<void> {}
  async lookup(_name: string): Promise<FSCursor> {
    return new MockFSCursor()
  }
  async readdirAll(): Promise<void> {}
  async mknod(): Promise<void> {}
  async symlink(): Promise<void> {}
  async readlink(): Promise<{ path: string[]; isAbsolute: boolean }> {
    return { path: ['target'], isAbsolute: false }
  }
  async copyTo(): Promise<boolean> {
    return false
  }
  async copyFrom(): Promise<boolean> {
    return false
  }
  async moveTo(): Promise<boolean> {
    return false
  }
  async moveFrom(): Promise<boolean> {
    return false
  }
  async remove(): Promise<void> {}
  async mknodWithContent(): Promise<void> {}
}

// MockFSCursor implements FSCursor backed by a MockFSCursorOps.
class MockFSCursor implements FSCursor {
  released = false
  private ops: MockFSCursorOps
  private proxyCursor: FSCursor | null = null

  constructor(opts?: { ops?: MockFSCursorOps; proxyCursor?: FSCursor | null }) {
    this.ops = opts?.ops ?? new MockFSCursorOps()
    this.proxyCursor = opts?.proxyCursor ?? null
  }

  checkReleased(): boolean {
    return this.released
  }

  async getProxyCursor(): Promise<FSCursor | null> {
    return this.proxyCursor
  }

  addChangeCb(_cb: FSCursorChangeCb): void {}

  async getCursorOps(): Promise<FSCursorOps | null> {
    return this.ops
  }

  release(): void {
    this.released = true
  }

  [Symbol.dispose](): void {
    this.release()
  }
}

describe('FSHandle', () => {
  describe('create', () => {
    it('creates a handle with a mock cursor', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      expect(handle).toBeInstanceOf(FSHandle)
      expect(handle.checkReleased()).toBe(false)
      handle.release()
    })

    it('has empty name for root inode', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      expect(handle.getName()).toBe('')
      handle.release()
    })
  })

  describe('release', () => {
    it('marks handle as released', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      expect(handle.checkReleased()).toBe(false)
      handle.release()
      expect(handle.checkReleased()).toBe(true)
    })

    it('is idempotent', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      handle.release()
      handle.release()
      expect(handle.checkReleased()).toBe(true)
    })

    it('Symbol.dispose calls release', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      handle[Symbol.dispose]()
      expect(handle.checkReleased()).toBe(true)
    })

    it('fires release callbacks', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      const cb = vi.fn()
      handle.addReleaseCallback(cb)
      handle.release()
      expect(cb).toHaveBeenCalledTimes(1)
    })
  })

  describe('clone', () => {
    it('creates a new handle sharing the same inode', async () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      const cloned = await handle.clone()
      expect(cloned).toBeInstanceOf(FSHandle)
      expect(cloned).not.toBe(handle)
      expect(cloned.getName()).toBe(handle.getName())

      // Releasing the original does not release the clone.
      handle.release()
      expect(cloned.checkReleased()).toBe(false)
      cloned.release()
    })
  })

  describe('accessOps', () => {
    it('resolves cursor and ops via accessInode', async () => {
      const ops = new MockFSCursorOps({ name: 'myfile', size: 42n })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      let resolvedOps: FSCursorOps | null = null
      await handle.accessOps(ctrl.signal, async (_c, o) => {
        resolvedOps = o
      })
      expect(resolvedOps).toBe(ops)
      expect(resolvedOps!.getName()).toBe('myfile')

      handle.release()
    })
  })

  describe('getOps', () => {
    it('returns the resolved cursor and ops', async () => {
      const ops = new MockFSCursorOps({
        name: 'root',
        isDir: true,
        isFile: false,
      })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const result = await handle.getOps(ctrl.signal)
      expect(result.ops).toBe(ops)
      expect(result.ops.getIsDirectory()).toBe(true)

      handle.release()
    })
  })

  describe('getSize', () => {
    it('returns size from ops', async () => {
      const ops = new MockFSCursorOps({ size: 999n })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const size = await handle.getSize(ctrl.signal)
      expect(size).toBe(999n)

      handle.release()
    })
  })

  describe('getModTimestamp', () => {
    it('returns mod time from ops', async () => {
      const ts = new Date(2025, 0, 1)
      const ops = new MockFSCursorOps({ modTime: ts })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const modTime = await handle.getModTimestamp(ctrl.signal)
      expect(modTime).toBe(ts)

      handle.release()
    })
  })

  describe('getPermissions', () => {
    it('returns permissions from ops', async () => {
      const ops = new MockFSCursorOps({ permissions: 0o755 })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const perms = await handle.getPermissions(ctrl.signal)
      expect(perms).toBe(0o755)

      handle.release()
    })
  })

  describe('readAt', () => {
    it('delegates to ops readAt for file', async () => {
      const data = new Uint8Array([10, 20, 30, 40])
      const ops = new MockFSCursorOps({ isFile: true, readData: data })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const result = await handle.readAt(ctrl.signal, 0n, 4n)
      expect(result.n).toBe(4n)
      expect(result.data).toEqual(data)

      handle.release()
    })

    it('throws ErrNotFile for directory', async () => {
      const ops = new MockFSCursorOps({ isDir: true, isFile: false })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      await expect(handle.readAt(ctrl.signal, 0n, 4n)).rejects.toThrow(
        'not a file',
      )

      handle.release()
    })
  })

  describe('writeAt', () => {
    it('throws ErrNotFile for directory', async () => {
      const ops = new MockFSCursorOps({ isDir: true, isFile: false })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      await expect(
        handle.writeAt(ctrl.signal, 0n, new Uint8Array([1]), new Date()),
      ).rejects.toThrow('not a file')

      handle.release()
    })
  })

  describe('getNodeType', () => {
    it('returns directory node type', async () => {
      const ops = new MockFSCursorOps({ isDir: true, isFile: false })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const nt = await handle.getNodeType(ctrl.signal)
      expect(nt.getIsDirectory()).toBe(true)
      expect(nt.getIsFile()).toBe(false)
      expect(nt.getIsSymlink()).toBe(false)

      handle.release()
    })

    it('returns file node type', async () => {
      const ops = new MockFSCursorOps({ isFile: true })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const nt = await handle.getNodeType(ctrl.signal)
      expect(nt.getIsFile()).toBe(true)
      expect(nt.getIsDirectory()).toBe(false)

      handle.release()
    })

    it('returns symlink node type', async () => {
      const ops = new MockFSCursorOps({
        isSymlink: true,
        isFile: false,
      })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const nt = await handle.getNodeType(ctrl.signal)
      expect(nt.getIsSymlink()).toBe(true)

      handle.release()
    })
  })

  describe('getFileInfo', () => {
    it('returns file info from ops', async () => {
      const ops = new MockFSCursorOps({
        name: 'readme.md',
        isFile: true,
        size: 512n,
        permissions: 0o644,
        modTime: new Date(2025, 5, 15),
      })
      const cursor = new MockFSCursor({ ops })
      const handle = FSHandle.create(cursor)
      const ctrl = new AbortController()

      const info = await handle.getFileInfo(ctrl.signal)
      expect(info.name).toBe('readme.md')
      expect(info.size).toBe(512n)
      expect(info.isDir).toBe(false)
      expect(info.modTime).toEqual(new Date(2025, 5, 15))
      // Mode should have permission bits set (no type bits for file).
      expect(info.mode & 0o777).toBe(0o644)

      handle.release()
    })
  })

  describe('addReleaseCallback', () => {
    it('fires immediately if already released', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      handle.release()

      const cb = vi.fn()
      handle.addReleaseCallback(cb)
      expect(cb).toHaveBeenCalledTimes(1)
    })

    it('fires on release', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      const cb = vi.fn()
      handle.addReleaseCallback(cb)
      expect(cb).not.toHaveBeenCalled()
      handle.release()
      expect(cb).toHaveBeenCalledTimes(1)
    })

    it('callback is only called once even if release is called twice', () => {
      const cursor = new MockFSCursor()
      const handle = FSHandle.create(cursor)
      const cb = vi.fn()
      handle.addReleaseCallback(cb)
      handle.release()
      handle.release()
      expect(cb).toHaveBeenCalledTimes(1)
    })
  })

  describe('FsInode', () => {
    it('creates inode with parent and name', () => {
      const cursor = new MockFSCursor()
      const parent = new FsInode(null, '', [cursor])
      const child = new FsInode(parent, 'child', [])
      expect(child.parent).toBe(parent)
      expect(child.name).toBe('child')
    })

    it('checkReleasedFlag returns false initially', () => {
      const cursor = new MockFSCursor()
      const inode = new FsInode(null, '', [cursor])
      expect(inode.checkReleasedFlag()).toBe(false)
    })

    it('checkReleasedWithErr returns null when not released', () => {
      const cursor = new MockFSCursor()
      const inode = new FsInode(null, '', [cursor])
      expect(inode.checkReleasedWithErr()).toBeNull()
    })

    it('addReferenceLocked creates an FSHandle', () => {
      const cursor = new MockFSCursor()
      const inode = new FsInode(null, '', [cursor])
      const handle = inode.addReferenceLocked(false)
      expect(handle).toBeInstanceOf(FSHandle)
      expect(handle.checkReleased()).toBe(false)
      handle.release()
    })
  })
})
