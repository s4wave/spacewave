import { describe, expect, it } from 'vitest'
import path from 'path'
import type { ResolvedConfig } from 'vite'

import { createWebPkgRemapPlugin } from './plugin.js'

describe('createWebPkgRemapPlugin', () => {
  it('preserves runtime externals and rewrites downstream web packages in rendered chunks', () => {
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
        '@aptre/protobuf-es-lite',
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
    expect(result).toContain('"@aptre/protobuf-es-lite/message"')
    expect(result).toContain('"/b/pkg/@s4wave/web/sdk/app.mjs"')
    expect(result).toContain('"/b/pkg/sonner/index.mjs"')
    expect(result).not.toContain('"/b/pkg/react/')
    expect(result).not.toContain('"/b/pkg/react-dom/')
    expect(result).not.toContain('"/b/pkg/@aptre/bldr/')
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
          throw new Error('on-disk resolution must not run for declared imports')
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
