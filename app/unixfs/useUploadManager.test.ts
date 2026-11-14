import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type {
  FSHandle,
  TreeUploadEntry,
  TreeUploadExpect,
  TreeUploadResult,
} from '@s4wave/sdk/unixfs/handle.js'
import { useUploadManager, type UploadManager } from './useUploadManager.js'

function buildFile(name: string, content: string, relativePath?: string): File {
  const file = new File([content], name)
  if (relativePath) {
    Object.defineProperty(file, 'webkitRelativePath', {
      configurable: true,
      value: relativePath,
    })
  }
  return file
}

describe('useUploadManager', () => {
  it('enqueues each selected entry as an independent tree upload', async () => {
    const uploadTree = vi.fn<FSHandle['uploadTree']>().mockResolvedValue({
      kind: 'written',
      bytesWritten: 8n,
      filesWritten: 1n,
      directoriesWritten: 0n,
      versions: [],
    })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager())
    act(() => {
      result.current.addFiles(
        handle,
        [
          buildFile('child.txt', 'hello', 'nested/child.txt'),
          buildFile('top.txt', 'top'),
        ],
        ['nested'],
      )
    })

    await waitFor(() => {
      expect(uploadTree).toHaveBeenCalledTimes(3)
    })

    const entries = uploadTree.mock.calls.map(([entry]) => entry[0])
    // Folders are sent at once; files wait for their folder's listing.
    expect(entries).toMatchObject([
      { kind: 'directory', path: 'nested' },
      { kind: 'file', path: 'nested/child.txt', expect: { kind: 'missing' } },
      { kind: 'file', path: 'top.txt', expect: { kind: 'missing' } },
    ])
  })

  it('emits a started event when files are added', () => {
    const pending = Promise.withResolvers<never>()
    const uploadTree = vi
      .fn<FSHandle['uploadTree']>()
      .mockReturnValue(pending.promise)
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager())
    act(() => {
      result.current.addFiles(handle, [buildFile('a.txt', 'hi')])
    })

    expect(result.current.lastEvent).toMatchObject({
      kind: 'started',
      fileCount: 1,
      errorCount: 0,
    })
  })

  it('projects the SDK pool limit as uploading and queued item states', async () => {
    const pending = Promise.withResolvers<never>()
    const uploadTree = vi
      .fn<FSHandle['uploadTree']>()
      .mockReturnValue(pending.promise)
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager())
    act(() => {
      result.current.addFiles(handle, [
        buildFile('a.txt', 'a'),
        buildFile('b.txt', 'b'),
        buildFile('c.txt', 'c'),
        buildFile('d.txt', 'd'),
      ])
    })

    await waitFor(() => {
      expect(uploadTree).toHaveBeenCalledTimes(3)
    })
    expect(result.current.items.map((item) => item.status)).toEqual([
      'uploading',
      'uploading',
      'uploading',
      'queued',
    ])
  })

  it('emits a completed event when all uploads finish', async () => {
    const uploadTree = vi.fn<FSHandle['uploadTree']>().mockResolvedValue({
      kind: 'written',
      bytesWritten: 8n,
      filesWritten: 2n,
      directoriesWritten: 0n,
      versions: [],
    })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager())
    act(() => {
      result.current.addFiles(handle, [
        buildFile('a.txt', 'hi'),
        buildFile('b.txt', 'yo'),
      ])
    })

    await waitFor(() => {
      expect(result.current.lastEvent?.kind).toBe('completed')
    })
    expect(result.current.lastEvent).toMatchObject({
      kind: 'completed',
      fileCount: 2,
      errorCount: 0,
    })
  })

  it('projects one file failure without stopping its siblings', async () => {
    const uploadTree = vi
      .fn<FSHandle['uploadTree']>()
      .mockImplementation(([entry]) => {
        if (entry.path === 'broken.txt') {
          return Promise.reject(new Error('broken file'))
        }
        return Promise.resolve({
          kind: 'written',
          bytesWritten: 2n,
          filesWritten: 1n,
          directoriesWritten: 0n,
          versions: [],
        })
      })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager())
    act(() => {
      result.current.addFiles(handle, [
        buildFile('broken.txt', 'no'),
        buildFile('good.txt', 'ok'),
      ])
    })

    await waitFor(() => {
      expect(
        result.current.items.every(
          (item) => item.status === 'done' || item.status === 'error',
        ),
      ).toBe(true)
    })
    expect(result.current.items).toMatchObject([
      { name: 'broken.txt', status: 'error', error: 'broken file' },
      { name: 'good.txt', status: 'done' },
    ])
  })

  it('uploads directory-only selections independently', async () => {
    const uploadTree = vi.fn<FSHandle['uploadTree']>().mockResolvedValue({
      kind: 'written',
      bytesWritten: 0n,
      filesWritten: 0n,
      directoriesWritten: 1n,
      versions: [],
    })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager())
    act(() => {
      result.current.addFiles(handle, [], ['nested', 'nested/empty'])
    })

    await waitFor(() => {
      expect(uploadTree).toHaveBeenCalledTimes(2)
    })

    expect(uploadTree.mock.calls.map(([entry]) => entry[0])).toEqual([
      { kind: 'directory', path: 'nested' },
      { kind: 'directory', path: 'nested/empty' },
    ])
  })

  describe('a dropped name the folder already holds', () => {
    it('asks before sending any of its bytes', async () => {
      const folder = fakeFolder({ 'x.txt': 1 })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt', 'new.txt'])

      await waitFor(() => {
        expect(result.current.question?.item.name).toBe('x.txt')
      })
      expect(result.current.question?.item.conflict?.version).toEqual(
        new Uint8Array([1]),
      )
      expect(folder.sent()).toEqual([['new.txt', { kind: 'missing' }]])
    })

    it('replaces the version shown, and asks again when it changed', async () => {
      // Another writer saves version 2 before the first Replace commits.
      const folder = fakeFolder({ 'x.txt': 1 }, [{ 'x.txt': 2 }])
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt'])
      await asked(result, 'x.txt')

      press(result, 'replace')
      await waitFor(() => {
        expect(result.current.question?.item.conflict?.version).toEqual(
          new Uint8Array([2]),
        )
      })
      press(result, 'replace')
      await waitFor(() => {
        expect(result.current.items[0]?.status).toBe('done')
      })
      expect(folder.sent()).toEqual([
        ['x.txt', { kind: 'version', version: new Uint8Array([1]) }],
        ['x.txt', { kind: 'version', version: new Uint8Array([2]) }],
      ])
    })

    it('keeps both under the first free numbered name', async () => {
      const folder = fakeFolder({ 'x.txt': 1, 'x (2).txt': 1 })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt'])
      await asked(result, 'x.txt')

      press(result, 'keep')
      await waitFor(() => {
        expect(result.current.items[0]?.status).toBe('done')
      })
      expect(folder.sent()).toEqual([['x (3).txt', { kind: 'missing' }]])
    })

    it('keeps both past a taken name the listing misses', async () => {
      // The listing never shows x (2).txt, which the store holds.
      const folder = fakeFolder(
        { 'x.txt': 1, 'x (2).txt': 1 },
        [],
        ['x (2).txt'],
      )
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt'])
      await asked(result, 'x.txt')

      press(result, 'keep')
      await waitFor(() => {
        expect(result.current.items[0]?.status).toBe('done')
      })
      expect(folder.sent()).toEqual([
        ['x (2).txt', { kind: 'missing' }],
        ['x (3).txt', { kind: 'missing' }],
      ])
    })

    it('sends nothing for a skipped file', async () => {
      const folder = fakeFolder({ 'x.txt': 1 })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt'])
      await asked(result, 'x.txt')

      press(result, 'skip')
      expect(result.current.items[0]?.status).toBe('skipped')
      expect(result.current.question).toBeNull()
      expect(folder.sent()).toEqual([])
    })

    it('applies Replace to every file of the drop, never a folder', async () => {
      const folder = fakeFolder({ 'a.txt': 1, 'b.txt': 2, c: 'dir' })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['a.txt', 'b.txt', 'c'])
      await waitFor(() => {
        expect(result.current.question?.others).toHaveLength(2)
      })

      press(result, 'replace', true)
      await waitFor(() => {
        expect(folder.sent()).toHaveLength(2)
      })
      expect(folder.sent()).toEqual([
        ['a.txt', { kind: 'version', version: new Uint8Array([1]) }],
        ['b.txt', { kind: 'version', version: new Uint8Array([2]) }],
      ])
      // The folder's name still asks, with no version to replace.
      await asked(result, 'c')
      expect(result.current.question?.item.conflict?.version).toBeNull()
    })

    it('offers no Replace over a folder', async () => {
      const folder = fakeFolder({ x: 'dir' })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x'])
      await asked(result, 'x')

      press(result, 'replace')
      expect(result.current.question?.item.name).toBe('x')
      expect(folder.sent()).toEqual([])
    })
  })
})

