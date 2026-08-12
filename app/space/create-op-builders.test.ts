import { describe, expect, it } from 'vitest'

import {
  buildForgeObjectKey,
  buildObjectKey,
  buildWizardObjectKey,
  lookupCreateOpBuilder,
} from './create-op-builders.js'

describe('buildObjectKey', () => {
  it('preserves the established numbered convention for non-Forge callers', () => {
    expect(buildObjectKey('canvas/', 'Canvas')).toBe('canvas-1')
    expect(buildObjectKey('canvas/', 'Canvas', ['canvas-1'])).toBe('canvas-2')
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

describe('buildForgeObjectKey', () => {
  it('uses the normalized requested key when it is free', () => {
    expect(buildForgeObjectKey('forge/cluster/', 'cluster-1')).toBe('cluster-1')
    expect(buildForgeObjectKey('forge/job/', 'Build Job')).toBe('build-job')
  })

  it('numbers only a colliding bare key and tolerates historical numbered keys', () => {
    expect(
      buildForgeObjectKey('forge/cluster/', 'cluster', [
        'cluster',
        'cluster-1',
        'cluster-2',
      ]),
    ).toBe('cluster-3')
  })

  it('does not confuse hierarchical graph keys with the bare create key', () => {
    expect(
      buildForgeObjectKey('forge/task/', 'compile', ['forge/task/compile']),
    ).toBe('compile')
  })
})

describe('lookupCreateOpBuilder', () => {
  it('builds the Computers dashboard create op', () => {
    expect(lookupCreateOpBuilder('spacewave/computers/create')).toBeDefined()
  })

  it('does not own plugin-provided notes creation ops', () => {
    expect(lookupCreateOpBuilder('notes/notebook/init')).toBeUndefined()
    expect(lookupCreateOpBuilder('notes/docs/create')).toBeUndefined()
    expect(lookupCreateOpBuilder('notes/blog/create')).toBeUndefined()
  })
})
