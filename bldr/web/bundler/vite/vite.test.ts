import { describe, expect, it } from 'vitest'
import path from 'path'

import { buildStableEntryFileName } from './output-naming.js'
import {
  buildWebPkgImportSpecifier,
  servedEntryName,
  specifierEntryNames,
} from './web-pkg-naming.js'

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
        false,
      ),
    ).toBe('non-index-root')
    expect(
      buildWebPkgImportSpecifier(
        'non-index-root',
        'examples/extra',
        'build/foo.module',
        false,
      ),
    ).toBe('non-index-root/examples/extra')
  })

  it('publishes TypeScript entries at their emitted .js path', () => {
    expect(
      buildWebPkgImportSpecifier('@aptre/bldr-sdk', 'index', null, true),
    ).toBe('@aptre/bldr-sdk')
    expect(
      buildWebPkgImportSpecifier(
        '@aptre/bldr-sdk',
        'resource/resource.pb',
        null,
        true,
      ),
    ).toBe('@aptre/bldr-sdk/resource/resource.pb.js')
  })
})

describe('servedEntryName', () => {
  it('keeps TypeScript proto modules distinct from their siblings', () => {
    expect(servedEntryName('resource/resource.ts')).toBe('resource/resource')
    expect(servedEntryName('./resource/resource.pb.ts')).toBe(
      'resource/resource.pb',
    )
  })

  it('names compiled proto modules by export subpath', () => {
    expect(servedEntryName('google/protobuf/timestamp.pb.js')).toBe(
      'google/protobuf/timestamp',
    )
  })
})

describe('specifierEntryNames', () => {
  it('tries the TypeScript entry before the compiled entry', () => {
    expect(specifierEntryNames('object/object.pb.js')).toEqual([
      'object/object.pb',
      'object/object',
    ])
    expect(specifierEntryNames('message')).toEqual(['message'])
  })
})
