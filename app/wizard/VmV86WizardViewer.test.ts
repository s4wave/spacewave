import { describe, expect, it, vi } from 'vitest'

import type { Space } from '@s4wave/sdk/space/space.js'
import { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import { V86ImageTypeID } from '@s4wave/sdk/vm/v86image.js'

import {
  getV86CatalogErrorCopy,
  loadCdnV86ImagesFromSpace,
} from './VmV86WizardViewer.js'

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  listObjectsWithType: vi.fn().mockResolvedValue(['v86image-default']),
}))

describe('loadCdnV86ImagesFromSpace', () => {
  it('requests typed V86Image unmarshalling before decoding catalog metadata', async () => {
    const typedData = V86Image.toBinary({
      name: 'Aperture Linux',
      platform: 'v86',
      tags: ['default'],
    })
    const rawData = new Uint8Array([
      0x0a, 0x07, 0x67, 0x61, 0x72, 0x62, 0x6c, 0x65, 0x64,
    ])
    const unmarshal = vi.fn(
      async (req: { blockType?: string }, _signal: AbortSignal) => ({
        found: true,
        data: req.blockType === V86ImageTypeID ? typedData : rawData,
      }),
    )
    const cursor = {
      unmarshal,
      [Symbol.dispose]: vi.fn(),
    }
    const objectState = {
      accessWorldState: vi.fn().mockResolvedValue(cursor),
      [Symbol.dispose]: vi.fn(),
    }
    const world = {
      getObject: vi.fn().mockResolvedValue(objectState),
    }
    const space = {
      accessWorldState: vi.fn().mockResolvedValue(world),
    } as unknown as Space
    const signal = new AbortController().signal

    const entries = await loadCdnV86ImagesFromSpace(space, signal)

    expect(unmarshal).toHaveBeenCalledWith(
      { blockType: V86ImageTypeID },
      signal,
    )
    expect(entries).toEqual([
      expect.objectContaining({
        objectKey: 'v86image-default',
        image: expect.objectContaining({ name: 'Aperture Linux' }),
      }),
    ])
  })
})

describe('getV86CatalogErrorCopy', () => {
  it('translates an unpublished catalog error without exposing backend detail', () => {
    const raw =
      'build cdn world engine: cdn shared object has no published head'

    const copy = getV86CatalogErrorCopy(new Error(raw))

    expect(copy).toEqual({
      title: 'No VM images are published yet',
      detail: 'This image catalog has no published images to copy.',
      unpublished: true,
    })
    expect(`${copy.title} ${copy.detail}`).not.toContain(raw)
  })

  it('gives other catalog failures a retryable user-level presentation', () => {
    const copy = getV86CatalogErrorCopy(new Error('dial tcp: connection reset'))

    expect(copy).toEqual({
      title: 'Image catalog unavailable',
      detail: 'The VM image catalog could not be loaded. Try again.',
      unpublished: false,
    })
  })
})
