import { describe, expect, it } from 'vitest'

import { buildObjectKey } from './create-op-builders.js'

describe('buildObjectKey', () => {
  it('uses simple type-based numbered keys', () => {
    expect(buildObjectKey('canvas/', 'Canvas')).toBe('canvas-1')
    expect(buildObjectKey('forge/cluster/', 'Forge Cluster')).toBe('cluster-1')
  })

  it('selects the next available numbered key', () => {
    expect(buildObjectKey('canvas/', 'Canvas', ['canvas-1'])).toBe('canvas-2')
  })
})
