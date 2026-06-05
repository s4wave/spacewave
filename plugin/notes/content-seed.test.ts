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

import {
  buildNotebookUnixfsObjectKey,
  createDocsClientSide,
  createNotebookClientSide,
} from './content-seed.js'

describe('content-seed', () => {
  it('derives notebook unixfs keys from notebook object keys', () => {
    expect(buildNotebookUnixfsObjectKey('project-notes')).toBe(
      'project-notes-fs',
    )
  })

  it('seeds the initial notebook tree via tree upload', async () => {
    h.mockCreateObjectWithBlockData.mockResolvedValue(undefined)
    h.mockSetObjectType.mockResolvedValue(undefined)
    h.mockUploadSeedTree.mockResolvedValue(undefined)

    const worldState = {
      applyWorldOp: vi.fn().mockResolvedValue(undefined),
    }

    const timestamp = new Date('2026-04-20T00:00:00Z')
    await createNotebookClientSide(
      worldState as never,
      'project-notes',
      'project-notes-fs',
      'Notebook',
      timestamp,
    )

    expect(worldState.applyWorldOp).toHaveBeenCalledWith(
      INIT_UNIXFS_OP_ID,
      expect.any(Uint8Array),
      '',
      undefined,
    )
    const initOpBytes = worldState.applyWorldOp.mock.calls[0]?.[1]
    expect(initOpBytes).toBeInstanceOf(Uint8Array)
    if (!(initOpBytes instanceof Uint8Array)) {
      throw new Error('expected init op payload bytes')
    }
    const initOp = InitUnixFSOp.fromBinary(initOpBytes)
    expect(initOp.objectKey).toBe('project-notes-fs')
    expect(initOp.timestamp?.toISOString()).toBe(timestamp.toISOString())
    expect(h.mockUploadSeedTree).toHaveBeenCalledWith(
      worldState,
      'project-notes-fs',
      expect.arrayContaining([
        expect.objectContaining({
          path: 'welcome.md',
          content: expect.stringContaining('Welcome'),
        }),
        expect.objectContaining({
          path: 'getting-started.md',
          content: expect.stringContaining('Getting started'),
        }),
      ]),
      undefined,
      undefined,
    )
    expect(h.mockSetObjectType).toHaveBeenCalledWith(
      worldState,
      'project-notes',
      'notes/notebook',
      undefined,
    )
  })

  it('seeds the initial docs tree via tree upload', async () => {
    h.mockCreateObjectWithBlockData.mockResolvedValue(undefined)
    h.mockSetObjectType.mockResolvedValue(undefined)
    h.mockUploadSeedTree.mockResolvedValue(undefined)

    const worldState = {
      applyWorldOp: vi.fn().mockResolvedValue(undefined),
    }

    const timestamp = new Date('2026-04-20T00:00:00Z')
    await createDocsClientSide(
      worldState as never,
      'docs/reference',
      'Documentation',
      '',
      timestamp,
    )

    expect(h.mockUploadSeedTree).toHaveBeenCalledWith(
      worldState,
      'docs/reference-fs',
      [
        expect.objectContaining({
          path: 'index.md',
          content: expect.stringContaining('Documentation'),
        }),
      ],
      undefined,
      undefined,
    )
    expect(h.mockSetObjectType).toHaveBeenCalledWith(
      worldState,
      'docs/reference',
      'notes/docs',
      undefined,
    )
  })
})
