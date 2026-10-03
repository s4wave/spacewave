import { describe, it, expect, beforeEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import { LuBot, LuBox, LuGitBranch, LuPaintbrush } from 'react-icons/lu'
import {
  ObjectTypeVisibility,
  type ObjectTypeRegistration,
} from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import {
  buildObjectTypeMetadataMap,
  buildObjectLevel,
  HIDDEN_OBJECT_TYPES,
  getObjectTypeLabel,
  getObjectTypeIcon,
  getObjectTypeIconComponent,
  getObjectDisplayName,
  isHiddenSpaceObject,
  listObjectLevel,
  listSpaceObjectTargets,
} from '@s4wave/web/space/object-tree.js'
import { listingWorld } from '@s4wave/web/test/world-query.js'

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

  it('hides object types marked hidden or internal by metadata', () => {
    const metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'plugin/internal',
        registrationId: 1,
        metadata: { visibility: ObjectTypeVisibility.INTERNAL },
      },
      {
        typeId: 'plugin/hidden',
        registrationId: 2,
        metadata: { visibility: ObjectTypeVisibility.HIDDEN },
      },
    ])
    expect(isHiddenSpaceObject('internal', 'plugin/internal', metadata)).toBe(
      true,
    )
    expect(isHiddenSpaceObject('hidden', 'plugin/hidden', metadata)).toBe(true)
  })
})

describe('getObjectTypeLabel', () => {
  it('returns registered display metadata before static fallbacks', () => {
    const metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'canvas',
        registrationId: 1,
        metadata: { displayName: 'Board' },
      },
    ])
    expect(getObjectTypeLabel('canvas', metadata)).toBe('Board')
  })

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

describe('getObjectTypeIconComponent', () => {
  it('prefers registered glyphs and keeps built-in and fallback glyphs distinct', () => {
    const metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'canvas',
        registrationId: 1,
        metadata: { iconName: 'bot' },
      },
    ])

    expect(getObjectTypeIconComponent('canvas', metadata)).toBe(LuBot)
    expect(getObjectTypeIconComponent('canvas')).toBe(LuPaintbrush)
    expect(getObjectTypeIconComponent('git/repo')).toBe(LuGitBranch)
    expect(getObjectTypeIconComponent('unknown/type')).toBe(LuBox)
  })
})

describe('getObjectTypeIcon', () => {
  it('returns registered icons before static fallbacks', () => {
    const metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'canvas',
        registrationId: 1,
        metadata: { iconName: 'bot' },
      },
    ])
    expect(getObjectTypeIcon('canvas', metadata)).toBeDefined()
  })

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

describe('getObjectDisplayName', () => {
  it('uses product labels for built-in object keys', () => {
    expect(getObjectDisplayName('unixfs')).toBe('Files')
    expect(getObjectDisplayName('object-layout')).toBe('Layout')
  })

  it('humanizes the final object-key segment for generated keys', () => {
    expect(getObjectDisplayName('gizmo/bootstrap/llm-session')).toBe(
      'Llm Session',
    )
  })
})

describe('buildObjectTypeMetadataMap', () => {
  it('keeps the first registration for a type by registration ID', () => {
    const registrations: ObjectTypeRegistration[] = [
      {
        typeId: 'plugin/object',
        registrationId: 2,
        metadata: { displayName: 'Second' },
      },
      {
        typeId: 'plugin/object',
        registrationId: 1,
        metadata: { displayName: 'First' },
      },
    ]
    const metadata = buildObjectTypeMetadataMap(registrations)
    expect(metadata.get('plugin/object')?.displayName).toBe('First')
  })
})

