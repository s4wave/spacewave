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
  const appEntry = Object.entries(manifest).find(([key, entry]) => {
    if (!entry.isEntry) {
      return false
    }
    if (key === 'app/App.tsx' || entry.src === 'app/App.tsx') {
      return true
    }
    return entry.name === 'app'
  })?.[1]
  if (appEntry?.css?.length) {
    return appEntry.css[0]
  }

  const appCssFiles = Object.values(manifest)
    .flatMap((entry) => entry.css ?? [])
    .filter((cssFile) => basename(cssFile).startsWith('app-'))

  if (appCssFiles.length === 1) {
    return appCssFiles[0]
  }

  return undefined
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
