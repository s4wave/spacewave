import { describe, expect, it } from 'vitest'

import { buildObjectKey, buildWizardObjectKey } from './create-op-builders.js'

describe('buildObjectKey', () => {
  it('uses simple name-based numbered keys', () => {
    expect(buildObjectKey('canvas/', 'Canvas')).toBe('canvas-1')
    expect(buildObjectKey('forge/cluster/', 'Forge Cluster')).toBe(
      'forge-cluster-1',
    )
  })

  it('selects the next available numbered key', () => {
    expect(buildObjectKey('canvas/', 'Canvas', ['canvas-1'])).toBe('canvas-2')
  })

  it('uses the prefix only when the name is empty', () => {
    expect(buildObjectKey('object-layout/', '')).toBe('object-layout-1')
  })

  it('uses the wizard prefix without coupling to the wizard type id', () => {
    expect(buildWizardObjectKey('Git Repository')).toBe(
      'wizard/git-repository-1',
    )
    expect(
      buildWizardObjectKey('Git Repository', ['wizard/git-repository-1']),
    ).toBe('wizard/git-repository-2')
  })
})
