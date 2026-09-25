import path from 'path'
import fs from 'fs'
import type { Rollup } from 'vite'
import { Plugin } from 'vite'

import { servedEntryName, specifierEntryNames } from './web-pkg-naming.js'

// List of file extensions that should be remapped to .mjs
const JS_EXTENSIONS = ['.js', '.mjs', '.cjs', '.jsx', '.ts', '.tsx']
const JS_EXTENSION_SET = new Set(JS_EXTENSIONS)

/** isWebPkgModule keeps styles and assets in Vite's normal loading pipeline. */
export function isWebPkgModule(source: string): boolean {
  const extension = path.extname(source.split('?')[0]!)
  return !extension || JS_EXTENSION_SET.has(extension)
}

export interface WebPkgRemapPluginConfig {
  // List of packages that can be bundled as web pkgs
  webPkgIDs: string[]
  // Package IDs kept as bare imports so the document import map owns them.
  preserveWebPkgIDs?: string[]
  // Per-package declared entry imports, relative to each package's web pkg root.
  // When present for a package, served names are derived from these imports
  // (matching buildWebPkg) instead of the package's on-disk file layout, whose
  // dist/ subdir and .pb.js filenames differ from the served names.
  webPkgImports?: Record<string, string[]>
  // Base URL path that serves web package files.
  // Defaults to the plugin-assets route; entrypoint web packages use /entrypoint/pkgs.
  webPkgBasePath?: string
  // Optional callback to report the resolved root directory for a web package.
  // Called once per package when the root is first discovered.
  addWebPkgRoot?: (webPkgID: string, webPkgRoot: string) => void
  // Enable debug logging
  debug?: boolean
}

// resolveExportTarget resolves a package.json export value to its target path,
// preferring the import, default, then require conditions.
function resolveExportTarget(raw: unknown): string | null {
  if (typeof raw === 'string') {
    return raw
  }
  if (!raw || typeof raw !== 'object') {
    return null
  }

  const obj = raw as Record<string, unknown>
  for (const key of ['import', 'default', 'require']) {
    const resolved = resolveExportTarget(obj[key])
    if (resolved) {
      return resolved
    }
  }
  for (const value of Object.values(obj)) {
    const resolved = resolveExportTarget(value)
    if (resolved) {
      return resolved
    }
  }
  return null
}

