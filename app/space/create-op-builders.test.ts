import { describe, expect, it } from 'vitest'
import { keysWorld } from '@s4wave/web/test/world-query.js'

import {
  buildForgeObjectKey,
  buildObjectKey,
  buildWizardObjectKey,
  lookupCreateOpBuilder,
} from './create-op-builders.js'

describe('buildObjectKey', () => {
  it('preserves the established numbered convention for non-Forge callers', async () => {
    expect(await buildObjectKey(keysWorld([]), 'canvas/', 'Canvas')).toBe(
      'canvas-1',
    )
    expect(
      await buildObjectKey(
        keysWorld(['canvas-1', 'canvas-2', 'canvas-3', 'canvas-x']),
        'canvas/',
        'Canvas',
      ),
    ).toBe('canvas-4')
    expect(await buildObjectKey(keysWorld([]), 'object-layout/', '')).toBe(
      'object-layout-1',
    )
  })

  it('uses the wizard prefix without coupling to the wizard type id', async () => {
    expect(await buildWizardObjectKey(keysWorld([]), 'Git Repository')).toBe(
      'wizard/git-repository-1',
    )
    expect(
      await buildWizardObjectKey(
        keysWorld(['wizard/git-repository-1']),
        'Git Repository',
      ),
    ).toBe('wizard/git-repository-2')
  })
})

describe('buildForgeObjectKey', () => {
  it('uses the normalized requested key when it is free', async () => {
    expect(
      await buildForgeObjectKey(keysWorld([]), 'forge/cluster/', 'cluster-1'),
    ).toBe('cluster-1')
    expect(
      await buildForgeObjectKey(keysWorld([]), 'forge/job/', 'Build Job'),
    ).toBe('build-job')
  })

  it('numbers only a colliding bare key and tolerates historical numbered keys', async () => {
    expect(
      await buildForgeObjectKey(
        keysWorld(['cluster', 'cluster-1', 'cluster-2']),
        'forge/cluster/',
        'cluster',
      ),
    ).toBe('cluster-3')
  })

  it('does not confuse hierarchical graph keys with the bare create key', async () => {
    expect(
      await buildForgeObjectKey(
        keysWorld(['forge/task/compile']),
        'forge/task/',
        'compile',
      ),
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
