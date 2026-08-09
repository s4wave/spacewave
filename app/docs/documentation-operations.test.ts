import { describe, expect, it, vi } from 'vitest'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import {
  createDocumentationPage,
  saveDocumentationPage,
} from './documentation-operations.js'

function fakeFile(writeAt = vi.fn().mockResolvedValue(1n)) {
  const release = vi.fn()
  const handle = {
    writeAt,
    truncate: vi.fn().mockResolvedValue(undefined),
    release,
    [Symbol.dispose]: release,
  } as unknown as FSHandle
  return { handle, release }
}

function fakeRoot(child: FSHandle) {
  return {
    mknod: vi.fn().mockResolvedValue(undefined),
    lookup: vi.fn().mockResolvedValue(child),
  } as unknown as FSHandle
}

describe('documentation operations', () => {
  it('writes and truncates a page before reporting save success', async () => {
    const { handle } = fakeFile()
    await saveDocumentationPage(handle, 'draft')
    expect(handle.writeAt).toHaveBeenCalledOnce()
    expect(handle.truncate).toHaveBeenCalledWith(5n, undefined)
  })

  it('releases a created handle after success', async () => {
    const { handle, release } = fakeFile()
    await expect(createDocumentationPage(fakeRoot(handle), [])).resolves.toBe(
      'untitled.md',
    )
    expect(release).toHaveBeenCalledOnce()
  })

  it('releases a created handle after a write error', async () => {
    const { handle, release } = fakeFile(
      vi.fn().mockRejectedValue(new Error('disk full')),
    )
    await expect(createDocumentationPage(fakeRoot(handle), [])).rejects.toThrow(
      'disk full',
    )
    expect(release).toHaveBeenCalledOnce()
  })

  it('releases a created handle when creation is cancelled', async () => {
    const controller = new AbortController()
    const { handle, release } = fakeFile(
      vi.fn().mockImplementation(async (_offset, _data, signal) => {
        if (signal?.aborted) throw signal.reason
        return 1n
      }),
    )
    controller.abort(new Error('cancelled'))
    await expect(
      createDocumentationPage(fakeRoot(handle), [], controller.signal),
    ).rejects.toThrow('cancelled')
    expect(release).toHaveBeenCalledOnce()
  })
})