// readPackageExportTargets maps each concrete export subpath of a package,
// "" for the root, to its target path. The root falls back to module or main.
// Wildcard subpaths are skipped: the provider serves only concrete entries.
function readPackageExportTargets(pkgRoot: string): Map<string, string> {
  const targets = new Map<string, string>()
  let pkgJSON: Record<string, unknown>
  try {
    pkgJSON = JSON.parse(
      fs.readFileSync(path.join(pkgRoot, 'package.json'), 'utf8'),
    ) as Record<string, unknown>
  } catch {
    return targets
  }

  const exportsValue = pkgJSON['exports']
  if (typeof exportsValue === 'string') {
    targets.set('', exportsValue)
  } else if (exportsValue && typeof exportsValue === 'object') {
    const exportsObj = exportsValue as Record<string, unknown>
    const subpaths = Object.keys(exportsObj).filter(
      (key) => key.startsWith('.') || key.startsWith('#'),
    )
    if (subpaths.length === 0) {
      const root = resolveExportTarget(exportsObj)
      if (root) targets.set('', root)
    }
    for (const key of subpaths) {
      if (key.startsWith('#') || key.includes('*')) continue
      const target = resolveExportTarget(exportsObj[key])
      if (!target || target.includes('*')) continue
      targets.set(key === '.' ? '' : key.replace(/^\.\//, ''), target)
    }
  }

  if (!targets.has('')) {
    for (const key of ['module', 'main']) {
      const resolved = pkgJSON[key]
      if (typeof resolved === 'string' && resolved) {
        targets.set('', resolved)
        break
      }
    }
  }
  return targets
}

/** readPackageRootServedName returns the served name of a package's root entry. */
export function readPackageRootServedName(pkgRoot: string): string | null {
  const root = readPackageExportTargets(pkgRoot).get('')
  return root ? servedEntryName(root) : null
}

/**
 * readPackageServedNameMap maps each export subpath of a package, "" for the
 * root, to the served "[name].mjs" file of its target, so "shiki/langs"
 * reaches the "dist/langs.mjs" entry the provider builds. Each target's own
 * served name maps too, for imports that address the file directly.
 */
export function readPackageServedNameMap(pkgRoot: string): Map<string, string> {
  const map = new Map<string, string>()
  for (const [subPath, target] of readPackageExportTargets(pkgRoot)) {
    const name = servedEntryName(target)
    const served = name + '.mjs'
    map.set(name, served)
    for (const key of subPath ? specifierEntryNames(subPath) : ['']) {
      map.set(key, served)
    }
  }
  return map
}

/**
 * resolveNodeWebPkgRoot returns the node_modules directory of a web package,
 * or null when the package is not installed there.
 */
export function resolveNodeWebPkgRoot(
  pkgID: string,
  root: string,
): string | null {
  try {
    return path.dirname(
      require.resolve(pkgID + '/package.json', { paths: [root] }),
    )
  } catch {
    const candidate = path.join(root, 'node_modules', pkgID)
    return fs.existsSync(path.join(candidate, 'package.json'))
      ? candidate
      : null
  }
}

// buildServedNameMap maps each declared entry's served name to its served
// "[name].mjs" file. The empty key maps the bare package specifier to its index
// entry when one is declared.
function buildServedNameMap(imports: string[]): Map<string, string> {
  const map = new Map<string, string>()
  for (const imp of imports) {
    const name = servedEntryName(imp)
    const served = name + '.mjs'
    map.set(name, served)
    if (name === 'index') {
      map.set('', served)
    }
  }
  return map
}

// webPkgURL returns the served URL for a web-package file.
function webPkgURL(basePath: string, pkg: string, subPath: string): string {
  return `${basePath}/${pkg}/${subPath}`
}

// lookupDeclaredServedURL returns the served URL for importId derived from the
// package's declared served-name map, or null when the package has no declared
// imports or importId is not a declared entry (callers then fall back to
// on-disk or specifier-based remapping).
function lookupDeclaredServedURL(
  basePath: string,
  importId: string,
  pkg: string,
  servedMap: Map<string, string> | undefined,
): string | null {
  if (!servedMap) return null
  const norm = importId.trim().replace(/^\//, '')
  let subPath: string
  if (norm === pkg) {
    subPath = ''
  } else if (norm.startsWith(pkg + '/')) {
    subPath = norm.substring(pkg.length + 1)
  } else {
    return null
  }
  for (const name of subPath ? specifierEntryNames(subPath) : ['']) {
    const served = servedMap.get(name)
    if (served) return webPkgURL(basePath, pkg, served)
  }
  return null
}

// remapWebPkgSpecifier rewrites a web pkg import specifier to a served URL.
// Returns null if the id does not match any webPkgID.
export function remapWebPkgSpecifier(
  id: string,
  webPkgIDs: string[],
  basePath: string,
): { pkg: string; subPath: string; remapped: string } | null {
  for (const pkg of webPkgIDs) {
    if (id === pkg || id.startsWith(pkg + '/')) {
      let subPath = id === pkg ? '' : id.substring(pkg.length + 1)
      if (subPath) {
        const ext = path.extname(subPath)
        if (ext === '') {
          subPath += '.mjs'
        } else if (JS_EXTENSION_SET.has(ext)) {
          subPath = subPath.substring(0, subPath.length - ext.length) + '.mjs'
        }
      }
      const remappedSubPath = subPath || 'index.mjs'
      return {
        pkg,
        subPath: remappedSubPath,
        remapped: webPkgURL(basePath, pkg, remappedSubPath),
      }
    }
  }
  return null
}

/**
 * resolveWebPkgImportURL returns the served URL for a web package import:
 * the package's served-name map when it names the import, else the specifier
 * remap. Returns null if the id does not match any webPkgID.
 */
export function resolveWebPkgImportURL(
  id: string,
  webPkgIDs: string[],
  basePath: string,
  servedNameMaps: Record<string, Map<string, string>>,
): string | null {
  const remap = remapWebPkgSpecifier(id, webPkgIDs, basePath)
  if (!remap) return null
  return (
    lookupDeclaredServedURL(
      basePath,
      id,
      remap.pkg,
      servedNameMaps[remap.pkg],
    ) ?? remap.remapped
  )
}

// escapeRegExp escapes special regex characters in a string.
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function relativePathInsideRoot(root: string, filename: string): string | null {
  const relPath = path.relative(root, filename)
  if (relPath === '' || relPath.startsWith('..') || path.isAbsolute(relPath)) {
    return null
  }
  return relPath
}

export function createWebPkgRemapPlugin(
  config: WebPkgRemapPluginConfig,
): Plugin {
  const debug = config.debug || false
  const webPkgBasePath = ('/' + (config.webPkgBasePath ?? '/b/pkg'))
    .replace(/\/+/g, '/')
    .replace(/\/+$/, '')
  const preservedWebPkgIDSet = new Set(config.preserveWebPkgIDs ?? [])
  const remappedWebPkgIDs = config.webPkgIDs.filter(
    (pkg) => !preservedWebPkgIDSet.has(pkg),
  )
  const webPkgIDSet = new Set(remappedWebPkgIDs)
  const webPkgPatterns = remappedWebPkgIDs.map((pkg) => ({
    pkg,
    pattern: new RegExp(
      `((?:from|import)\\s*\\(?\\s*["'])${escapeRegExp(pkg)}(/[^"']*)?(?=["'])`,
      'g',
    ),
  }))

  // Resolved root directories for each web pkg, populated in configResolved.
  const webPkgRoots: Record<string, string> = {}

  // Served-name maps for packages with declared imports. When a package has a
  // map, its served names are derived from the declared imports (matching
  // buildWebPkg) instead of the on-disk file layout.
  const servedNameMaps: Record<string, Map<string, string>> = {}
  for (const pkg of remappedWebPkgIDs) {
    const imports = config.webPkgImports?.[pkg]
    if (imports && imports.length > 0) {
      servedNameMaps[pkg] = buildServedNameMap(imports)
    }
  }

  return {
    name: 'bldr-pkg-resolve',
    enforce: 'pre',
    apply: 'build',

    // Extract web pkg root directories from the resolved Vite config.
    // We look at resolve.alias entries that match web pkg IDs.
    // For tsconfig-aliased packages, Vite injects alias entries from
    // compilerOptions.paths.
    configResolved(resolvedConfig) {
      const root = resolvedConfig.root || process.cwd()
      const aliases = resolvedConfig.resolve?.alias
      if (Array.isArray(aliases)) {
        for (const alias of aliases) {
          const find =
            typeof alias.find === 'string' ? alias.find : alias.find?.source
          if (find && webPkgIDSet.has(find) && alias.replacement) {
            const resolved = path.isAbsolute(alias.replacement)
              ? alias.replacement
              : path.resolve(root, alias.replacement)
            webPkgRoots[find] = resolved
            if (debug)
              console.log(`[bldr-pkg-resolve] root for ${find}: ${resolved}`)
          }
        }
      }
      // Fall back to node_modules resolution for any unresolved pkgs.
      for (const pkgID of remappedWebPkgIDs) {
        if (!webPkgRoots[pkgID]) {
          const pkgRoot = resolveNodeWebPkgRoot(pkgID, root)
          if (pkgRoot) {
            webPkgRoots[pkgID] = pkgRoot
            if (debug)
              console.log(
                `[bldr-pkg-resolve] root for ${pkgID} (node_modules): ${pkgRoot}`,
              )
          }
        }
        // Declared imports own served names relative to the provider's root.
        // Without them, map the package's exports to the entries emitted by
        // buildWebPkg, retaining their directory paths.
        if (servedNameMaps[pkgID] || !webPkgRoots[pkgID]) continue
        const map = readPackageServedNameMap(webPkgRoots[pkgID])
        if (map.size === 0) continue
        servedNameMaps[pkgID] = map
        if (debug)
          console.log(
            `[bldr-pkg-resolve] export served names for ${pkgID}: ${[...map.keys()].join(', ')}`,
          )
      }
    },

    // resolveId resolves sibling web pkg imports to /b/pkg/ URLs.
    // Uses Vite's resolver to find the actual file path, then computes
    // the relative path within the package and remaps .js -> .mjs.
    async resolveId(
      importId,
      importer,
      options,
    ): Promise<Rollup.ResolveIdResult> {
      if (options?.custom?.['bldr-pkg-resolve'] || importId?.startsWith('.')) {
        return null
      }

      const normalizedImportId = importId.trim().replace(/^\//, '')
      if (normalizedImportId.length === 0) return null

      // CSS and assets belong to Vite's asset pipeline, even when their package
      // supplies shared JavaScript modules from another plugin.
      if (!isWebPkgModule(normalizedImportId)) return null

      let pkgID: string
      if (normalizedImportId.startsWith('@')) {
        const firstSlash = normalizedImportId.indexOf('/')
        if (firstSlash === -1) return null
        const secondSlash = normalizedImportId.indexOf('/', firstSlash + 1)
        pkgID =
          secondSlash === -1
            ? normalizedImportId
            : normalizedImportId.substring(0, secondSlash)
      } else {
        const firstSlash = normalizedImportId.indexOf('/')
        pkgID =
          firstSlash === -1
            ? normalizedImportId
            : normalizedImportId.substring(0, firstSlash)
      }

      const pkgNameRegex =
        /^(@[a-z0-9-~][a-z0-9-._~]*\/)?[a-z0-9-~][a-z0-9-._~]*$/
      if (!pkgNameRegex.test(pkgID) || !remappedWebPkgIDs.includes(pkgID)) {
        return null
      }

      // Packages with declared imports define their served names from the
      // import list, not on-disk layout. Derive the served URL directly so the
      // dist/ subdir and .pb.js filenames never leak into the baked URL.
      const declaredURL = lookupDeclaredServedURL(
        webPkgBasePath,
        normalizedImportId,
        pkgID,
        servedNameMaps[pkgID],
      )
      if (declaredURL) {
        if (config.addWebPkgRoot && webPkgRoots[pkgID]) {
          config.addWebPkgRoot(pkgID, webPkgRoots[pkgID])
        }
        if (debug)
          console.log(
            `[bldr-pkg-resolve] resolveId (declared): ${importId} -> ${declaredURL}`,
          )
        return { id: declaredURL, external: true }
      }

      // Resolve the import to find the actual file on disk.
      const resolved = await this.resolve(importId, importer, {
        ...options,
        custom: { 'bldr-pkg-resolve': true },
      })
      if (!resolved || !resolved.id) {
        // Fall back to simple remap without resolution.
        const result = remapWebPkgSpecifier(
          importId,
          remappedWebPkgIDs,
          webPkgBasePath,
        )
        if (!result) return null
        if (debug)
          console.log(
            `[bldr-pkg-resolve] resolveId (fallback): ${importId} -> ${result.remapped}`,
          )
        return { id: result.remapped, external: true }
      }

      // Compute relative path within the package root.
      const pkgRoot = webPkgRoots[pkgID]
      const resolvedRelPath = pkgRoot
        ? relativePathInsideRoot(pkgRoot, resolved.id)
        : null
      if (!resolvedRelPath) {
        // Could not determine relative path, use the specifier subpath.
        const result = remapWebPkgSpecifier(
          importId,
          remappedWebPkgIDs,
          webPkgBasePath,
        )
        if (!result) return null
        if (debug)
          console.log(
            `[bldr-pkg-resolve] resolveId (no root): ${importId} -> ${result.remapped}`,
          )
        return { id: result.remapped, external: true }
      }

      // Remap JS extensions to .mjs to match web pkg output.
      const ext = path.extname(resolvedRelPath)
      const relPath = JS_EXTENSIONS.includes(ext)
        ? resolvedRelPath.substring(0, resolvedRelPath.length - ext.length) +
          '.mjs'
        : resolvedRelPath

      const remapped = webPkgURL(webPkgBasePath, pkgID, relPath)

      // Report the resolved root for this web package.
      if (config.addWebPkgRoot && pkgRoot) {
        config.addWebPkgRoot(pkgID, pkgRoot)
      }

      if (debug)
        console.log(`[bldr-pkg-resolve] resolveId: ${importId} -> ${remapped}`)

      return { id: remapped, external: true }
    },

    // renderChunk rewrites external web pkg import specifiers in the
    // output code. This handles the case where rolldownOptions.external
    // marks the import as external (preserving the original specifier)
    // but we need /b/pkg/ URLs with .mjs extensions.
    //
    // NOTE: This hook only rewrites specifiers. It does NOT track imports
    // for entry point discovery. Entry points are configured explicitly
    // via WebPkgRefConfig.entrypoints (project-local packages) or read
    // from package.json exports (node_modules packages).
    renderChunk(code) {
      if (remappedWebPkgIDs.length === 0) return null

      let modified = false
      let result = code

      for (const { pattern, pkg } of webPkgPatterns) {
        result = result.replace(pattern, (match, prefix, subPathMatch) => {
          const fullId = pkg + (subPathMatch ?? '')
          const remapped = resolveWebPkgImportURL(
            fullId,
            remappedWebPkgIDs,
            webPkgBasePath,
            servedNameMaps,
          )
          if (!remapped) return match
          modified = true
          if (debug)
            console.log(
              `[bldr-pkg-resolve] renderChunk: ${fullId} -> ${remapped}`,
            )
          return prefix + remapped
        })
      }

      return modified ? result : null
    },
  }
}