// A FakeFolder is a folder handle holding files at a one byte version and
// folders, whose uploadTree commits like the store: it refuses an expectation
// the name does not meet with the version it holds. Each later write in writes
// lands, as another writer's, just before the next commit. Its listing omits
// the names in unlisted.
type Holds = Record<string, number | 'dir'>

function fakeFolder(
  start: Holds,
  writes: Holds[] = [],
  unlisted: string[] = [],
) {
  const disk: Holds = { ...start }
  const calls: [string, TreeUploadExpect][] = []
  const versionOf = (name: string) => {
    const held = disk[name]
    return typeof held === 'number' ? new Uint8Array([held]) : undefined
  }
  const uploadTree = vi.fn(
    async ([entry]: TreeUploadEntry[]): Promise<TreeUploadResult> => {
      if (entry.kind !== 'file') throw new Error('want a file')
      calls.push([entry.path, entry.expect])
      Object.assign(disk, writes.shift() ?? {})
      const held = disk[entry.path]
      const want = entry.expect
      const ok =
        want.kind === 'overwrite' ||
        (want.kind === 'missing' && held === undefined) ||
        (want.kind === 'version' &&
          typeof held === 'number' &&
          want.version[0] === held)
      if (!ok) {
        return {
          kind: 'conflict',
          path: entry.path,
          version: versionOf(entry.path),
        }
      }
      disk[entry.path] = 100 + calls.length
      return {
        kind: 'written',
        bytesWritten: 1n,
        filesWritten: 1n,
        directoriesWritten: 0n,
      }
    },
  )
  const handle = {
    uploadTree,
    readdirAll: async () =>
      Object.entries(disk)
        .filter(([name]) => !unlisted.includes(name))
        .map(([name, held]) => ({ name, isDir: held === 'dir' })),
    lookupPath: async (path: string) => ({
      handle: {
        readAt: async () => ({ version: versionOf(path) ?? new Uint8Array() }),
        release: () => {},
      },
    }),
  } as unknown as FSHandle
  return { handle, sent: () => calls }
}

function drop(
  manager: UploadManager,
  folder: { handle: FSHandle },
  names: string[],
) {
  act(() => {
    manager.addFiles(
      folder.handle,
      names.map((name) => buildFile(name, 'mine')),
    )
  })
}

async function asked(result: { current: UploadManager }, name: string) {
  await waitFor(() => {
    expect(result.current.question?.item.name).toBe(name)
  })
}

function press(
  result: { current: UploadManager },
  choice: 'replace' | 'keep' | 'skip',
  all = false,
) {
  const id = result.current.question?.item.id
  if (!id) throw new Error('no question')
  act(() => {
    result.current.answer(id, choice, all)
  })
}
