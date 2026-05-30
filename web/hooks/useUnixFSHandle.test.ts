import { describe, expect, it, vi } from 'vitest'

import {
  getMimeType,
  isAudioMimeType,
  isVideoMimeType,
  readUnixFSHandleStat,
  resolveUnixFSHandle,
} from './useUnixFSHandle.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'

describe('UnixFS MIME helpers', () => {
  it.each([
    ['song.mp3', 'audio/mpeg'],
    ['song.m4a', 'audio/mp4'],
    ['song.wav', 'audio/wav'],
    ['song.ogg', 'audio/ogg'],
    ['song.opus', 'audio/ogg'],
    ['song.oga', 'audio/ogg'],
    ['song.flac', 'audio/flac'],
    ['audiobook.m4b', 'audio/mp4'],
    ['ringtone.m4r', 'audio/mp4'],
  ])('classifies %s as audio', (filename, expected) => {
    const mimeType = getMimeType(filename)

    expect(mimeType).toBe(expected)
    expect(isAudioMimeType(mimeType)).toBe(true)
  })

  it('keeps webm extension routing as video media', () => {
    const mimeType = getMimeType('demo.webm')

    expect(mimeType).toBe('video/webm')
    expect(isVideoMimeType(mimeType)).toBe(true)
  })
})

describe('resolveUnixFSHandle', () => {
  it.each(['', '/', '.'])(
    'reuses the root handle for root path %s',
    async (path) => {
      const root = {
        clone: vi.fn(),
        lookupPath: vi.fn(),
      }
      const cleaned: unknown[] = []
      const cleanup: RegisterCleanup = (handle) => {
        cleaned.push(handle)
        return handle
      }
      const signal = new AbortController().signal

      const result = await resolveUnixFSHandle(
        root as unknown as FSHandle,
        path,
        signal,
        cleanup,
      )

      expect(result).toBe(root)
      expect(cleaned).toHaveLength(0)
      expect(root.clone).not.toHaveBeenCalled()
      expect(root.lookupPath).not.toHaveBeenCalled()
    },
  )

  it('looks up and owns non-root handles', async () => {
    const child = { [Symbol.dispose]: vi.fn() }
    const root = {
      lookupPath: vi.fn().mockResolvedValue({ handle: child }),
    }
    const cleaned: unknown[] = []
    const cleanup: RegisterCleanup = (handle) => {
      cleaned.push(handle)
      return handle
    }
    const signal = new AbortController().signal

    const result = await resolveUnixFSHandle(
      root as unknown as FSHandle,
      'docs',
      signal,
      cleanup,
    )

    expect(result).toBe(child)
    expect(root.lookupPath).toHaveBeenCalledWith('docs', signal)
    expect(cleaned).toEqual([child])
  })
})

describe('readUnixFSHandleStat', () => {
  it.each(['', '/', '.'])(
    'uses cached metadata for root handle path %s',
    async (path) => {
      const root = {
        getPath: vi.fn(() => path),
        getInfo: vi.fn(() => ({ isDir: true })),
        getFileInfo: vi.fn(),
      }

      const stat = await readUnixFSHandleStat(
        root as unknown as FSHandle,
        new AbortController().signal,
      )

      expect(stat.info).toEqual({ isDir: true })
      expect(stat.fileKind).toBe('directory')
      expect(stat.mimeType).toBe('inode/directory')
      expect(root.getFileInfo).not.toHaveBeenCalled()
    },
  )

  it('treats root handles with empty cached metadata as directories', async () => {
    const root = {
      getPath: vi.fn(() => '/'),
      getInfo: vi.fn(() => ({})),
      getFileInfo: vi.fn(),
    }

    const stat = await readUnixFSHandleStat(
      root as unknown as FSHandle,
      new AbortController().signal,
    )

    expect(stat.info).toEqual({ isDir: true })
    expect(stat.fileKind).toBe('directory')
    expect(stat.mimeType).toBe('inode/directory')
    expect(root.getFileInfo).not.toHaveBeenCalled()
  })

  it('fetches fresh metadata for non-root handles', async () => {
    const file = {
      getPath: vi.fn(() => 'getting-started.md'),
      getInfo: vi.fn(() => ({ isDir: false })),
      getFileInfo: vi.fn().mockResolvedValue({
        name: 'getting-started.md',
        isDir: false,
      }),
    }
    const signal = new AbortController().signal

    const stat = await readUnixFSHandleStat(file as unknown as FSHandle, signal)

    expect(stat.info).toEqual({ name: 'getting-started.md', isDir: false })
    expect(stat.fileKind).toBe('file')
    expect(stat.mimeType).toBe('text/markdown')
    expect(file.getFileInfo).toHaveBeenCalledWith(signal)
  })
})
