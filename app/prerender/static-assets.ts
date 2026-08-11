import { copyFileSync, existsSync, mkdirSync, readdirSync } from 'fs'
import { basename, dirname, extname, join, relative } from 'path'

export interface ViteManifestEntry {
  css?: string[]
  file?: string
  isEntry?: boolean
  imports?: string[]
  name?: string
  src?: string
}

export type ViteManifest = Record<string, ViteManifestEntry>

export interface ViteEntryAssets {
  file: string
  css: string[]
}

// selectViteEntryAssets returns an entry's emitted script and every stylesheet
// on its static import graph. Prerender hydration owns route component CSS, so
// its generated HTML must link the styles emitted with that entry.
export function selectViteEntryAssets(
  manifest: ViteManifest,
  entrySource: string,
): ViteEntryAssets | undefined {
  const entryKey = Object.keys(manifest).find(
    (key) => key === entrySource || manifest[key].src === entrySource,
  )
  if (!entryKey) return undefined

  const entryFile = manifest[entryKey].file
  if (!entryFile) return undefined

  const seenEntries = new Set<string>()
  const cssFiles = new Set<string>()
  const queue = [entryKey]
  while (queue.length) {
    const key = queue.shift()
    if (key === undefined || seenEntries.has(key)) continue
    seenEntries.add(key)

    const entry = manifest[key]
    if (!entry) continue
    for (const cssFile of entry.css ?? []) cssFiles.add(cssFile)
    for (const importKey of entry.imports ?? []) queue.push(importKey)
  }

  return { file: entryFile, css: Array.from(cssFiles) }
}

const requiredStaticExtensions = new Set([
  '.css',
  '.woff2',
  '.png',
  '.svg',
  '.ico',
])

const sourceStaticExtensions = new Set(['.png', '.svg', '.ico'])

export interface PrerenderStaticAssets {
  mainCssUrl: string
  additionalCssUrls: string[]
  iconUrl: string
}

// preparePrerenderStaticAssets verifies the hydration output and copies the
// source assets that static page modules reference. The static shell links its
// complete stylesheet graph from the hydration build, independently of release
// app plugins.
export function preparePrerenderStaticAssets(
  outputDir: string,
  sourceAssetsDir: string,
  hydrateAssets: ViteEntryAssets,
): PrerenderStaticAssets {
  const [mainCssFile, ...additionalCssFiles] = hydrateAssets.css
  if (!mainCssFile) {
    throw new Error('Hydration entry emitted no CSS.')
  }
  for (const file of [hydrateAssets.file, ...hydrateAssets.css]) {
    if (!existsSync(join(outputDir, file))) {
      throw new Error(`Hydration asset not found at ${join(outputDir, file)}`)
    }
  }

  const iconFile = 'spacewave-icon.png'
  if (!existsSync(join(sourceAssetsDir, iconFile))) {
    throw new Error(
      `Prerender icon not found at ${join(sourceAssetsDir, iconFile)}`,
    )
  }

  const outputAssetsDir = join(outputDir, 'assets')
  mkdirSync(outputAssetsDir, { recursive: true })
  for (const file of readdirSync(sourceAssetsDir)) {
    if (!sourceStaticExtensions.has(extname(file))) continue
    copyFileSync(join(sourceAssetsDir, file), join(outputAssetsDir, file))
  }

  return {
    mainCssUrl: '/static/' + mainCssFile,
    additionalCssUrls: additionalCssFiles.map((file) => '/static/' + file),
    iconUrl: '/static/assets/' + iconFile,
  }
}

function isRequiredStaticAsset(rel: string): boolean {
  const ext = extname(rel)
  if (requiredStaticExtensions.has(ext)) {
    return true
  }
  return (
    ext === '.js' &&
    dirname(rel) === '.' &&
    basename(rel).startsWith('hydrate-')
  )
}

export function collectRequiredStaticAssetUrls(dir: string): string[] {
  const assets: string[] = []

  function walk(curr: string) {
    for (const entry of readdirSync(curr, { withFileTypes: true })) {
      const entryPath = join(curr, entry.name)
      if (entry.isDirectory()) {
        walk(entryPath)
        continue
      }
      if (!entry.isFile()) {
        continue
      }

      const rel = relative(dir, entryPath).replaceAll('\\', '/')
      if (!isRequiredStaticAsset(rel)) {
        continue
      }

      assets.push('/static/' + rel)
    }
  }

  walk(dir)
  return assets
}
