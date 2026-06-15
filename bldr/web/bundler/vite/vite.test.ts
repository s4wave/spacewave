import { describe, expect, it } from 'vitest'
import path from 'path'

import { buildStableEntryFileName } from './output-naming.js'
import { buildWebPkgImportSpecifier } from './web-pkg-naming.js'

describe('buildStableEntryFileName', () => {
  it('keeps plugin root entry module paths stable across platform builds', () => {
    const rootDir = path.join(path.sep, 'repo', 'game')

    expect(
      buildStableEntryFileName(
        rootDir,
        path.join(rootDir, 'web', 'entry.tsx'),
        'entry',
      ),
    ).toBe('web/entry.mjs')
  })

  it('uses the chunk name for synthetic entries without content hashes', () => {
    expect(buildStableEntryFileName('/repo/game', null, 'frontend')).toBe(
      'frontend.mjs',
    )
  })
})

describe('buildWebPkgImportSpecifier', () => {
  it('publishes a non-index package root as the bare package specifier', () => {
    expect(
      buildWebPkgImportSpecifier(
        'non-index-root',
        'build/foo.module',
        'build/foo.module',
      ),
    ).toBe('non-index-root')
    expect(
      buildWebPkgImportSpecifier(
        'non-index-root',
        'examples/extra',
        'build/foo.module',
      ),
    ).toBe('non-index-root/examples/extra')
  })
})