describe('buildObjectLevel', () => {
  it('returns empty array for empty input', () => {
    expect(buildObjectLevel([], [])).toEqual([])
  })

  it('creates leaf nodes for listed objects', () => {
    const result = buildObjectLevel(
      [{ objectKey: 'readme', typeId: 'unixfs/fs-node' }],
      [],
    )
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('readme')
    expect(result[0].name).toBe('readme')
    expect(result[0].hasChildren).toBeUndefined()
    expect(result[0].data?.objectKey).toBe('readme')
    expect(result[0].data?.objectType).toBe('unixfs/fs-node')
    expect(result[0].data?.objectTypeLabel).toBe('File System')
    expect(result[0].detail).toBe('File System')
    expect(result[0].data?.isVirtual).toBe(false)
  })

  it('creates virtual folders for key groups', () => {
    const result = buildObjectLevel([], ['dir/sub/'])
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('dir/sub')
    expect(result[0].name).toBe('sub')
    expect(result[0].hasChildren).toBe(true)
    expect(result[0].data?.isVirtual).toBe(true)
    expect(result[0].data?.objectKey).toBe('dir/sub')
    expect(result[0].data?.objectType).toBe('')
    expect(result[0].detail).toBe('')
  })

  it('merges an object with the group of keys below it', () => {
    const result = buildObjectLevel(
      [{ objectKey: 'dir', typeId: 'canvas' }],
      ['dir/'],
    )
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('dir')
    expect(result[0].hasChildren).toBe(true)
    expect(result[0].data?.isVirtual).toBe(false)
    expect(result[0].data?.objectType).toBe('canvas')
  })

  it('filters out hidden types', () => {
    const result = buildObjectLevel(
      [
        { objectKey: 'settings', typeId: 'space/settings' },
        {
          objectKey: 'settings-block',
          typeId: 'github.com/s4wave/spacewave/core/space/world.SpaceSettings',
        },
        { objectKey: 'doc', typeId: 'canvas' },
      ],
      [],
    )
    expect(result.map((node) => node.id)).toEqual(['doc'])
  })

  it('uses metadata labels and descriptions in node data', () => {
    const metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'gizmo/operator-home',
        registrationId: 1,
        metadata: {
          displayName: 'Gizmo Home',
          description: 'Operator command surface.',
        },
      },
    ])
    const result = buildObjectLevel(
      [{ objectKey: 'gizmo-home', typeId: 'gizmo/operator-home' }],
      [],
      metadata,
    )
    expect(result[0].name).toBe('Gizmo Home')
    expect(result[0].detail).toBe('Gizmo Home')
    expect(result[0].data?.objectTypeLabel).toBe('Gizmo Home')
    expect(result[0].data?.objectTypeDescription).toBe(
      'Operator command surface.',
    )
  })

  it('filters objects hidden by registered metadata', () => {
    const metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'plugin/internal',
        registrationId: 1,
        metadata: { visibility: ObjectTypeVisibility.INTERNAL },
      },
    ])
    const result = buildObjectLevel(
      [
        { objectKey: 'internal', typeId: 'plugin/internal' },
        { objectKey: 'doc', typeId: 'canvas' },
      ],
      [],
      metadata,
    )
    expect(result.map((node) => node.id)).toEqual(['doc'])
  })

  it('sorts objects and folders together by key', () => {
    const result = buildObjectLevel(
      [
        { objectKey: 'zebra', typeId: 'canvas' },
        { objectKey: 'apple', typeId: 'canvas' },
      ],
      ['mango/'],
    )
    expect(result.map((node) => node.id)).toEqual(['apple', 'mango', 'zebra'])
  })
})

describe('listObjectLevel', () => {
  const objects = [
    { objectKey: 'a', objectType: 'canvas' },
    { objectKey: 'a/x', objectType: 'canvas' },
    { objectKey: 'b', objectType: 'canvas' },
    { objectKey: 'c/deep/key', objectType: 'canvas' },
    { objectKey: 'd', objectType: 'canvas' },
  ]

  it('reads one level across listing pages', async () => {
    const level = await listObjectLevel(listingWorld(objects), '', 100)
    expect(level.more).toBe(false)
    expect(level.nodes.map((node) => [node.id, !!node.hasChildren])).toEqual([
      ['a', true],
      ['b', false],
      ['c', true],
      ['d', false],
    ])
  })

  it('reads a nested level under its prefix', async () => {
    const level = await listObjectLevel(listingWorld(objects), 'c/', 100)
    expect(level.nodes.map((node) => node.id)).toEqual(['c/deep'])
    expect(level.nodes[0].data?.isVirtual).toBe(true)
  })

  it('stops at the limit and reports more entries', async () => {
    const level = await listObjectLevel(listingWorld(objects), '', 3)
    expect(level.more).toBe(true)
    expect(level.nodes.map((node) => node.id)).toEqual(['a', 'b'])
  })
})

describe('listSpaceObjectTargets', () => {
  it('skips hidden and internal objects across pages', async () => {
    const metadataById = buildObjectTypeMetadataMap([
      {
        typeId: 'plugin/internal',
        registrationId: 1,
        metadata: { visibility: ObjectTypeVisibility.INTERNAL },
      },
    ])
    const world = listingWorld([
      { objectKey: 'a-internal', objectType: 'plugin/internal' },
      { objectKey: 'b-doc', objectType: 'canvas' },
      { objectKey: 'settings', objectType: 'space/settings' },
      { objectKey: 'z-doc', objectType: 'canvas' },
    ])

    const { targets, more } = await listSpaceObjectTargets(world, metadataById)
    expect(targets.map((target) => target.objectKey)).toEqual([
      'b-doc',
      'z-doc',
    ])
    expect(more).toBe(false)
  })

  it('stops at the target limit and reports more', async () => {
    const objects = Array.from({ length: 150 }, (_, i) => ({
      objectKey: `doc-${String(i).padStart(3, '0')}`,
      objectType: 'canvas',
    }))

    const { targets, more } = await listSpaceObjectTargets(
      listingWorld(objects, 1000),
    )
    expect(targets).toHaveLength(100)
    expect(targets.at(-1)?.objectKey).toBe('doc-099')
    expect(more).toBe(true)
  })
})
