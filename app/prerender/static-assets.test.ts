import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'fs'
import { join } from 'path'
import { tmpdir } from 'os'

import { afterEach, describe, expect, it } from 'vitest'

import {
  collectRequiredStaticAssetUrls,
  selectAppCssFile,
  selectViteEntryAssets,
  type ViteManifest,
} from './static-assets.js'

let dir = ''

afterEach(() => {
  if (dir) {
    rmSync(dir, { recursive: true, force: true })
    dir = ''
  }
})

describe('collectRequiredStaticAssetUrls', () => {
  it('keeps prerender assets and the hydrate entrypoint but excludes split JS chunks', () => {
    dir = mkdtempSync(join(tmpdir(), 'spacewave-prerender-assets-'))
    mkdirSync(join(dir, 'assets'))
    writeFileSync(join(dir, 'hydrate-abc123.js'), '')
    writeFileSync(join(dir, 'App-abc123.css'), '')
    writeFileSync(join(dir, 'assets', 'spacewave-icon-abc123.png'), '')
    writeFileSync(join(dir, 'assets', 'font-abc123.woff2'), '')
    writeFileSync(join(dir, 'assets', 'latex-abc123.js'), '')
    writeFileSync(join(dir, 'assets', 'docker-abc123.js'), '')

    expect(collectRequiredStaticAssetUrls(dir).sort()).toEqual([
      '/static/App-abc123.css',
      '/static/assets/font-abc123.woff2',
      '/static/assets/spacewave-icon-abc123.png',
      '/static/hydrate-abc123.js',
    ])
  })
})

describe('selectAppCssFile', () => {
  it('selects the full app entry CSS instead of component chunk CSS', () => {
    const manifest: ViteManifest = {
      '_AnimatedLogo-abc.js': {
        file: 'assets/AnimatedLogo-abc.js',
        css: ['assets/AnimatedLogo-def.css'],
      },
      'app/App.tsx': {
        file: 'assets/app-abc.js',
        src: 'app/App.tsx',
        name: 'app',
        isEntry: true,
        css: ['assets/app-def.css'],
      },
    }

    expect(selectAppCssFile(manifest)).toBe('assets/app-def.css')
  })

  it('resolves app CSS from the split App chunk in release manifests', () => {
    // Release builds emit a thin cssless entry that statically imports a shared
    // _App-<hash>.mjs chunk (name "App") carrying app.css, alongside unrelated
    // component-chunk css that must not be selected.
    const manifest: ViteManifest = {
      '_AnimatedLogo-D_8yyfZb.mjs': {
        file: 'AnimatedLogo-D_8yyfZb.mjs',
        name: 'AnimatedLogo',
        css: ['AnimatedLogo-DaLnvTQS.css'],
      },
      '_App-DbWiT3Sv.mjs': {
        file: 'App-DbWiT3Sv.mjs',
        name: 'App',
        css: ['App-Cgwaez6P.css'],
      },
      'app/App.tsx': {
        file: 'app/App2.mjs',
        src: 'app/App.tsx',
        name: 'App',
        isEntry: true,
        imports: ['_App-DbWiT3Sv.mjs'],
      },
    }

    expect(selectAppCssFile(manifest)).toBe('App-Cgwaez6P.css')
  })

  it('returns undefined when no App-module css is reachable from the entry', () => {
    const manifest: ViteManifest = {
      '_vendor-abc.mjs': {
        file: 'vendor-abc.mjs',
        name: 'vendor',
        css: ['vendor-abc.css'],
      },
      'app/App.tsx': {
        file: 'app/App2.mjs',
        src: 'app/App.tsx',
        name: 'App',
        isEntry: true,
        imports: [],
      },
    }

    expect(selectAppCssFile(manifest)).toBeUndefined()
  })
})

describe('selectViteEntryAssets', () => {
  it('keeps the hydration script and CSS emitted with its component graph', () => {
    const manifest: ViteManifest = {
      '_shared-abc.js': {
        file: 'assets/shared-abc.js',
        css: ['assets/shared-abc.css'],
      },
      'app/prerender/hydrate.tsx': {
        file: 'hydrate-abc.js',
        src: 'app/prerender/hydrate.tsx',
        isEntry: true,
        imports: ['_shared-abc.js'],
        css: ['assets/hydrate-abc.css'],
      },
    }

    expect(
      selectViteEntryAssets(manifest, 'app/prerender/hydrate.tsx'),
    ).toEqual({
      file: 'hydrate-abc.js',
      css: ['assets/hydrate-abc.css', 'assets/shared-abc.css'],
    })
  })

  it('reports a missing hydration entry instead of omitting its CSS', () => {
    expect(
      selectViteEntryAssets({}, 'app/prerender/hydrate.tsx'),
    ).toBeUndefined()
  })
})
