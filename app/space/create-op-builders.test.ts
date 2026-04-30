import { describe, expect, it } from 'vitest'

import { buildObjectKey } from './create-op-builders.js'

describe('buildObjectKey', () => {
  it('uses simple type-based numbered keys', () => {
    expect(buildObjectKey('canvas/', 'Canvas')).toBe('canvas/canvas-1')
    expect(buildObjectKey('forge/cluster/', 'Forge Cluster')).toBe(
      'forge/cluster/cluster-1',
    )
  })

  it('selects the next available numbered key', () => {
    expect(buildObjectKey('canvas/', 'Canvas', ['canvas/canvas-1'])).toBe(
      'canvas/canvas-2',
    )
  })

  it('keeps strict object layout keys under the object-layout prefix', () => {
    expect(buildObjectKey('object-layout/', 'Layout')).toBe(
      'object-layout/object-layout-1',
    )
  })
})
