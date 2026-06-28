import { describe, expect, it } from 'vitest'

import {
  buildOfflineNavigationFallbacks,
  buildReleaseCachePaths,
  createEmptyBrowserReleaseState,
  isBrowserCacheSupportedURL,
  promoteBrowserRelease,
  retainedGenerationIds,
} from './browser-release-state.js'
import type { BrowserReleaseDescriptor } from './browser-release-state.js'

function buildRelease(generationId: string): BrowserReleaseDescriptor {
  return {
    schemaVersion: 1,
    generationId,
    shellAssets: {
      entrypoint: `entrypoint/${generationId}/entrypoint.mjs`,
      serviceWorker: `sw-${generationId}.mjs`,
      sharedWorker: `shw-${generationId}.mjs`,
      opfsWorker: `opfs-worker-${generationId}.mjs`,
      wasm: `entrypoint/${generationId}/runtime.wasm.gz`,
      css: [`entrypoint/${generationId}/entrypoint.css`],
    },
    prerenderedRoutes: ['/', '/pricing', '/blog/launch'],
    requiredStaticAssets: ['/images/favicon.ico', 'images/logo.png'],
  }
}

describe('browser release state helpers', () => {
  it('builds one normalized cache path set for a release', () => {
    const release = buildRelease('gen-a')
    release.prerenderedRoutes.push('/pricing/')
    release.requiredStaticAssets.push('/images/logo.png')

    expect(buildReleaseCachePaths(release)).toEqual([
      '/',
      '/blog/launch',
      '/entrypoint/gen-a/entrypoint.css',
      '/entrypoint/gen-a/entrypoint.mjs',
      '/entrypoint/gen-a/runtime.wasm.gz',
      '/images/favicon.ico',
      '/images/logo.png',
      '/opfs-worker-gen-a.mjs',
      '/pricing',
      '/shw-gen-a.mjs',
      '/sw-gen-a.mjs',
    ])
  })

  it('builds cache paths for a release without a wasm shell asset', () => {
    const release = buildRelease('gen-a')
    delete release.shellAssets.wasm

    expect(buildReleaseCachePaths(release)).not.toContain(
      '/entrypoint/gen-a/runtime.wasm.gz',
    )
    expect(buildReleaseCachePaths(release)).toContain(
      '/entrypoint/gen-a/entrypoint.mjs',
    )
  })

  it('falls back to the promoted root document for non-prerendered routes', () => {
    const release = buildRelease('gen-a')

    expect(buildOfflineNavigationFallbacks('/pricing', release)).toEqual([
      '/pricing',
    ])
    expect(buildOfflineNavigationFallbacks('/session/1', release)).toEqual([
      '/',
    ])
  })

  it('detects URLs supported by the browser Cache API', () => {
    expect(isBrowserCacheSupportedURL('https://example.com/boot.mjs')).toBe(
      true,
    )
    expect(isBrowserCacheSupportedURL('http://localhost:8080/boot.mjs')).toBe(
      true,
    )
    expect(isBrowserCacheSupportedURL('app://index.html/boot.mjs')).toBe(false)
  })

  it('promotes generations without dropping the previous promoted release', () => {
    let state = createEmptyBrowserReleaseState()
    const first = buildRelease('gen-a')
    const second = buildRelease('gen-b')

    state = promoteBrowserRelease(state, first)
    expect(state.promotedCurrent?.generationId).toBe('gen-a')
    expect(state.promotedPrevious).toBeNull()

    state = promoteBrowserRelease(state, second)
    expect(state.promotedCurrent?.generationId).toBe('gen-b')
    expect(state.promotedPrevious?.generationId).toBe('gen-a')
    expect(retainedGenerationIds(state)).toEqual(['gen-b', 'gen-a'])
  })
})
