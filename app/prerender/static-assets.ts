import { readdirSync } from 'fs'
import { basename, dirname, extname, join, relative } from 'path'

export interface ViteManifestEntry {
  css?: string[]
  file?: string
  isEntry?: boolean
  name?: string
  src?: string
}

export type ViteManifest = Record<string, ViteManifestEntry>

export function selectAppCssFile(manifest: ViteManifest): string | undefined {
  let fallbackCssFile: string | undefined
  let fallbackCssCount = 0

  for (const [key, entry] of Object.entries(manifest)) {
    if (
      entry.isEntry &&
      (key === 'app/App.tsx' ||
        entry.src === 'app/App.tsx' ||
        entry.name === 'app')
    ) {
      const [cssFile] = entry.css ?? []
      if (cssFile) return cssFile
    }

    for (const cssFile of entry.css ?? []) {
      if (!basename(cssFile).startsWith('app-')) continue
      fallbackCssCount++
      if (fallbackCssCount === 1) {
        fallbackCssFile = cssFile
      }
    }
  }

  return fallbackCssCount === 1 ? fallbackCssFile : undefined
}
const requiredStaticExtensions = new Set([
  '.css',
  '.woff2',
  '.png',
  '.svg',
  '.ico',
])

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
