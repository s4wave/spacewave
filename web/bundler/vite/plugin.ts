import path from 'path'
import fs from 'fs'
import type { Rollup } from 'vite'
import { Plugin } from 'vite'

// List of file extensions that should be remapped to .mjs
const JS_EXTENSIONS = ['.js', '.cjs', '.jsx', '.ts', '.tsx']

export interface WebPkgRemapPluginConfig {
  // List of packages that can be bundled as web pkgs
  webPkgIDs: string[]
  // Optional callback
  addWebPkgImport?: (
    webPkgID: string,
    webPkgRoot: string,
    webPkgSubPath: string,
  ) => void
  // Enable debug logging
  debug?: boolean
}

// remapWebPkgSpecifier rewrites a web pkg import specifier to a /b/pkg/ URL.
// Returns null if the id does not match any webPkgID.
function remapWebPkgSpecifier(
  id: string,
  webPkgIDs: string[],
): { pkg: string; subPath: string; remapped: string } | null {
  for (const pkg of webPkgIDs) {
    if (id === pkg || id.startsWith(pkg + '/')) {
      let subPath = id === pkg ? '' : id.substring(pkg.length + 1)
      if (subPath) {
        const ext = path.extname(subPath)
        if (JS_EXTENSIONS.includes(ext)) {
          subPath = subPath.substring(0, subPath.length - ext.length) + '.mjs'
        }
      }
      return {
        pkg,
        subPath,
        remapped: `/b/pkg/${pkg}${subPath ? '/' + subPath : ''}`,
      }
    }
  }
  return null
}

// escapeRegExp escapes special regex characters in a string.
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// TS_RESOLVE_EXTENSIONS are the extensions to try when resolving a .js
// import to a TypeScript source file.
const TS_RESOLVE_EXTENSIONS = ['.ts', '.tsx', '.jsx', '.mts', '.cts']

// resolveSubPathExtension resolves a subpath like "contexts/Foo.js" to
// the actual file extension on disk (e.g. "contexts/Foo.tsx"). If the
// root is empty or the file can't be found, returns the original subpath.
function resolveSubPathExtension(root: string, subPath: string): string {
  if (!root) return subPath

  const fullPath = path.join(root, subPath)
  if (fs.existsSync(fullPath)) return subPath

  // Try replacing .js with TS extensions
  const ext = path.extname(subPath)
  if (ext === '.js' || ext === '.mjs' || ext === '.cjs') {
    const base = subPath.substring(0, subPath.length - ext.length)
    for (const tsExt of TS_RESOLVE_EXTENSIONS) {
      const candidate = base + tsExt
      if (fs.existsSync(path.join(root, candidate))) {
        return candidate
      }
    }
  }

  return subPath
}

export function createWebPkgRemapPlugin(
  config: WebPkgRemapPluginConfig,
): Plugin {
  const debug = config.debug || false

  // Resolved root directories for each web pkg, populated in configResolved.
  const webPkgRoots: Record<string, string> = {}

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
          if (find && config.webPkgIDs.includes(find) && alias.replacement) {
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
      for (const pkgID of config.webPkgIDs) {
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
      }
    },

    // resolveId handles imports that bypass tsconfig resolution
    // (e.g. from node_modules). For tsconfig-aliased packages,
    // rolldownOptions.external catches them first and renderChunk
    // rewrites the specifiers.
    async resolveId(
      importId,
      importer,
      options,
    ): Promise<Rollup.ResolveIdResult> {
      if (importer === 'bldr-pkg-resolve' || importId?.startsWith('.')) {
        return null
      }

      const normalizedImportId = importId.trim().replace(/^\//, '')
      if (normalizedImportId.length === 0) return null

      let pkgID: string,
        subPath: string = ''
      if (normalizedImportId.startsWith('@')) {
        const firstSlash = normalizedImportId.indexOf('/')
        if (firstSlash === -1) return null
        const secondSlash = normalizedImportId.indexOf('/', firstSlash + 1)
        pkgID =
          secondSlash === -1 ?
            normalizedImportId
          : normalizedImportId.substring(0, secondSlash)
        subPath =
          secondSlash === -1 ?
            ''
          : normalizedImportId.substring(secondSlash + 1)
      } else {
        const firstSlash = normalizedImportId.indexOf('/')
        pkgID =
          firstSlash === -1 ?
            normalizedImportId
          : normalizedImportId.substring(0, firstSlash)
        subPath =
          firstSlash === -1 ? '' : normalizedImportId.substring(firstSlash + 1)
      }

      subPath = path.posix.normalize(subPath).replace(/^(\.\/|\.|\/)/, '')
      const pkgNameRegex =
        /^(@[a-z0-9-~][a-z0-9-._~]*\/)?[a-z0-9-~][a-z0-9-._~]*$/
      if (!pkgNameRegex.test(pkgID) || !config.webPkgIDs.includes(pkgID)) {
        return null
      }

      const result = remapWebPkgSpecifier(importId, config.webPkgIDs)
      if (!result) return null

      if (debug)
        console.log(
          `[bldr-pkg-resolve] resolveId: ${importId} -> ${result.remapped}`,
        )

      return { id: result.remapped, external: true }
    },

    // renderChunk rewrites external web pkg import specifiers in the
    // output code. This handles the case where rolldownOptions.external
    // marks the import as external (preserving the original specifier)
    // but we need /b/pkg/ URLs with .mjs extensions.
    renderChunk(code) {
      if (config.webPkgIDs.length === 0) return null

      let modified = false
      let result = code

      for (const pkg of config.webPkgIDs) {
        // Match both named imports and side-effect imports:
        //   from "@s4wave/web/..."      (named/namespace imports)
        //   import "@s4wave/web/..."     (side-effect, e.g. CSS)
        //   import("@s4wave/web/...")     (dynamic import)
        const pattern = new RegExp(
          `((?:from|import)\\s*\\(?\\s*["'])${escapeRegExp(pkg)}(/[^"']*)?(?=["'])`,
          'g',
        )
        result = result.replace(pattern, (_match, prefix, subPathMatch) => {
          const fullId = pkg + (subPathMatch ?? '')
          const remap = remapWebPkgSpecifier(fullId, config.webPkgIDs)
          if (!remap) return _match
          modified = true
          if (debug)
            console.log(
              `[bldr-pkg-resolve] renderChunk: ${fullId} -> ${remap.remapped}`,
            )
          // Track the import with the filesystem subpath. The Go
          // esbuild step needs the actual file extension (.ts/.tsx)
          // because esbuild doesn't resolve .js -> .tsx like TS does.
          if (config.addWebPkgImport && subPathMatch) {
            const originalSubPath = subPathMatch.substring(1) // strip leading /
            const root = webPkgRoots[remap.pkg] ?? ''
            const resolvedSubPath = resolveSubPathExtension(root, originalSubPath)
            config.addWebPkgImport(remap.pkg, root, resolvedSubPath)
          }
          return prefix + remap.remapped
        })
      }

      return modified ? result : null
    },
  }
}
