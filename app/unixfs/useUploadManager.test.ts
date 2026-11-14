import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type {
  FSHandle,
  TreeUploadEntry,
  TreeUploadExpect,
  TreeUploadResult,
} from '@s4wave/sdk/unixfs/handle.js'
import { treeUploadConcurrency } from '@s4wave/sdk/unixfs/upload-pool.js'
import {
  answersFor,
  useUploadManager,
  type UploadAnswer,
  type UploadManager,
} from './useUploadManager.js'

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
    const handle = plain(uploadTree)

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

    const entries = uploadTree.mock.calls
      .map(([entry]) => entry[0])
      .sort((a, b) => a.path.localeCompare(b.path))
    // A free folder name is sent, then what was dropped inside it.
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
    const handle = plain(uploadTree)

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
    const handle = plain(uploadTree)

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
    const handle = plain(uploadTree)

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
    const handle = plain(uploadTree)

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
    const handle = plain(uploadTree)

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

      await asked(result, 'x.txt')
      expect(result.current.question?.item.conflict).toEqual({
        holds: 'file',
        version: new Uint8Array([1]),
      })
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
      await done(result)
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
      await done(result)
      expect(folder.sent()).toEqual([
        ['x (2).txt', { kind: 'missing' }],
        ['x (3).txt', { kind: 'missing' }],
      ])
    })

    it('keeps both past a numbered name and the names its own drop sends', async () => {
      // a (2).txt is taken and dropped too: keeping a.txt must not take
      // a (2).txt from the drop, and keeping a (2).txt counts on from it.
      const folder = fakeFolder({ 'a.txt': 1, 'a (2).txt': 2 })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['a.txt', 'a (2).txt'])
      await waitFor(() => {
        expect(result.current.question?.others).toHaveLength(1)
      })

      press(result, 'keep', true)
      await done(result)
      expect(
        folder
          .sent()
          .map(([path]) => path)
          .sort(),
      ).toEqual(['a (3).txt', 'a (4).txt'])
    })

    it('sends nothing for a skipped file', async () => {
      const folder = fakeFolder({ 'x.txt': 1 })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt'])
      await asked(result, 'x.txt')

      press(result, 'skip')
      await waitFor(() => {
        expect(result.current.items[0]?.status).toBe('skipped')
      })
      expect(result.current.question).toBeNull()
      expect(folder.sent()).toEqual([])
    })

    it('applies Replace to every file of the drop, never a folder', async () => {
      const folder = fakeFolder({ 'a.txt': 1, 'b.txt': 2, c: 'dir' })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['a.txt', 'b.txt', 'c'])
      await waitFor(() => {
        expect(result.current.question?.others).toHaveLength(1)
      })

      press(result, 'replace', true)
      await waitFor(() => {
        expect(folder.sent()).toHaveLength(2)
      })
      expect(folder.sent()).toEqual([
        ['a.txt', { kind: 'version', version: new Uint8Array([1]) }],
        ['b.txt', { kind: 'version', version: new Uint8Array([2]) }],
      ])
      // The folder's name still asks, offering no Replace.
      await asked(result, 'c')
      expect(answersFor(result.current.question!.item)).toEqual([
        'keep',
        'skip',
      ])
    })

    it('offers no Replace over a folder or a link', async () => {
      const folder = fakeFolder({ x: 'dir', y: 'link' })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x', 'y'])
      await asked(result, 'x')
      expect(result.current.question?.item.conflict?.holds).toBe('folder')

      press(result, 'replace')
      expect(result.current.question?.item.name).toBe('x')
      press(result, 'skip')
      await asked(result, 'y')
      expect(result.current.question?.item.conflict?.holds).toBe('link')
      expect(answersFor(result.current.question!.item)).toEqual([
        'keep',
        'skip',
      ])
      expect(folder.sent()).toEqual([])
    })

    it('answers with the drop’s own handle after the browsing one is released', async () => {
      const folder = fakeFolder({ 'x.txt': 1 })
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, ['x.txt'])
      await asked(result, 'x.txt')

      // The person navigates away: the folder view releases its handle.
      folder.handle.release()
      press(result, 'replace')
      await done(result)
      expect(folder.sent()).toEqual([
        ['x.txt', { kind: 'version', version: new Uint8Array([1]) }],
      ])
      await waitFor(() => {
        expect(folder.held()).toBe(0)
      })
    })

    it('bounds the listing and version reads to the pool limit', async () => {
      const names = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].map(
        (name) => `${name}.txt`,
      )
      const folder = fakeFolder(Object.fromEntries(names.map((n) => [n, 1])))
      const { result } = renderHook(() => useUploadManager())
      drop(result.current, folder, names)
      await waitFor(() => {
        expect(result.current.question?.others).toHaveLength(7)
      })
      expect(folder.listed()).toBe(1)
      expect(folder.peak()).toBeLessThanOrEqual(treeUploadConcurrency)
    })
  })

  it('takes a new clone on Retry after the drop could not clone its folder', async () => {
    const folder = fakeFolder({})
    let clones = 0
    const handle = {
      ...folder.handle,
      clone: () =>
        clones++ ? folder.handle.clone() : Promise.reject(new Error('gone')),
    } as FSHandle
    const { result } = renderHook(() => useUploadManager())
    drop(result.current, { handle }, ['a.txt', 'b.txt'])
    await waitFor(() => {
      expect(result.current.items.map((item) => item.status)).toEqual([
        'error',
        'error',
      ])
    })

    act(() => {
      result.current.retryUpload(result.current.items[0].id)
    })
    await done(result)
    expect(clones).toBe(2)
    expect(folder.sent().sort()).toEqual([
      ['a.txt', { kind: 'missing' }],
      ['b.txt', { kind: 'missing' }],
    ])
  })

  describe('a listing that fails', () => {
    it('is not cached when the row that asked for it is cancelled', async () => {
      const folder = fakeFolder({ A: 'dir', 'A/a.txt': 1 })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['x.txt', 'A/b.txt'])
      const x = result.current.items.find((item) => item.path === 'x.txt')!
      act(() => {
        result.current.cancelUpload(x.id)
      })

      await asked(result, 'A')
      expect(folder.sent()).toEqual([])
    })

    it('fails the rows it decides, and Retry reads it again', async () => {
      const folder = fakeFolder({ 'a.txt': 1, A: 'dir' }, [], [], {
        lists: [1],
      })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['a.txt', 'A/b.txt'])
      const status = (path: string) =>
        result.current.items.find((item) => item.path === path)?.status
      await waitFor(() => {
        expect([status('a.txt'), status('A'), status('A/b.txt')]).toEqual([
          'error',
          'error',
          'waiting',
        ])
      })
      expect(folder.sent()).toEqual([])

      for (const path of ['a.txt', 'A']) {
        act(() => {
          result.current.retryUpload(
            result.current.items.find((item) => item.path === path)!.id,
          )
        })
      }
      await waitFor(() => {
        expect([status('a.txt'), status('A')]).toEqual(['asking', 'asking'])
      })
      expect(folder.sent()).toEqual([])
    })
  })

  describe('a failed folder row', () => {
    // folderStatus returns the statuses of the dropped folder A and its file.
    const statuses = (result: { current: UploadManager }) =>
      ['A', 'A/a.txt'].map(
        (path) =>
          result.current.items.find(
            (item) => item.name === path.split('/').at(-1),
          )?.status,
      )
    const retryA = (result: { current: UploadManager }) =>
      act(() => {
        result.current.retryUpload(
          result.current.items.find((item) => item.name === 'A')!.id,
        )
      })

    it('retries its check and opens what waits in it', async () => {
      const folder = fakeFolder({}, [], [], { lists: [1] })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['A/a.txt'])
      await waitFor(() => {
        expect(statuses(result)).toEqual(['error', 'waiting'])
      })

      retryA(result)
      await done(result)
      expect(folder.sent()).toEqual([
        ['A', { kind: 'directory' }],
        ['A/a.txt', { kind: 'missing' }],
      ])
    })

    it('retries its Keep both pick and opens what waits in it', async () => {
      const folder = fakeFolder({ A: 'dir' }, [], [], { lists: [2] })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['A/a.txt'])
      await asked(result, 'A')
      press(result, 'keep')
      await waitFor(() => {
        expect(statuses(result)).toEqual(['error', 'waiting'])
      })

      retryA(result)
      await done(result)
      expect(folder.sent()).toEqual([
        ['A (2)', { kind: 'directory' }],
        ['A (2)/a.txt', { kind: 'missing' }],
      ])
    })

    it('retries its send', async () => {
      const folder = fakeFolder({}, [], [], { sends: [1] })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['A/a.txt'])
      await waitFor(() => {
        expect(statuses(result)[0]).toBe('error')
      })

      retryA(result)
      await done(result)
      expect(folder.sent()).toContainEqual(['A', { kind: 'directory' }])
    })
  })

  it('answers a question once, however fast the presses', async () => {
    const folder = fakeFolder({ 'a.txt': 1 })
    const { result } = renderHook(() => useUploadManager())
    drop(result.current, folder, ['a.txt'])
    await asked(result, 'a.txt')

    const id = result.current.question!.item.id
    act(() => {
      result.current.answer(id, 'skip', false)
      result.current.answer(id, 'replace', false)
    })
    await waitFor(() => {
      expect(result.current.items[0]?.status).toBe('skipped')
    })
    expect(folder.sent()).toEqual([])
  })

  describe('a dropped folder whose name the folder already holds', () => {
    it('asks once, and merges its files asking as files do', async () => {
      const folder = fakeFolder({ photos: 'dir', 'photos/a.jpg': 1 })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/a.jpg', 'photos/b.jpg'])
      await asked(result, 'photos')
      expect(folder.sent()).toEqual([])
      expect(answersFor(result.current.question!.item)).toEqual([
        'merge',
        'keep',
        'skip',
      ])

      press(result, 'merge')
      await asked(result, 'a.jpg')
      expect(folder.sent().sort()).toEqual([
        ['photos', { kind: 'directory' }],
        ['photos/b.jpg', { kind: 'missing' }],
      ])
    })

    it('keeps both, sending its files under the new name', async () => {
      const folder = fakeFolder({ photos: 'dir', 'photos/a.jpg': 1 })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/a.jpg'])
      await asked(result, 'photos')

      press(result, 'keep')
      await done(result)
      expect(folder.sent()).toEqual([
        ['photos (2)', { kind: 'directory' }],
        ['photos (2)/a.jpg', { kind: 'missing' }],
      ])
    })

    it('skips its whole tree', async () => {
      const folder = fakeFolder({ photos: 'dir' })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/a.jpg', 'photos/b.jpg'])
      await asked(result, 'photos')

      press(result, 'skip')
      await waitFor(() => {
        expect(
          result.current.items.every((item) => item.status === 'skipped'),
        ).toBe(true)
      })
      expect(folder.sent()).toEqual([])
    })

    it('keeps both from two drops under two names', async () => {
      const folder = fakeFolder({ photos: 'dir' })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/a.jpg'])
      dropTree(result.current, folder, ['photos/b.jpg'])
      await asked(result, 'photos')

      // Both drops answer before either folder is written.
      const ids = result.current.items
        .filter((item) => item.kind === 'directory')
        .map((item) => item.id)
      act(() => {
        for (const id of ids) result.current.answer(id, 'keep', false)
      })
      await waitFor(() => {
        expect(folder.sent()).toHaveLength(4)
      })
      expect(folder.sent().sort()).toEqual([
        ['photos (2)', { kind: 'directory' }],
        ['photos (2)/a.jpg', { kind: 'missing' }],
        ['photos (3)', { kind: 'directory' }],
        ['photos (3)/b.jpg', { kind: 'missing' }],
      ])
    })

    it('cancels what it holds with it while it asks', async () => {
      const folder = fakeFolder({ photos: 'dir' })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/a.jpg', 'photos/b.jpg'])
      await asked(result, 'photos')

      act(() => {
        result.current.cancelUpload(result.current.question!.item.id)
      })
      expect(result.current.items).toEqual([])
      await waitFor(() => {
        expect(folder.held()).toBe(1)
      })
      expect(folder.sent()).toEqual([])
    })

    it('asks about a subfolder whose name a file holds in a merge', async () => {
      const folder = fakeFolder({ photos: 'dir', 'photos/raw': 1 })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/raw/a.jpg', 'photos/b.jpg'])
      await asked(result, 'photos')

      press(result, 'merge')
      await asked(result, 'raw')
      expect(result.current.question?.item.conflict?.holds).toBe('file')
      expect(answersFor(result.current.question!.item)).toEqual([
        'keep',
        'skip',
      ])
      press(result, 'keep')
      await done(result)
      expect(folder.sent().sort()).toEqual([
        ['photos', { kind: 'directory' }],
        ['photos/b.jpg', { kind: 'missing' }],
        ['photos/raw (2)', { kind: 'directory' }],
        ['photos/raw (2)/a.jpg', { kind: 'missing' }],
      ])
    })

    it('never replaces a file of its name', async () => {
      const folder = fakeFolder({ photos: 1 })
      const { result } = renderHook(() => useUploadManager())
      dropTree(result.current, folder, ['photos/a.jpg'])
      await asked(result, 'photos')
      expect(result.current.question?.item.conflict?.holds).toBe('file')
      expect(answersFor(result.current.question!.item)).toEqual([
        'keep',
        'skip',
      ])
      expect(folder.sent()).toEqual([])
    })
  })
})

