import { readdirSync } from 'fs'
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

// APP_ENTRY_SRC is the source module of the app plugin entry, which imports
// web/style/app.css. Its stylesheet is the full app CSS the prerendered shell
// links.
const APP_ENTRY_SRC = 'app/App.tsx'

// isAppEntryRecord reports whether a manifest record is the app plugin entry.
function isAppEntryRecord(key: string, entry: ViteManifestEntry): boolean {
  return (
    !!entry.isEntry &&
    (key === APP_ENTRY_SRC ||
      entry.src === APP_ENTRY_SRC ||
      entry.name === 'app')
  )
}

// isAppModuleRecord reports whether a record is the App application module: the
// chunk App.tsx (and its app.css import) compiles into. Release builds split
// App into a shared _App-<hash>.mjs chunk named "App" that carries app.css and
// leave the thin entry record cssless, while dev/test builds keep the css on
// the entry record itself. Matching the module by identity covers both shapes.
function isAppModuleRecord(entry: ViteManifestEntry): boolean {
  return entry.src === APP_ENTRY_SRC || entry.name === 'App'
}

// selectAppCssFile returns the app plugin stylesheet from a Vite manifest. It
// walks the app entry's static import graph and returns the app.css attached to
// the App application module, so it resolves the css whether the build records
// it on the entry record or on the split App chunk the entry imports. It
// returns undefined when no App-module css is reachable; the caller reports the
// searched manifest keys so a changed manifest shape fails loudly.
export function selectAppCssFile(manifest: ViteManifest): string | undefined {
  const entryKey = Object.keys(manifest).find((key) =>
    isAppEntryRecord(key, manifest[key]),
  )
  if (!entryKey) return undefined

  const seen = new Set<string>()
  const queue = [entryKey]
  while (queue.length) {
    const key = queue.shift()
    if (key === undefined || seen.has(key)) continue
    seen.add(key)
    const entry = manifest[key]
    if (!entry) continue
    if (isAppModuleRecord(entry)) {
      const [cssFile] = entry.css ?? []
      if (cssFile) return cssFile
    }
    for (const importKey of entry.imports ?? []) queue.push(importKey)
  }
  return undefined
}

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
