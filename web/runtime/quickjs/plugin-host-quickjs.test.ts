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
        imports: ['chunks/shared-1.mjs'],
        dynamicImports: ['chunks/lazy-2.mjs'],
        css: ['assets/backend.css'],
        assets: ['assets/icon.svg'],
      },
      'plugin/vm/backend.ts': {
        file: 'plugin/vm/backend-def456.mjs',
        imports: ['chunks/shared-1.mjs'],
      },
    })

    expect(paths).toEqual([
      'plugin/notes/backend-abc123.mjs',
      'chunks/shared-1.mjs',
      'chunks/lazy-2.mjs',
      'assets/backend.css',
      'assets/icon.svg',
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
