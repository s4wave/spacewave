import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { FSHandle, TreeUploadEntry } from '@s4wave/sdk/unixfs/handle.js'
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
  it('batches one addFiles call into one tree upload', async () => {
    const uploadTree = vi.fn<FSHandle['uploadTree']>().mockResolvedValue({
      bytesWritten: 8n,
      filesWritten: 2n,
      directoriesWritten: 1n,
    })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager(handle))
    act(() => {
      result.current.addFiles(
        [
          buildFile('child.txt', 'hello', 'nested/child.txt'),
          buildFile('top.txt', 'top'),
        ],
        ['nested'],
      )
    })

    await waitFor(() => {
      expect(uploadTree).toHaveBeenCalledTimes(1)
    })

    const [entries] = uploadTree.mock.calls[0] as [TreeUploadEntry[]]
    expect(entries).toHaveLength(3)
    expect(entries[0]).toMatchObject({ kind: 'directory', path: 'nested' })
    expect(entries[1]).toMatchObject({ kind: 'file', path: 'nested/child.txt' })
    expect(entries[2]).toMatchObject({ kind: 'file', path: 'top.txt' })
  })

  it('uploads directories-only selections as a tree batch', async () => {
    const uploadTree = vi.fn<FSHandle['uploadTree']>().mockResolvedValue({
      bytesWritten: 0n,
      filesWritten: 0n,
      directoriesWritten: 2n,
    })
    const handle = { uploadTree } as Pick<FSHandle, 'uploadTree'> as FSHandle

    const { result } = renderHook(() => useUploadManager(handle))
    act(() => {
      result.current.addFiles([], ['nested', 'nested/empty'])
    })

    await waitFor(() => {
      expect(uploadTree).toHaveBeenCalledTimes(1)
    })

    const [entries] = uploadTree.mock.calls[0] as [TreeUploadEntry[]]
    expect(entries).toEqual([
      { kind: 'directory', path: 'nested' },
      { kind: 'directory', path: 'nested/empty' },
    ])
  })
})
