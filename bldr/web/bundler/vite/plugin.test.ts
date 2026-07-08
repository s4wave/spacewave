import { describe, expect, it } from 'vitest'
import path from 'path'
import os from 'os'
import fs from 'fs'
import type { ResolvedConfig } from 'vite'

import { createWebPkgRemapPlugin, readPackageRootServedName } from './plugin.js'

describe('createWebPkgRemapPlugin', () => {
  it('preserves runtime externals and rewrites sibling web packages in rendered chunks', () => {
    const plugin = createWebPkgRemapPlugin({
      webPkgIDs: [
        '@aptre/bldr',
        '@aptre/bldr-react',
        '@aptre/protobuf-es-lite',
        '@s4wave/web',
        'react',
        'react-dom',
        'sonner',
      ],
      preserveWebPkgIDs: [
        '@aptre/bldr',
        '@aptre/bldr-react',
        'react',
        'react-dom',
      ],
    })

    const renderChunk = plugin.renderChunk
    if (typeof renderChunk !== 'function') {
      throw new Error('missing renderChunk hook')
    }

    const result = renderChunk.call(
      {} as never,
      [
        'import "@aptre/bldr";',
        'import { WebView } from "@aptre/bldr-react";',
        'import React from "react";',
        'import { createPortal } from "react-dom";',
        'import { jsxDEV } from "react/jsx-dev-runtime";',
        'import { createMessageType } from "@aptre/protobuf-es-lite";',
        'import { Message } from "@aptre/protobuf-es-lite/message";',
        'import { SpacewaveRuntimeProviders } from "@s4wave/web/sdk/app";',
        'import { toast } from "sonner";',
      ].join('\n'),
      {} as never,
      {} as never,
      {} as never,
    )

    expect(result).toContain('"@aptre/bldr"')
    expect(result).toContain('"@aptre/bldr-react"')
    expect(result).toContain('"react"')
    expect(result).toContain('"react-dom"')
    expect(result).toContain('"react/jsx-dev-runtime"')
    expect(result).toContain('"/b/pkg/@aptre/protobuf-es-lite/index.mjs"')
    expect(result).toContain('"/b/pkg/@aptre/protobuf-es-lite/message.mjs"')
    expect(result).toContain('"/b/pkg/@s4wave/web/sdk/app.mjs"')
    expect(result).toContain('"/b/pkg/sonner/index.mjs"')
    expect(result).not.toMatch(
      /\b(?:from|import)\s*\(?\s*["']@aptre\/protobuf-es-lite(?:\/[^"']*)?["']/,
    )
    expect(result).not.toContain('"/b/pkg/react/')
    expect(result).not.toContain('"/b/pkg/react-dom/')
    expect(result).not.toContain('"/b/pkg/@aptre/bldr/')
  })

  it('uses the configured web package base path for rendered sibling web packages', () => {
    const plugin = createWebPkgRemapPlugin({
      webPkgIDs: ['@aptre/protobuf-es-lite', '@s4wave/web', 'sonner'],
      webPkgImports: {
        '@aptre/protobuf-es-lite': ['index.js', 'message.js'],
      },
      webPkgBasePath: '/entrypoint/pkgs',
    })

    const renderChunk = plugin.renderChunk
    if (typeof renderChunk !== 'function') {
      throw new Error('missing renderChunk hook')
    }

    const result = renderChunk.call(
      {} as never,
      [
        'import { createMessageType } from "@aptre/protobuf-es-lite";',
        'import { Message } from "@aptre/protobuf-es-lite/message";',
        'import { SpacewaveRuntimeProviders } from "@s4wave/web/sdk/app";',
        'import { toast } from "sonner";',
      ].join('\n'),
      {} as never,
      {} as never,
      {} as never,
    )

    expect(result).toContain(
      '"/entrypoint/pkgs/@aptre/protobuf-es-lite/index.mjs"',
    )
    expect(result).toContain(
      '"/entrypoint/pkgs/@aptre/protobuf-es-lite/message.mjs"',
    )
    expect(result).toContain('"/entrypoint/pkgs/@s4wave/web/sdk/app.mjs"')
    expect(result).toContain('"/entrypoint/pkgs/sonner/index.mjs"')
    expect(result).not.toContain('"/b/pkg/@aptre/protobuf-es-lite/')
    expect(result).not.toContain('"/b/pkg/@s4wave/web/')
    expect(result).not.toContain('"/b/pkg/sonner/')
  })

  it('normalizes repeated slashes in the configured web package base path', () => {
    const plugin = createWebPkgRemapPlugin({
      webPkgIDs: ['react'],
      webPkgBasePath: '/entrypoint//pkgs/',
    })

    const renderChunk = plugin.renderChunk
    if (typeof renderChunk !== 'function') {
      throw new Error('missing renderChunk hook')
    }

    const result = renderChunk.call(
      {} as never,
      'import React from "react";',
      {} as never,
      {} as never,
      {} as never,
    )

    expect(result).toContain('"/entrypoint/pkgs/react/index.mjs"')
    expect(result).not.toContain('/entrypoint//pkgs/')
  })

  it('derives served names from declared imports, stripping dist subdir and .pb extension', async () => {
    const plugin = createWebPkgRemapPlugin({
      webPkgIDs: ['@aptre/protobuf-es-lite'],
      webPkgImports: {
        '@aptre/protobuf-es-lite': [
          'index.js',
          'message.js',
          'google/protobuf/timestamp.pb.js',
        ],
      },
    })

    const renderChunk = plugin.renderChunk
    if (typeof renderChunk !== 'function') {
      throw new Error('missing renderChunk hook')
    }
    const rendered = renderChunk.call(
      {} as never,
      [
        'import { createMessageType } from "@aptre/protobuf-es-lite";',
        'import { Message } from "@aptre/protobuf-es-lite/message";',
        'import { Timestamp } from "@aptre/protobuf-es-lite/google/protobuf/timestamp";',
      ].join('\n'),
      {} as never,
      {} as never,
      {} as never,
    )
    expect(rendered).toContain('"/b/pkg/@aptre/protobuf-es-lite/index.mjs"')
    expect(rendered).toContain('"/b/pkg/@aptre/protobuf-es-lite/message.mjs"')
    expect(rendered).toContain(
      '"/b/pkg/@aptre/protobuf-es-lite/google/protobuf/timestamp.mjs"',
    )
    expect(rendered).not.toContain('/dist/')
    expect(rendered).not.toContain('.pb.mjs')

    const resolveId = plugin.resolveId
    if (typeof resolveId !== 'function') {
      throw new Error('missing resolveId hook')
    }
    // A declared import resolves to the served name without touching on-disk
    // layout, even if the resolver would have returned a dist/.pb.js path.
    const resolved = await resolveId.call(
      {
        resolve: async () => {
          throw new Error(
            'on-disk resolution must not run for declared imports',
          )
        },
      } as never,
      '@aptre/protobuf-es-lite/google/protobuf/timestamp.pb.js',
      '/repo/src/entry.js',
      { isEntry: false },
    )
    expect(resolved).toEqual({
      id: '/b/pkg/@aptre/protobuf-es-lite/google/protobuf/timestamp.mjs',
      external: true,
    })
  })

  it('maps a dist mjs package root export to the served index URL', async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'web-pkg-dist-mjs-'))
    try {
      const pkgRoot = path.join(root, 'node_modules', 'root-output-pkg')
      fs.mkdirSync(path.join(pkgRoot, 'dist'), { recursive: true })
      fs.writeFileSync(
        path.join(pkgRoot, 'package.json'),
        JSON.stringify({
          name: 'root-output-pkg',
          type: 'module',
          exports: { '.': { import: './dist/index.mjs' } },
        }),
      )
      fs.writeFileSync(
        path.join(pkgRoot, 'dist', 'index.mjs'),
        'export const root = true\n',
      )

      expect(readPackageRootServedName(pkgRoot)).toBe('index')

      const reportedRoots: Array<[string, string]> = []
      const plugin = createWebPkgRemapPlugin({
        webPkgIDs: ['root-output-pkg'],
        webPkgBasePath: '/entrypoint/pkgs',
        addWebPkgRoot: (webPkgID, webPkgRoot) => {
          reportedRoots.push([webPkgID, webPkgRoot])
        },
      })

      const configResolved = plugin.configResolved
      if (typeof configResolved !== 'function') {
        throw new Error('missing configResolved hook')
      }
      configResolved.call(
        {} as never,
        {
          root,
          resolve: {
            alias: [{ find: 'root-output-pkg', replacement: pkgRoot }],
          },
        } as ResolvedConfig,
      )
      expect(reportedRoots).toEqual([['root-output-pkg', pkgRoot]])

      const renderChunk = plugin.renderChunk
      if (typeof renderChunk !== 'function') {
        throw new Error('missing renderChunk hook')
      }
      const rendered = renderChunk.call(
        {} as never,
        'import { root as pkgRoot } from "root-output-pkg";',
        {} as never,
        {} as never,
        {} as never,
      )
      expect(rendered).toContain('"/entrypoint/pkgs/root-output-pkg/index.mjs"')
      expect(rendered).not.toContain('/dist/')

      const resolveId = plugin.resolveId
      if (typeof resolveId !== 'function') {
        throw new Error('missing resolveId hook')
      }
      const resolved = await resolveId.call(
        {
          resolve: async () => {
            throw new Error(
              'root export served map must not hit on-disk resolution',
            )
          },
        } as never,
        'root-output-pkg',
        path.join(root, 'src', 'entry.js'),
        { isEntry: false },
      )
      expect(resolved).toEqual({
        id: '/entrypoint/pkgs/root-output-pkg/index.mjs',
        external: true,
      })
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  })

  it('reports alias entry-file replacements as package roots', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'web-pkg-alias-file-'))
    try {
      const existingPkgRoot = path.join(root, 'packages', 'entry-file-pkg')
      const bldrPkgRoot = path.join(root, 'bldr', 'web', 'bldr')
      fs.mkdirSync(existingPkgRoot, { recursive: true })
      fs.mkdirSync(bldrPkgRoot, { recursive: true })
      fs.writeFileSync(
        path.join(existingPkgRoot, 'index.js'),
        'export const root = true\n',
      )
      fs.writeFileSync(
        path.join(bldrPkgRoot, 'index.ts'),
        'export const root = true\n',
      )

      const reportedRoots: Array<[string, string]> = []
      const plugin = createWebPkgRemapPlugin({
        webPkgIDs: ['entry-file-pkg', '@aptre/bldr'],
        addWebPkgRoot: (webPkgID, webPkgRoot) => {
          reportedRoots.push([webPkgID, webPkgRoot])
        },
      })

      const configResolved = plugin.configResolved
      if (typeof configResolved !== 'function') {
        throw new Error('missing configResolved hook')
      }
      configResolved.call(
        {} as never,
        {
          root,
          resolve: {
            alias: [
              {
                find: 'entry-file-pkg',
                replacement: path.join(existingPkgRoot, 'index.js'),
              },
              {
                find: '@aptre/bldr',
                replacement: path.join(bldrPkgRoot, 'index.js'),
              },
            ],
          },
        } as ResolvedConfig,
      )

      expect(reportedRoots).toEqual([
        ['entry-file-pkg', existingPkgRoot],
        ['@aptre/bldr', bldrPkgRoot],
      ])
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  })

  it('maps a dist root export to the served index URL without declared imports', async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'web-pkg-dist-'))
    try {
      const pkgRoot = path.join(
        root,
        'node_modules',
        '@aptre',
        'protobuf-es-lite',
      )
      fs.mkdirSync(path.join(pkgRoot, 'dist'), { recursive: true })
      // exports["."] points at ./dist/index.js. With no declared webPkgImports
      // map, the package root export still serves as index.mjs, not dist/index.mjs.
      fs.writeFileSync(
        path.join(pkgRoot, 'package.json'),
        JSON.stringify({
          name: '@aptre/protobuf-es-lite',
          type: 'module',
          exports: { '.': { import: './dist/index.js' } },
        }),
      )
      fs.writeFileSync(
        path.join(pkgRoot, 'dist', 'index.js'),
        'export const root = true\n',
      )

      const plugin = createWebPkgRemapPlugin({
        webPkgIDs: ['@aptre/protobuf-es-lite'],
        webPkgBasePath: '/entrypoint/pkgs',
      })

      const configResolved = plugin.configResolved
      if (typeof configResolved !== 'function') {
        throw new Error('missing configResolved hook')
      }
      configResolved.call(
        {} as never,
        {
          root,
          resolve: {
            alias: [{ find: '@aptre/protobuf-es-lite', replacement: pkgRoot }],
          },
        } as ResolvedConfig,
      )

      const renderChunk = plugin.renderChunk
      if (typeof renderChunk !== 'function') {
        throw new Error('missing renderChunk hook')
      }
      const rendered = renderChunk.call(
        {} as never,
        'import { createMessageType } from "@aptre/protobuf-es-lite";',
        {} as never,
        {} as never,
        {} as never,
      )
      expect(rendered).toContain(
        '"/entrypoint/pkgs/@aptre/protobuf-es-lite/index.mjs"',
      )
      expect(rendered).not.toContain('/dist/')
      expect(rendered).not.toContain('"/b/pkg/@aptre/protobuf-es-lite/')

      const resolveId = plugin.resolveId
      if (typeof resolveId !== 'function') {
        throw new Error('missing resolveId hook')
      }
      const resolved = await resolveId.call(
        {
          resolve: async () => {
            throw new Error(
              'root export served map must not hit on-disk resolution',
            )
          },
        } as never,
        '@aptre/protobuf-es-lite',
        path.join(root, 'src', 'entry.js'),
        { isEntry: false },
      )
      expect(resolved).toEqual({
        id: '/entrypoint/pkgs/@aptre/protobuf-es-lite/index.mjs',
        external: true,
      })
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  })

  it('maps a bare package import to a non-index package root without clobbering declared imports', async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'web-pkg-root-'))
    try {
      const pkgRoot = path.join(root, 'packages', 'non-index-root')
      fs.mkdirSync(path.join(pkgRoot, 'build'), { recursive: true })
      fs.mkdirSync(path.join(pkgRoot, 'examples'), { recursive: true })
      fs.writeFileSync(
        path.join(pkgRoot, 'package.json'),
        JSON.stringify({
          name: 'non-index-root',
          type: 'module',
          exports: {
            '.': {
              import: './build/foo.module.js',
            },
          },
          module: './build/foo.module.js',
        }),
      )
      fs.writeFileSync(
        path.join(pkgRoot, 'build', 'foo.module.js'),
        'export const root = true\n',
      )
      fs.writeFileSync(
        path.join(pkgRoot, 'examples', 'extra.js'),
        'export const extra = true\n',
      )

      const plugin = createWebPkgRemapPlugin({
        webPkgIDs: ['non-index-root'],
        webPkgImports: {
          'non-index-root': ['examples/extra.js'],
        },
      })

      const configResolved = plugin.configResolved
      if (typeof configResolved !== 'function') {
        throw new Error('missing configResolved hook')
      }
      configResolved.call(
        {} as never,
        {
          root,
          resolve: {
            alias: [{ find: 'non-index-root', replacement: pkgRoot }],
          },
        } as ResolvedConfig,
      )

      const renderChunk = plugin.renderChunk
      if (typeof renderChunk !== 'function') {
        throw new Error('missing renderChunk hook')
      }
      const rendered = renderChunk.call(
        {} as never,
        [
          'import * as root from "non-index-root";',
          'import { extra } from "non-index-root/examples/extra.js";',
        ].join('\n'),
        {} as never,
        {} as never,
        {} as never,
      )

      expect(rendered).toContain('"/b/pkg/non-index-root/build/foo.module.mjs"')
      expect(rendered).toContain('"/b/pkg/non-index-root/examples/extra.mjs"')
      expect(rendered).not.toContain('"/b/pkg/non-index-root/index.mjs"')

      const resolveId = plugin.resolveId
      if (typeof resolveId !== 'function') {
        throw new Error('missing resolveId hook')
      }
      const resolvedRoot = await resolveId.call(
        {
          resolve: async () => {
            throw new Error(
              'root export served map must not hit on-disk resolution',
            )
          },
        } as never,
        'non-index-root',
        path.join(root, 'src', 'entry.js'),
        { isEntry: false },
      )
      expect(resolvedRoot).toEqual({
        id: '/b/pkg/non-index-root/build/foo.module.mjs',
        external: true,
      })

      const resolvedExtra = await resolveId.call(
        {
          resolve: async () => {
            throw new Error('declared import must not hit on-disk resolution')
          },
        } as never,
        'non-index-root/examples/extra.js',
        path.join(root, 'src', 'entry.js'),
        { isEntry: false },
      )
      expect(resolvedExtra).toEqual({
        id: '/b/pkg/non-index-root/examples/extra.mjs',
        external: true,
      })
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  })

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
