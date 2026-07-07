import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'fs'
import { join } from 'path'
import { tmpdir } from 'os'

import { afterEach, describe, expect, it } from 'vitest'

import {
  collectRequiredStaticAssetUrls,
  selectAppCssFile,
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
})
