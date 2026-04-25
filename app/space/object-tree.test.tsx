import { describe, it, expect, beforeEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import type { WorldContentsObject } from '@s4wave/core/space/world/world.pb.js'
import {
  buildObjectTree,
  HIDDEN_OBJECT_TYPES,
  getObjectTypeLabel,
  getObjectTypeIcon,
  isHiddenSpaceObject,
} from '@s4wave/web/space/object-tree.js'

beforeEach(() => {
  cleanup()
})

describe('HIDDEN_OBJECT_TYPES', () => {
  it('contains space/settings', () => {
    expect(HIDDEN_OBJECT_TYPES.has('space/settings')).toBe(true)
  })

  it('contains the SpaceSettings block type', () => {
    expect(
      HIDDEN_OBJECT_TYPES.has(
        'github.com/s4wave/spacewave/core/space/world.SpaceSettings',
      ),
    ).toBe(true)
  })
})

describe('isHiddenSpaceObject', () => {
  it('hides the reserved settings object key', () => {
    expect(isHiddenSpaceObject('settings', 'canvas')).toBe(true)
  })

  it('hides the SpaceSettings block type', () => {
    expect(
      isHiddenSpaceObject(
        'custom-settings',
        'github.com/s4wave/spacewave/core/space/world.SpaceSettings',
      ),
    ).toBe(true)
  })
})

describe('getObjectTypeLabel', () => {
  it('returns Layout for alpha/object-layout', () => {
    expect(getObjectTypeLabel('alpha/object-layout')).toBe('Layout')
  })

  it('returns File System for unixfs/fs-node', () => {
    expect(getObjectTypeLabel('unixfs/fs-node')).toBe('File System')
  })

  it('returns Git Repository for git/repo', () => {
    expect(getObjectTypeLabel('git/repo')).toBe('Git Repository')
  })

  it('returns Git Worktree for git/worktree', () => {
    expect(getObjectTypeLabel('git/worktree')).toBe('Git Worktree')
  })

  it('returns Canvas for canvas', () => {
    expect(getObjectTypeLabel('canvas')).toBe('Canvas')
  })

  it('returns the type ID string for unknown types', () => {
    expect(getObjectTypeLabel('some/unknown-type')).toBe('some/unknown-type')
  })

  it('returns Object for empty string', () => {
    expect(getObjectTypeLabel('')).toBe('Object')
  })
})

describe('getObjectTypeIcon', () => {
  it('returns a React element for alpha/object-layout', () => {
    expect(getObjectTypeIcon('alpha/object-layout')).toBeDefined()
  })

  it('returns a React element for unixfs/fs-node', () => {
    expect(getObjectTypeIcon('unixfs/fs-node')).toBeDefined()
  })

  it('returns a React element for git/repo', () => {
    expect(getObjectTypeIcon('git/repo')).toBeDefined()
  })

  it('returns a React element for git/worktree', () => {
    expect(getObjectTypeIcon('git/worktree')).toBeDefined()
  })

  it('returns a React element for canvas', () => {
    expect(getObjectTypeIcon('canvas')).toBeDefined()
  })

  it('returns a React element for unknown type', () => {
    expect(getObjectTypeIcon('unknown/type')).toBeDefined()
  })
})

describe('buildObjectTree', () => {
  it('returns empty array for empty input', () => {
    expect(buildObjectTree([])).toEqual([])
  })

  it('creates flat nodes for single-segment keys', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'readme', objectType: 'unixfs/fs-node' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('readme')
    expect(result[0].name).toBe('readme')
    expect(result[0].data?.objectKey).toBe('readme')
    expect(result[0].data?.objectType).toBe('unixfs/fs-node')
    expect(result[0].data?.isVirtual).toBe(false)
  })

  it('creates hierarchical nodes for slash-delimited keys', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'dir/file', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('dir')
    expect(result[0].name).toBe('dir')
    expect(result[0].data?.isVirtual).toBe(true)
    expect(result[0].data?.objectType).toBe('')
    expect(result[0].children).toHaveLength(1)
    expect(result[0].children![0].id).toBe('dir/file')
    expect(result[0].children![0].name).toBe('file')
    expect(result[0].children![0].data?.objectKey).toBe('dir/file')
    expect(result[0].children![0].data?.objectType).toBe('canvas')
    expect(result[0].children![0].data?.isVirtual).toBe(false)
  })

  it('filters out hidden types', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'settings', objectType: 'space/settings' },
      { objectKey: 'doc', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('doc')
  })

  it('filters out the SpaceSettings block type', () => {
    const objects: WorldContentsObject[] = [
      {
        objectKey: 'settings',
        objectType:
          'github.com/s4wave/spacewave/core/space/world.SpaceSettings',
      },
      { objectKey: 'doc', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('doc')
  })

  it('sorts nodes alphabetically at each level', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'zebra', objectType: 'canvas' },
      { objectKey: 'apple', objectType: 'canvas' },
      { objectKey: 'mango', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(3)
    expect(result[0].name).toBe('apple')
    expect(result[1].name).toBe('mango')
    expect(result[2].name).toBe('zebra')
  })

  it('handles mixed flat and nested keys', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'toplevel', objectType: 'canvas' },
      { objectKey: 'folder/nested', objectType: 'unixfs/fs-node' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(2)
    const folder = result.find((n) => n.name === 'folder')
    const toplevel = result.find((n) => n.name === 'toplevel')
    expect(folder).toBeDefined()
    expect(toplevel).toBeDefined()
    expect(folder!.data?.isVirtual).toBe(true)
    expect(toplevel!.data?.isVirtual).toBe(false)
    expect(folder!.children).toHaveLength(1)
    expect(folder!.children![0].name).toBe('nested')
  })

  it('creates correct hierarchy for deep nesting', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'a/b/c', objectType: 'git/repo' },
    ]
    const result = buildObjectTree(objects)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('a')
    expect(result[0].data?.isVirtual).toBe(true)
    expect(result[0].children).toHaveLength(1)
    expect(result[0].children![0].id).toBe('a/b')
    expect(result[0].children![0].data?.isVirtual).toBe(true)
    expect(result[0].children![0].children).toHaveLength(1)
    expect(result[0].children![0].children![0].id).toBe('a/b/c')
    expect(result[0].children![0].children![0].data?.objectType).toBe(
      'git/repo',
    )
    expect(result[0].children![0].children![0].data?.isVirtual).toBe(false)
  })

  it('uses full key path as node ID', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'dir/file', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result[0].id).toBe('dir')
    expect(result[0].children![0].id).toBe('dir/file')
  })

  it('marks virtual nodes with isVirtual true and no objectType', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'parent/child', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result[0].data?.isVirtual).toBe(true)
    expect(result[0].data?.objectType).toBe('')
  })

  it('marks leaf nodes with isVirtual false and correct objectType', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'parent/child', objectType: 'git/repo' },
    ]
    const result = buildObjectTree(objects)
    const leaf = result[0].children![0]
    expect(leaf.data?.isVirtual).toBe(false)
    expect(leaf.data?.objectType).toBe('git/repo')
    expect(leaf.data?.objectKey).toBe('parent/child')
  })

  it('sorts children alphabetically within nested levels', () => {
    const objects: WorldContentsObject[] = [
      { objectKey: 'dir/zebra', objectType: 'canvas' },
      { objectKey: 'dir/alpha', objectType: 'canvas' },
    ]
    const result = buildObjectTree(objects)
    expect(result[0].children![0].name).toBe('alpha')
    expect(result[0].children![1].name).toBe('zebra')
  })
})
