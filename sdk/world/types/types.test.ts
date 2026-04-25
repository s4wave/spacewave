import { describe, expect, it, vi } from 'vitest'

import { getObjectType } from './types.js'

describe('getObjectType', () => {
  it('falls back to the type index when quad object is missing', async () => {
    const iter = {
      close: vi.fn(),
      key: vi
        .fn()
        .mockResolvedValueOnce('types/git/repo')
        .mockResolvedValueOnce('types/unixfs/fs-node'),
      next: vi
        .fn()
        .mockResolvedValueOnce(true)
        .mockResolvedValueOnce(true)
        .mockResolvedValueOnce(false),
    }
    const ws = {
      iterateObjects: vi.fn().mockResolvedValue(iter),
      listObjectsWithType: vi
        .fn()
        .mockResolvedValueOnce(['repo-1'])
        .mockResolvedValueOnce([]),
      lookupGraphQuads: vi.fn().mockResolvedValue({
        quads: [{ subject: '<repo-1>', predicate: '<type>' }],
      }),
    }

    await expect(getObjectType(ws as never, 'repo-1')).resolves.toBe('git/repo')
    expect(ws.iterateObjects).toHaveBeenCalledWith('types/', false, undefined)
    expect(iter.close).toHaveBeenCalledWith(undefined)
  })
})
