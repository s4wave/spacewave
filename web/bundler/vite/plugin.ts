import path from 'path'
import type { Rollup } from 'vite'
import { Plugin } from 'vite'

// List of file extensions that should be remapped to .mjs
const JS_EXTENSIONS = ['.js', '.cjs', '.jsx', '.ts', '.tsx']

export interface WebPkgRemapPluginConfig {
  // List of packages that can be bundled as web pkgs
  webPkgIDs: string[]
  // Optional callback to report the resolved root directory for a web package.
  // Called once per package when the root is first discovered.
  addWebPkgRoot?: (webPkgID: string, webPkgRoot: string) => void
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
      if (!pkgNameRegex.test(pkgID) || !config.webPkgIDs.includes(pkgID)) {
        return null
      }

      // Resolve the import to find the actual file on disk.
      const resolved = await this.resolve(importId, importer, {
        ...options,
        custom: { 'bldr-pkg-resolve': true },
      })
      if (!resolved || !resolved.id) {
        // Fall back to simple remap without resolution.
        const result = remapWebPkgSpecifier(importId, config.webPkgIDs)
        if (!result) return null
        if (debug)
          console.log(
            `[bldr-pkg-resolve] resolveId (fallback): ${importId} -> ${result.remapped}`,
          )
        return { id: result.remapped, external: true }
      }

      // Compute relative path within the package root.
      const pkgRoot = webPkgRoots[pkgID]
      let relPath: string
      if (pkgRoot && resolved.id.startsWith(pkgRoot)) {
        relPath = resolved.id.substring(pkgRoot.length).replace(/^\//, '')
      } else {
        // Could not determine relative path, use the specifier subpath.
        const result = remapWebPkgSpecifier(importId, config.webPkgIDs)
        if (!result) return null
        if (debug)
          console.log(
            `[bldr-pkg-resolve] resolveId (no root): ${importId} -> ${result.remapped}`,
          )
        return { id: result.remapped, external: true }
      }

      // Remap JS extensions to .mjs to match web pkg output.
      const ext = path.extname(relPath)
      if (JS_EXTENSIONS.includes(ext)) {
        relPath = relPath.substring(0, relPath.length - ext.length) + '.mjs'
      }

      const remapped = `/b/pkg/${pkgID}/${relPath}`

      // Report the resolved root for this web package.
      if (config.addWebPkgRoot && pkgRoot) {
        config.addWebPkgRoot(pkgID, pkgRoot)
      }

      if (debug)
        console.log(
          `[bldr-pkg-resolve] resolveId: ${importId} -> ${remapped}`,
        )

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
          return prefix + remap.remapped
        })
      }

      return modified ? result : null
    },
  }
}
