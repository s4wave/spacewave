import path from 'path'
import fs from 'fs'
import { Plugin } from 'vite'
import type { ResolveIdResult } from 'rollup'

export interface WebPkgRemapPluginConfig {
  webPkgIDs: string[] // List of packages that can be bundled as web pkgs
  addWebPkgImport?: (
    webPkgID: string,
    webPkgRoot: string,
    webPkgSubPath: string,
  ) => void // Optional callback
  debug?: boolean // Enable debug logging
}

export function createWebPkgRemapPlugin(
  config: WebPkgRemapPluginConfig,
): Plugin {
  const WEB_PKG_PREFIX = '/b/pkg/' // Prefix for remapped external imports
  const debug = config.debug || false

  // Helper function to find package root by walking up the directory tree
  async function findPkgRoot(startPath: string): Promise<string | null> {
    let currentDir = startPath
    while (currentDir !== path.parse(currentDir).root) {
      const pkgJsonPath = path.join(currentDir, 'package.json')
      if (fs.existsSync(pkgJsonPath)) {
        return currentDir
      }
      currentDir = path.dirname(currentDir)
    }
    return null
  }

  return {
    name: 'bldr-pkg-resolve',
    enforce: 'pre',
    apply: 'build',

    async resolveId(importId, importer, options): Promise<ResolveIdResult> {
      // Skip self-resolution or relative imports
      if (importer === 'bldr-pkg-resolve' || importId?.startsWith('.')) {
        return null
      }

      if (debug) {
        console.log(
          `[bldr-pkg-resolve] Resolving: ${importId} from ${importer}`,
        )
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
        if (debug) console.log(`[bldr-pkg-resolve] Skipping: ${pkgID}`)
        return null
      }

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

      // Step 1: Try resolving package.json to find the package root
      let pkgRoot: string | null = null
      const pkgJsonResolve = await this.resolve(
        `${pkgID}/package.json`,
        importer,
        {
          skipSelf: true,
          ...options,
        },
      )
      if (pkgJsonResolve) {
        pkgRoot = path.dirname(pkgJsonResolve.id)
      } else {
        // Step 2: Fallback to finding package.json by walking up from the import path
        pkgRoot = await findPkgRoot(path.dirname(importPath))
        if (!pkgRoot) {
          if (debug)
            console.log(
              `[bldr-pkg-resolve] Failed to find package root for: ${pkgID}`,
            )
          return null
        }
      }

      if (debug) console.log(`[bldr-pkg-resolve] Package root: ${pkgRoot}`)

      // Step 3: Compute the relative subpath from package root to import path
      let relSubPath = path.relative(pkgRoot, importPath).replace(/\\/g, '/')
      relSubPath = path.posix.normalize(relSubPath).replace(/^(\.\/|\.|\/)/, '')
      if (debug)
        console.log(`[bldr-pkg-resolve] Relative subpath: ${relSubPath}`)

      // Check if the import resolves outside the package
      if (relSubPath.startsWith('../')) {
        const errorMsg = `Web package ${pkgID} import ${subPath} resolved outside package: ${relSubPath}`
        if (debug) console.error(`[bldr-pkg-resolve] Error: ${errorMsg}`)
        this.error(errorMsg)
      }

      // Step 4: Remap extension to .mjs if applicable
      let finalSubPath = relSubPath
      const ext = path.extname(relSubPath)
      if (['.js', '.cjs', '.jsx', '.ts', '.tsx'].includes(ext)) {
        finalSubPath =
          finalSubPath.substring(0, finalSubPath.length - ext.length) + '.mjs'
        if (debug)
          console.log(`[bldr-pkg-resolve] Remapped to: ${finalSubPath}`)
      }

      // Step 5: Construct the remapped external path
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
