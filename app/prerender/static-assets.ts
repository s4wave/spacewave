import { readdirSync } from 'fs'
import { basename, dirname, extname, join, relative } from 'path'

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
