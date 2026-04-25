import { describe, expect, it } from 'vitest'

import { buildBrowserReleaseDescriptor } from './browser-release.js'

describe('buildBrowserReleaseDescriptor', () => {
  it('builds a deterministic generation descriptor from shell assets and routes', () => {
    const shellAssets = {
      entrypoint: 'entrypoint/abc123/entrypoint.mjs',
      serviceWorker: 'sw-deadbeef.mjs',
      sharedWorker: 'shw-def456.mjs',
      wasm: 'entrypoint/abc123/runtime.wasm',
      css: ['/static/app-1.css', '/static/app-2.css'],
    }

    const descriptor = buildBrowserReleaseDescriptor(
      shellAssets,
      ['/pricing', '/landing', '/pricing', '/'],
      ['/static/app-2.css', '/static/assets/icon.png', '/static/app-2.css'],
    )

    expect(descriptor.schemaVersion).toBe(1)
    expect(descriptor.generationId).toMatch(/^[0-9a-f]{16}$/)
    expect(descriptor.shellAssets).toEqual(shellAssets)
    expect(descriptor.prerenderedRoutes).toEqual(['/', '/landing', '/pricing'])
    expect(descriptor.requiredStaticAssets).toEqual([
      '/static/app-2.css',
      '/static/assets/icon.png',
    ])
  })

  it('normalizes prerendered route order before deriving the generation id', () => {
    const shellAssets = {
      entrypoint: 'entrypoint/abc123/entrypoint.mjs',
      serviceWorker: 'sw-deadbeef.mjs',
      sharedWorker: 'shw-def456.mjs',
      wasm: 'entrypoint/abc123/runtime.wasm',
      css: ['/static/app.css'],
    }

    const a = buildBrowserReleaseDescriptor(
      shellAssets,
      ['/pricing', '/', '/landing'],
      ['/static/assets/icon.png', '/static/app.css'],
    )
    const b = buildBrowserReleaseDescriptor(
      shellAssets,
      ['/landing', '/pricing', '/'],
      ['/static/app.css', '/static/assets/icon.png'],
    )

    expect(a.generationId).toBe(b.generationId)
    expect(a.prerenderedRoutes).toEqual(b.prerenderedRoutes)
    expect(a.requiredStaticAssets).toEqual(b.requiredStaticAssets)
  })
})
