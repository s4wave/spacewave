import path from 'path'
import fs from 'fs'
import type { Rollup } from 'vite'
import { Plugin } from 'vite'

// List of file extensions that should be remapped to .mjs
const JS_EXTENSIONS = ['.js', '.cjs', '.jsx', '.ts', '.tsx']
const JS_EXTENSION_SET = new Set(JS_EXTENSIONS)

// Extensions stripped when deriving a served web pkg entry name. Must match the
// knownExts set in buildWebPkg (vite.ts), including ".pb" for proto modules, so
// a consumer derives the same served "[name].mjs" the build emits.
const KNOWN_EXTENSIONS = new Set([
  '.js',
  '.cjs',
  '.mjs',
  '.ts',
  '.tsx',
  '.jsx',
  '.pb',
  '.css',
])

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

// stripKnownExts removes all trailing known extensions, mirroring buildWebPkg's
// served-name derivation (e.g. "google/protobuf/timestamp.pb.js" -> "google/
// protobuf/timestamp").
function stripKnownExts(name: string): string {
  while (true) {
    const ext = path.extname(name)
    if (!ext || !KNOWN_EXTENSIONS.has(ext)) break
    name = name.substring(0, name.length - ext.length)
  }
  return name
}

function normalizePackageImportPath(importPath: string): string {
  return stripKnownExts(
    importPath.startsWith('./') ? importPath.substring(2) : importPath,
  )
}

function normalizePackageRootExport(raw: unknown): string | null {
  if (typeof raw === 'string') {
    return raw
  }
  if (!raw || typeof raw !== 'object') {
    return null
  }

  const obj = raw as Record<string, unknown>
  for (const key of ['import', 'default', 'require']) {
    const resolved = normalizePackageRootExport(obj[key])
    if (resolved) {
      return resolved
    }
  }
  for (const value of Object.values(obj)) {
    const resolved = normalizePackageRootExport(value)
    if (resolved) {
      return resolved
    }
  }
  return null
}

export function readPackageRootServedName(pkgRoot: string): string | null {
  try {
    const pkgJSON = JSON.parse(
      fs.readFileSync(path.join(pkgRoot, 'package.json'), 'utf8'),
    ) as Record<string, unknown>
    const exportsValue = pkgJSON['exports']
    if (exportsValue !== undefined) {
      let rootExport: unknown
      if (typeof exportsValue === 'string') {
        rootExport = exportsValue
      } else if (exportsValue && typeof exportsValue === 'object') {
        const exportsObj = exportsValue as Record<string, unknown>
        rootExport = exportsObj['.']
        if (rootExport === undefined) {
          const hasSubpath = Object.keys(exportsObj).some(
            (key) => key.startsWith('.') || key.startsWith('#'),
          )
          if (!hasSubpath) {
            rootExport = exportsObj
          }
        }
      }

      const resolved = normalizePackageRootExport(rootExport)
      if (resolved) {
        const name = normalizePackageImportPath(resolved)
        return name.startsWith('dist/') ? name.substring('dist/'.length) : name
      }
    }

    for (const key of ['module', 'main']) {
      const resolved = pkgJSON[key]
      if (typeof resolved === 'string' && resolved) {
        const name = normalizePackageImportPath(resolved)
        return name.startsWith('dist/') ? name.substring('dist/'.length) : name
      }
    }
  } catch {
    return null
  }
  return null
}

// buildServedNameMap maps an import subpath (relative to the package root, no
// extension) to the served "[name].mjs" file for that entry. The empty key maps
// the bare package specifier to its index entry when one is declared.
function buildServedNameMap(imports: string[]): Map<string, string> {
  const map = new Map<string, string>()
  for (const imp of imports) {
    const name = normalizePackageImportPath(imp)
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
  subPath = normalizePackageImportPath(subPath)
  const served = servedMap.get(subPath)
  if (!served) return null
  return webPkgURL(basePath, pkg, served)
}

// remapWebPkgSpecifier rewrites a web pkg import specifier to a served URL.
// Returns null if the id does not match any webPkgID.
function remapWebPkgSpecifier(
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
  const webPkgBasePath = (config.webPkgBasePath ?? '/b/pkg').replace(/\/+$/, '')
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
      // Fall back to trying node_modules resolution for any unresolved pkgs
      for (const pkgID of remappedWebPkgIDs) {
        if (!webPkgRoots[pkgID]) {
          try {
            const pkgJsonPath = require.resolve(pkgID + '/package.json', {
              paths: [root],
            })
            webPkgRoots[pkgID] = path.dirname(pkgJsonPath)
            if (debug)
              console.log(
                `[bldr-pkg-resolve] root for ${pkgID} (node_modules): ${webPkgRoots[pkgID]}`,
              )
          } catch {
            // Not resolvable from node_modules, will use empty root
          }
        }
        // Declared imports (webPkgImports) own the served-name map: they map the
        // bare specifier to the dist-stripped served index buildWebPkg emits.
        // Only fall back to the package.json root export (whose dist/ subdir
        // differs from the served names) when the package has no declared map,
        // so the on-disk path never clobbers an authoritative declared entry.
        const rootServedName =
          !servedNameMaps[pkgID] && webPkgRoots[pkgID]
            ? readPackageRootServedName(webPkgRoots[pkgID])
            : null
        if (rootServedName) {
          let map = servedNameMaps[pkgID]
          if (!map) {
            map = new Map<string, string>()
            servedNameMaps[pkgID] = map
          }
          const served = rootServedName + '.mjs'
          map.set('', served)
          map.set(rootServedName, served)
          if (debug)
            console.log(
              `[bldr-pkg-resolve] root served name for ${pkgID}: ${served}`,
            )
        }
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
        result = result.replace(pattern, (_match, prefix, subPathMatch) => {
          const fullId = pkg + (subPathMatch ?? '')
          const declaredURL = lookupDeclaredServedURL(
            webPkgBasePath,
            fullId,
            pkg,
            servedNameMaps[pkg],
          )
          if (declaredURL) {
            modified = true
            if (debug)
              console.log(
                `[bldr-pkg-resolve] renderChunk (declared): ${fullId} -> ${declaredURL}`,
              )
            return prefix + declaredURL
          }
          const remap = remapWebPkgSpecifier(
            fullId,
            remappedWebPkgIDs,
            webPkgBasePath,
          )
          if (!remap) return _match
          modified = true
          if (debug)
            console.log(
              `[bldr-pkg-resolve] renderChunk: ${fullId} -> ${remap.remapped}`,
            )
          return prefix + remap.remapped
        })
      }

      return modified ? result : null
    },
  }
}
