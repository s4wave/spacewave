import { describe, expect, it } from 'vitest'

import {
  addAssetToFileSystem,
  collectViteManifestAssetPaths,
  resolveBackendAssetPath,
} from './plugin-host-quickjs.js'

describe('plugin-host-quickjs asset helpers', () => {
  it('collects unique vite manifest asset paths across entry fields', () => {
    const paths = collectViteManifestAssetPaths({
      'plugin/notes/backend.ts': {
        file: 'plugin/notes/backend-abc123.mjs',
        imports: ['_chunk-shared-1.mjs'],
        dynamicImports: ['_chunk-lazy-2.mjs'],
        css: ['assets/backend.css'],
        assets: ['assets/icon.svg'],
      },
      '_chunk-shared-1.mjs': {
        file: 'chunks/shared-1.mjs',
      },
      '_chunk-lazy-2.mjs': {
        file: 'chunks/lazy-2.mjs',
      },
      'plugin/vm/backend.ts': {
        file: 'plugin/vm/backend-def456.mjs',
        imports: ['_chunk-shared-1.mjs'],
      },
    })

    expect(paths).toEqual([
      'plugin/notes/backend-abc123.mjs',
      'assets/backend.css',
      'assets/icon.svg',
      'chunks/shared-1.mjs',
      'chunks/lazy-2.mjs',
      'plugin/vm/backend-def456.mjs',
    ])
  })

  it('normalizes backend asset paths into the v/b/be tree', () => {
    expect(resolveBackendAssetPath('plugin/notes/backend-abc123.mjs')).toBe(
      'v/b/be/plugin/notes/backend-abc123.mjs',
    )
    expect(resolveBackendAssetPath('b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'v/b/be/plugin/notes/backend-abc123.mjs',
    )
    expect(resolveBackendAssetPath('v/b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'v/b/be/plugin/notes/backend-abc123.mjs',
    )
  })

  it('mirrors assets under both asset-relative and /assets paths', () => {
    const files = new Map<string, string | Uint8Array>()

    addAssetToFileSystem(
      files,
      'plugin/notes/backend-abc123.mjs',
      'export default {}',
    )

    expect(files.get('v/b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'export default {}',
    )
    expect(files.get('/assets/v/b/be/plugin/notes/backend-abc123.mjs')).toBe(
      'export default {}',
    )
  })
})
