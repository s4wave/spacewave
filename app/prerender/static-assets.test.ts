import { execFileSync } from 'child_process'
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  mkdirSync,
  rmSync,
  writeFileSync,
} from 'fs'
import { join, resolve } from 'path'
import { tmpdir } from 'os'

import { afterEach, describe, expect, it } from 'vitest'

import {
  collectRequiredStaticAssetUrls,
  preparePrerenderStaticAssets,
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

describe('preparePrerenderStaticAssets', () => {
  it('uses hydration output and source assets without spacewave-app build output', () => {
    dir = mkdtempSync(join(tmpdir(), 'spacewave-prerender-assets-'))
    const outputDir = join(dir, 'output')
    const sourceAssetsDir = join(dir, 'web', 'images')
    mkdirSync(join(outputDir, 'assets'), { recursive: true })
    mkdirSync(sourceAssetsDir, { recursive: true })
    writeFileSync(join(outputDir, 'hydrate-abc.js'), '')
    writeFileSync(join(outputDir, 'assets', 'hydrate-abc.css'), '')
    writeFileSync(join(sourceAssetsDir, 'spacewave-icon.png'), 'icon')
    writeFileSync(join(sourceAssetsDir, 'brand.svg'), 'brand')

    expect(
      preparePrerenderStaticAssets(outputDir, sourceAssetsDir, {
        file: 'hydrate-abc.js',
        css: ['assets/hydrate-abc.css'],
      }),
    ).toEqual({
      mainCssUrl: '/static/assets/hydrate-abc.css',
      additionalCssUrls: [],
      iconUrl: '/static/assets/spacewave-icon.png',
    })
    expect(collectRequiredStaticAssetUrls(outputDir).sort()).toEqual([
      '/static/assets/brand.svg',
      '/static/assets/hydrate-abc.css',
      '/static/assets/spacewave-icon.png',
      '/static/hydrate-abc.js',
    ])
    expect(
      readFileSync(join(outputDir, 'assets', 'spacewave-icon.png'), 'utf-8'),
    ).toBe('icon')
    expect(readFileSync(join(outputDir, 'assets', 'brand.svg'), 'utf-8')).toBe(
      'brand',
    )
  })
})

describe('hydration production assets', () => {
  it('publishes every generated non-data CSS URL under /static/', () => {
    dir = mkdtempSync(join(tmpdir(), 'spacewave-prerender-hydrate-'))
    const outputDir = join(dir, 'dist')
    execFileSync(
      'bunx',
      [
        'vite',
        'build',
        '--config',
        'app/prerender/vite.hydrate.config.ts',
        '--outDir',
        outputDir,
      ],
      { cwd: resolve(import.meta.dirname, '../..') },
    )

    const manifest = JSON.parse(
      readFileSync(join(outputDir, '.vite', 'manifest.json'), 'utf-8'),
    ) as ViteManifest
    const hydrateAssets = selectViteEntryAssets(
      manifest,
      'app/prerender/hydrate.tsx',
    )
    if (!hydrateAssets) {
      throw new Error('Hydration build did not emit its entry manifest record.')
    }
    preparePrerenderStaticAssets(
      outputDir,
      resolve(import.meta.dirname, '../../web/images'),
      hydrateAssets,
    )
    const published = new Set(collectRequiredStaticAssetUrls(outputDir))
    const urls = hydrateAssets.css
      .flatMap((cssFile) =>
        Array.from(
          readFileSync(join(outputDir, cssFile), 'utf-8').matchAll(
            /url\(\s*(['"]?)(.*?)\1\s*\)/g,
          ),
          (match) => match[2],
        ),
      )
      .filter((url) => !url.startsWith('data:'))

    expect(urls).not.toHaveLength(0)
    for (const url of urls) {
      expect(url).toMatch(/^\/static\//)
      expect(published).toContain(url)
      expect(existsSync(join(outputDir, url.slice('/static/'.length)))).toBe(
        true,
      )
    }
  }, 20_000)
})
