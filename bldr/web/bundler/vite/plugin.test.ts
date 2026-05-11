import { describe, expect, it } from 'vitest'
import path from 'path'
import type { ResolvedConfig } from 'vite'

import { createWebPkgRemapPlugin } from './plugin.js'

describe('createWebPkgRemapPlugin', () => {
  it('falls back to the import specifier when a resolved file only shares the package root prefix', async () => {
    const root = path.join(path.sep, 'repo')
    const pkgRoot = path.join(root, 'node_modules', 'pkg')
    const otherRoot = path.join(root, 'node_modules', 'pkg-extra')
    const plugin = createWebPkgRemapPlugin({ webPkgIDs: ['pkg'] })

    const configResolved = plugin.configResolved
    if (typeof configResolved !== 'function') {
      throw new Error('missing configResolved hook')
    }
    configResolved.call(
      {} as never,
      {
        root,
        resolve: {
          alias: [{ find: 'pkg', replacement: pkgRoot }],
        },
      } as ResolvedConfig,
    )

    const resolveId = plugin.resolveId
    if (typeof resolveId !== 'function') {
      throw new Error('missing resolveId hook')
    }
    const result = await resolveId.call(
      {
        resolve: async () => ({
          id: path.join(otherRoot, 'index.js'),
          external: false,
          meta: {},
          moduleSideEffects: true,
        }),
      } as never,
      'pkg/index.js',
      path.join(root, 'src', 'entry.js'),
      { isEntry: false },
    )

    expect(result).toEqual({ id: '/b/pkg/pkg/index.mjs', external: true })
  })
})
