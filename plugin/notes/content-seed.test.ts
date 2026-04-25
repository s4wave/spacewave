import { describe, expect, it, vi } from 'vitest'

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
    expect(buildNotebookUnixfsObjectKey('notebook/project-notes')).toBe(
      'fs/project-notes',
    )
    expect(buildNotebookUnixfsObjectKey('notes')).toBe('notes-fs')
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
      'notebook/project-notes',
      'fs/project-notes',
      'Notebook',
      timestamp,
    )

    expect(worldState.applyWorldOp).toHaveBeenCalledWith(
      'hydra/unixfs/init',
      expect.any(Uint8Array),
      '',
      undefined,
    )
    expect(h.mockUploadSeedTree).toHaveBeenCalledWith(
      worldState,
      'fs/project-notes',
      expect.arrayContaining([
        expect.objectContaining({ path: 'welcome.md' }),
        expect.objectContaining({ path: 'getting-started.md' }),
      ]),
      undefined,
      undefined,
    )
    expect(h.mockSetObjectType).toHaveBeenCalledWith(
      worldState,
      'notebook/project-notes',
      'spacewave-notes/notebook',
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
      [expect.objectContaining({ path: 'index.md' })],
      undefined,
      undefined,
    )
    expect(h.mockSetObjectType).toHaveBeenCalledWith(
      worldState,
      'docs/reference',
      'spacewave-notes/docs',
      undefined,
    )
  })
})
