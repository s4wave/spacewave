import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { useUploadManager } from './useUploadManager.js'

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
      bytesWritten: 8n,
      filesWritten: 1n,
      directoriesWritten: 0n,
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
    expect(entries).toMatchObject([
      { kind: 'file', path: 'nested/child.txt' },
      { kind: 'file', path: 'top.txt' },
      { kind: 'directory', path: 'nested' },
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
      bytesWritten: 8n,
      filesWritten: 2n,
      directoriesWritten: 0n,
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
          bytesWritten: 2n,
          filesWritten: 1n,
          directoriesWritten: 0n,
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
      bytesWritten: 0n,
      filesWritten: 0n,
      directoriesWritten: 1n,
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
  it('clears terminal errors once without contaminating a later completion', async () => {
    vi.useFakeTimers()
    try {
      const uploadTree = vi
        .fn<FSHandle['uploadTree']>()
        .mockRejectedValueOnce(new Error('first upload failed'))
        .mockResolvedValue({
          bytesWritten: 2n,
          filesWritten: 1n,
          directoriesWritten: 0n,
        })
      const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle
      const { result } = renderHook(() => useUploadManager())

      act(() => {
        result.current.addFiles(handle, [buildFile('broken.txt', 'no')])
      })
      await act(async () => {
        await Promise.resolve()
        await Promise.resolve()
        await Promise.resolve()
      })
      expect(result.current.lastEvent).toMatchObject({
        kind: 'completed',
        fileCount: 0,
        errorCount: 1,
      })

      act(() => {
        vi.advanceTimersByTime(3000)
      })
      expect(result.current.items).toEqual([])
      act(() => {
        vi.advanceTimersByTime(6000)
      })
      expect(result.current.items).toEqual([])

      act(() => {
        result.current.addFiles(handle, [buildFile('good.txt', 'ok')])
      })
      await act(async () => {
        await Promise.resolve()
        await Promise.resolve()
        await Promise.resolve()
      })
      expect(result.current.lastEvent).toMatchObject({
        kind: 'completed',
        fileCount: 1,
        errorCount: 0,
      })
    } finally {
      vi.useRealTimers()
    }
  })
  it('excludes prior terminal failures when a new burst starts immediately', async () => {
    const uploadTree = vi
      .fn<FSHandle['uploadTree']>()
      .mockRejectedValueOnce(new Error('first upload failed'))
      .mockResolvedValueOnce({
        bytesWritten: 2n,
        filesWritten: 1n,
        directoriesWritten: 0n,
      })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle
    const { result } = renderHook(() => useUploadManager())

    act(() => {
      result.current.addFiles(handle, [buildFile('broken.txt', 'no')])
    })
    await waitFor(() => {
      expect(result.current.lastEvent).toMatchObject({
        kind: 'completed',
        errorCount: 1,
      })
    })

    act(() => {
      result.current.addFiles(handle, [buildFile('good.txt', 'ok')])
    })
    await waitFor(() => {
      expect(result.current.lastEvent).toMatchObject({
        kind: 'completed',
        fileCount: 1,
        errorCount: 0,
      })
    })
  })
})
