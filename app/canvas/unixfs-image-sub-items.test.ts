import { describe, expect, it, vi } from 'vitest'

import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

import { getUnixFSImageSubItems } from './unixfs-image-sub-items.js'

function makeHandle(entries: Array<{ name: string; isDir: boolean }>) {
  return {
    readdirAll: vi.fn().mockResolvedValue(entries),
    lookupPath: vi.fn(),
    release: vi.fn(),
    [Symbol.dispose]: vi.fn(),
  }
}

describe('getUnixFSImageSubItems', () => {
  it('filters root entries to image files', async () => {
    const root = makeHandle([
      { name: 'photos', isDir: true },
      { name: 'cover.png', isDir: false },
      { name: 'notes.txt', isDir: false },
    ])

    const items = await getUnixFSImageSubItems(
      root as unknown as FSHandle,
      '',
      new AbortController().signal,
    )

    expect(items.map((item) => item.label)).toEqual(['/cover.png'])
    expect(items[0]?.id).toBe('/cover.png')
  })

  it('completes and filters an image path inside the selected directory', async () => {
    const root = makeHandle([])
    const photos = makeHandle([
      { name: 'cat.webp', isDir: false },
      { name: 'car.txt', isDir: false },
      { name: 'dog.png', isDir: false },
    ])
    root.lookupPath.mockResolvedValue({ handle: photos })

    const items = await getUnixFSImageSubItems(
      root as unknown as FSHandle,
      'photos/ca',
      new AbortController().signal,
    )

    expect(root.lookupPath).toHaveBeenCalledWith(
      'photos',
      expect.any(AbortSignal),
    )
    expect(items.map((item) => item.id)).toEqual(['/photos/cat.webp'])
    expect(photos[Symbol.dispose]).toHaveBeenCalledTimes(1)
  })
})
