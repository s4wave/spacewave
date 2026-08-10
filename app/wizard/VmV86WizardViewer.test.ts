import { describe, expect, it, vi } from 'vitest'

import { V86ImageTypeID } from '@s4wave/sdk/vm/v86image.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import { V86Image } from '@s4wave/sdk/vm/v86.pb.js'

import {
  getV86CatalogErrorCopy,
  loadCdnV86ImagesFromSpace,
} from './VmV86WizardViewer.js'

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  listObjectsWithType: vi.fn().mockResolvedValue(['v86image-default']),
}))

describe('loadCdnV86ImagesFromSpace', () => {
  it('unmarshals typed V86Image metadata before decoding catalog entries', async () => {
    const typedData = V86Image.toBinary({
      name: 'Aperture Linux',
      platform: 'v86',
      tags: ['default'],
    })
    const unmarshal = vi.fn(
      (_req: { blockType?: string }, _signal: AbortSignal) =>
        Promise.resolve({
          found: true,
          data: typedData,
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
      [Symbol.dispose]: vi.fn(),
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
    expect(world[Symbol.dispose]).toHaveBeenCalledOnce()
    expect(entries).toHaveLength(1)
    expect(entries[0]?.objectKey).toBe('v86image-default')
    expect(entries[0]?.image.name).toBe('Aperture Linux')
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