// A FakeFolder is a folder handle over disk, which maps each path below it to
// a file at a one byte version, a folder or a link. Its uploadTree commits
// like the store: it refuses a file expectation the path does not meet with
// the version it holds, and merges a folder into a folder. Each later write in
// writes lands, as another writer's, just before the next file commit. Its
// listing omits the names in unlisted. The listings and commits counted in
// faults (from 1) reject, and a listing whose signal ends rejects. clone hands out handles to the same
// folder; a released handle refuses every call.
type Holds = Record<string, number | 'dir' | 'link'>

function fakeFolder(
  start: Holds,
  writes: Holds[] = [],
  unlisted: string[] = [],
  faults: { lists?: number[]; sends?: number[] } = {},
) {
  const disk: Holds = { ...start }
  const calls: [string, TreeUploadExpect | { kind: 'directory' }][] = []
  let held = 0
  let listed = 0
  let reading = 0
  let peak = 0
  const versionOf = (path: string) => {
    const at = disk[path]
    return typeof at === 'number' ? new Uint8Array([at]) : undefined
  }
  // read counts a listing or version read in flight across a macrotask.
  const read = async <T>(run: () => T): Promise<T> => {
    peak = Math.max(peak, ++reading)
    await new Promise((resolve) => setTimeout(resolve, 1))
    reading--
    return run()
  }
  const list = (dir: string, signal?: AbortSignal) =>
    read(() => {
      if (signal?.aborted) throw new Error('aborted')
      if (faults.lists?.includes(listed)) throw new Error('stream closed')
      return Object.entries(disk)
        .filter(([path]) => {
          const cut = path.lastIndexOf('/')
          const parent = cut < 0 ? '' : path.slice(0, cut)
          return parent === dir && !unlisted.includes(path)
        })
        .map(([path, at]) => ({
          name: path.slice(path.lastIndexOf('/') + 1),
          isDir: at === 'dir',
          isSymlink: at === 'link',
        }))
    })
  let commits = 0
  const commit = async ([
    entry,
  ]: TreeUploadEntry[]): Promise<TreeUploadResult> => {
    if (faults.sends?.includes(++commits)) throw new Error('stream closed')
    const written: TreeUploadResult = {
      kind: 'written',
      bytesWritten: 1n,
      filesWritten: 1n,
      directoriesWritten: 0n,
      versions: [],
    }
    if (entry.kind === 'directory') {
      calls.push([entry.path, { kind: 'directory' }])
      if (disk[entry.path] === undefined) disk[entry.path] = 'dir'
      if (disk[entry.path] !== 'dir') throw new Error('file already exists')
      return written
    }
    calls.push([entry.path, entry.expect])
    Object.assign(disk, writes.shift() ?? {})
    const at = disk[entry.path]
    const want = entry.expect
    const ok =
      want.kind === 'overwrite' ||
      (want.kind === 'missing' && at === undefined) ||
      (want.kind === 'version' &&
        typeof at === 'number' &&
        want.version[0] === at)
    if (!ok) {
      return {
        kind: 'conflict',
        path: entry.path,
        version: versionOf(entry.path),
      }
    }
    disk[entry.path] = 100 + calls.length
    return written
  }
  const open = (): FSHandle => {
    let released = false
    const live =
      <A extends unknown[], T>(run: (...args: A) => T) =>
      (...args: A): T => {
        if (released) throw new Error('handle released')
        return run(...args)
      }
    held++
    return {
      uploadTree: live(commit),
      readdirAll: live((_: unknown, signal?: AbortSignal) => {
        listed++
        return list('', signal)
      }),
      lookupPath: live(async (path: string) => ({
        handle: {
          readdirAll: (_: unknown, signal?: AbortSignal) => {
            listed++
            return list(path, signal)
          },
          readAt: () =>
            read(() => ({ version: versionOf(path) ?? new Uint8Array() })),
          release: () => {},
        },
      })),
      clone: live(async () => open()),
      release: () => {
        if (!released) held--
        released = true
      },
    } as unknown as FSHandle
  }
  const handle = open()
  return {
    handle,
    sent: () => calls,
    held: () => held,
    listed: () => listed,
    peak: () => peak,
  }
}

// plain returns a handle on an empty folder whose only work is uploadTree;
// its clones share it.
function plain(uploadTree: FSHandle['uploadTree']): FSHandle {
  const handle = {
    uploadTree,
    readdirAll: async () => [],
    clone: async () => handle,
    release: () => {},
  } as unknown as FSHandle
  return handle
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

// dropTree drops the files at paths inside a dropped folder.
function dropTree(
  manager: UploadManager,
  folder: { handle: FSHandle },
  paths: string[],
) {
  act(() => {
    manager.addFiles(
      folder.handle,
      paths.map((path) =>
        buildFile(path.slice(path.lastIndexOf('/') + 1), 'mine', path),
      ),
    )
  })
}

async function asked(result: { current: UploadManager }, name: string) {
  await waitFor(() => {
    expect(result.current.question?.item.name).toBe(name)
  })
}

async function done(result: { current: UploadManager }) {
  await waitFor(() => {
    expect(result.current.items.every((item) => item.status === 'done')).toBe(
      true,
    )
  })
}

function press(
  result: { current: UploadManager },
  choice: UploadAnswer,
  all = false,
) {
  const id = result.current.question?.item.id
  if (!id) throw new Error('no question')
  act(() => {
    result.current.answer(id, choice, all)
  })
}
