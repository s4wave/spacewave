import path from 'path'
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

export function createWebPkgRemapPlugin(
  config: WebPkgRemapPluginConfig,
): Plugin {
  const WEB_PKG_PREFIX = '/b/pkg/' // Prefix for remapped external imports
  const debug = config.debug || false

  return {
    name: 'bldr-pkg-resolve',
    enforce: 'pre',
    apply: 'build',

    async resolveId(importId, importer, options): Promise<Rollup.ResolveIdResult> {
      // Skip self-resolution or relative imports
      if (importer === 'bldr-pkg-resolve' || importId?.startsWith('.')) {
        return null
      }

      // Parse package ID and subpath from import
      const normalizedImportId = importId.trim().replace(/^\//, '')
      if (normalizedImportId.length === 0) return null

      let pkgID: string,
        subPath: string = ''
      if (normalizedImportId.startsWith('@')) {
        const firstSlash = normalizedImportId.indexOf('/')
        if (firstSlash === -1) return null
        const secondSlash = normalizedImportId.indexOf('/', firstSlash + 1)
        pkgID =
          secondSlash === -1 ? normalizedImportId : (
            normalizedImportId.substring(0, secondSlash)
          )
        subPath =
          secondSlash === -1 ? '' : (
            normalizedImportId.substring(secondSlash + 1)
          )
      } else {
        const firstSlash = normalizedImportId.indexOf('/')
        pkgID =
          firstSlash === -1 ? normalizedImportId : (
            normalizedImportId.substring(0, firstSlash)
          )
        subPath =
          firstSlash === -1 ? '' : normalizedImportId.substring(firstSlash + 1)
      }

      subPath = path.posix.normalize(subPath).replace(/^(\.\/|\.|\/)/, '')
      const pkgNameRegex =
        /^(@[a-z0-9-~][a-z0-9-._~]*\/)?[a-z0-9-~][a-z0-9-._~]*$/
      if (!pkgNameRegex.test(pkgID) || !config.webPkgIDs.includes(pkgID)) {
        return null
      }

      if (debug)
        console.log(
          `[bldr-pkg-resolve] Processing: ${pkgID}, subpath: ${subPath}`,
        )

      // Resolve the full import path
      const resolvedImport = await this.resolve(importId, importer, {
        skipSelf: true,
        ...options,
      })
      if (!resolvedImport) {
        if (debug)
          console.log(`[bldr-pkg-resolve] Failed to resolve: ${importId}`)
        return null
      }

      const importPath = resolvedImport.id
      if (debug) console.log(`[bldr-pkg-resolve] Resolved path: ${importPath}`)

      // Infer package root from the resolved import path
      const pkgRoot = path.dirname(importPath)
      let relSubPath = subPath // Default to the parsed subpath

      // Handle case with no subpath (e.g., package entry point)
      if (!subPath) {
        relSubPath = path.basename(importPath)
      } else {
        // Verify the resolved path aligns with the expected package
        const expectedPrefix = pkgID.replace(/^@/, '').replace(/\//g, '-')
        if (!importPath.includes(expectedPrefix)) {
          if (debug)
            console.log(
              `[bldr-pkg-resolve] Resolved path does not match package: ${importPath}`,
            )
          return null
        }
        relSubPath = path.relative(pkgRoot, importPath).replace(/\\/g, '/')
        relSubPath = path.posix
          .normalize(relSubPath)
          .replace(/^(\.\/|\.|\/)/, '')
      }

      if (debug)
        console.log(
          `[bldr-pkg-resolve] Package root: ${pkgRoot}, Relative subpath: ${relSubPath}`,
        )

      // Check if the import resolves outside the package
      if (relSubPath.startsWith('../')) {
        const errorMsg = `Web package ${pkgID} import ${subPath} resolved outside package: ${relSubPath}`
        if (debug) console.error(`[bldr-pkg-resolve] Error: ${errorMsg}`)
        this.error(errorMsg)
      }

      // Remap extension to .mjs if applicable
      let finalSubPath = relSubPath
      const ext = path.extname(relSubPath)
      if (JS_EXTENSIONS.includes(ext)) {
        finalSubPath =
          finalSubPath.substring(0, finalSubPath.length - ext.length) + '.mjs'
        if (debug)
          console.log(`[bldr-pkg-resolve] Remapped to: ${finalSubPath}`)
      }

      // Construct the remapped external path
      const remappedPath = `${WEB_PKG_PREFIX}${pkgID}${finalSubPath ? '/' + finalSubPath : ''}`
      if (debug) console.log(`[bldr-pkg-resolve] Remapped to: ${remappedPath}`)

      // Optional callback and return the externalized import
      if (config.addWebPkgImport) {
        config.addWebPkgImport(pkgID, pkgRoot, relSubPath)
      }
      return { id: remappedPath, external: true }
    },
  }
}
