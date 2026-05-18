import { describe, expect, it, vi } from 'vitest'
import { InitUnixFSOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { INIT_UNIXFS_OP_ID } from '@s4wave/core/space/world/ops/init-unixfs.js'

const h = vi.hoisted(() => ({
  mockCreateObjectWithBlockData: vi.fn(),
  mockSetObjectType: vi.fn(),
  mockUploadSeedTree: vi.fn(),
}))

vi.mock('./object-block.js', () => ({
  createObjectWithBlockData: h.mockCreateObjectWithBlockData,
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  setObjectType: h.mockSetObjectType,
}))

vi.mock('./unixfs-seed.js', () => ({
  uploadSeedTree: h.mockUploadSeedTree,
}))

import { createBlogClientSide } from './blog-seed.js'

describe('createBlogClientSide', () => {
  it('seeds the initial blog post via tree upload', async () => {
    h.mockCreateObjectWithBlockData.mockResolvedValue(undefined)
    h.mockSetObjectType.mockResolvedValue(undefined)
    h.mockUploadSeedTree.mockResolvedValue(undefined)

    const worldState = {
      applyWorldOp: vi.fn().mockResolvedValue(undefined),
      getObject: vi.fn().mockResolvedValue(null),
    }

    const timestamp = new Date('2026-04-19T00:00:00Z')
    await createBlogClientSide(
      worldState as never,
      'blog/site',
      'Blog',
      '',
      '',
      timestamp,
    )

    expect(worldState.applyWorldOp).toHaveBeenCalledWith(
      INIT_UNIXFS_OP_ID,
      expect.any(Uint8Array),
      '',
      undefined,
    )
    const initOp = InitUnixFSOp.fromBinary(
      worldState.applyWorldOp.mock.calls[0]?.[1] as Uint8Array,
    )
    expect(initOp.objectKey).toBe('fs/blog/site')
    expect(initOp.timestamp?.toISOString()).toBe(timestamp.toISOString())

    expect(h.mockUploadSeedTree).toHaveBeenCalledWith(
      worldState,
      'fs/blog/site',
      expect.arrayContaining([
        expect.objectContaining({
          path: 'hello-world.md',
        }),
      ]),
      undefined,
      undefined,
    )
  })
})
