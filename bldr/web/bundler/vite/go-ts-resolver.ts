import { dirname, isAbsolute, resolve } from 'path'
import { access } from 'node:fs/promises'
import { readFileSync } from 'node:fs'
import { type Alias, type Plugin } from 'vite'

// readLocalModuleSync reads the module path from projectRoot/go.mod so the
// @go/<module>/... alias can resolve to local source instead of vendor.
// Returns null when go.mod is absent or has no `module` line, in which case
// every @go/ import falls back to vendor.
function readLocalModuleSync(projectRoot: string): string | null {
  let content: string
  try {
    content = readFileSync(resolve(projectRoot, 'go.mod'), 'utf-8')
  } catch {
    return null
  }
  const match = content.match(/^\s*module\s+(\S+)/m)
  return match ? match[1] : null
}

// GoModule is a Go module root and its declared module path.
interface GoModule {
  root: string
  path: string
}

// findGoModule returns the module containing dir by walking up to the nearest
// go.mod, caching the answer for every directory visited.
function findGoModule(
  dir: string,
  cache: Map<string, GoModule | null>,
): GoModule | null {
  const visited: string[] = []
  let found: GoModule | null = null
  for (let cur = dir; ; cur = dirname(cur)) {
    const cached = cache.get(cur)
    if (cached !== undefined) {
      found = cached
      break
    }
    visited.push(cur)
    const path = readLocalModuleSync(cur)
    if (path) {
      found = { root: cur, path }
      break
    }
    if (dirname(cur) === cur) break
  }
  for (const cur of visited) cache.set(cur, found)
  return found
}

function vendorRoots(
  projectRoot: string,
  distRoot: string,
  importerModule: GoModule | null,
): string[] {
  const roots = [projectRoot, distRoot]
  if (importerModule) roots.push(importerModule.root)
  return [...new Set(roots.map((root) => resolve(root, 'vendor')))]
}

// resolveGoImportPaths returns the candidate sources of an @go/ import. An
// import of the project module or of the importer's own module resolves to
// that module's sources; any other import resolves through the vendor trees.
// The importer's module matters when the build root reaches source outside the
// project, such as an app startup module bundled into the Bldr renderer.
function resolveGoImportPaths(
  projectRoot: string,
  distRoot: string,
  localModule: string | null,
  importerModule: GoModule | null,
  source: string,
): string[] | null {
  if (!source.startsWith('@go/')) {
    return null
  }

  const importPath = source.slice('@go/'.length)
  if (localModule && importPath.startsWith(localModule + '/')) {
    return [resolve(projectRoot, importPath.slice(localModule.length + 1))]
  }
  if (importerModule && importPath.startsWith(importerModule.path + '/')) {
    return [
      resolve(
        importerModule.root,
        importPath.slice(importerModule.path.length + 1),
      ),
    ]
  }

  return vendorRoots(projectRoot, distRoot, importerModule).map((root) =>
    resolve(root, importPath),
  )
}

function resolveSourcePaths(
  projectRoot: string,
  distRoot: string,
  source: string,
  importer?: string,
): string[] | null {
  if (source.startsWith('@go/')) {
    return null
  }
  if (source.startsWith('vendor/')) {
    const importPath = source.slice('vendor/'.length)
    return vendorRoots(projectRoot, distRoot, null).map((root) =>
      resolve(root, importPath),
    )
  }
  if (isAbsolute(source)) {
    return [source]
  }
  if (!source.startsWith('.')) {
    return null
  }
  if (!importer) {
    return null
  }

  return [resolve(dirname(importer), source)]
}

// buildGoAliases builds Vite aliases for monorepo-local @go imports that do
// not go through the generated .js-to-.ts resolver. Vendored @go imports are
// resolved by goTsResolver so external apps can fall back to the Bldr dist
// vendor tree when they do not materialize an app-root vendor mirror.
export function buildGoAliases(
  projectRoot: string,
  _distRoot = projectRoot,
): Alias[] {
  const aliases: Alias[] = []
  const localModule = readLocalModuleSync(projectRoot)
  if (localModule) {
    const escaped = localModule.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    aliases.push({
      find: new RegExp(`^@go\\/${escaped}\\/(.*)$`),
      replacement: resolve(projectRoot, '$1'),
    })
  }
  return aliases
}

/**
 * Creates a Vite plugin that resolves @go/ paths that end in .js to their .ts equivalents
 * when the .ts file exists but the .js file doesn't
 */
export function goTsResolver(
  projectRoot: string,
  distRoot = projectRoot,
): Plugin {
  const localModule = readLocalModuleSync(projectRoot)
  const tsPathCache = new Map<string, Promise<string | null>>()
  const moduleCache = new Map<string, GoModule | null>()
  return {
    name: 'go-ts-resolver',
    enforce: 'pre',
    buildStart() {
      tsPathCache.clear()
      moduleCache.clear()
    },
    watchChange() {
      tsPathCache.clear()
      moduleCache.clear()
    },
    async resolveId(source, importer) {
      // Handle only .js imports that may map to source .ts files.
      if (!source.endsWith('.js')) {
        return null
      }

      const importerModule =
        importer && isAbsolute(importer)
          ? findGoModule(dirname(importer), moduleCache)
          : null
      const sourcePaths =
        resolveGoImportPaths(
          projectRoot,
          distRoot,
          localModule,
          importerModule,
          source,
        ) ?? resolveSourcePaths(projectRoot, distRoot, source, importer)
      if (!sourcePaths) {
        return null
      }

      for (const sourcePath of sourcePaths) {
        const tsPath = resolve(projectRoot, sourcePath).replace(/\.js$/, '.ts')

        let cached = tsPathCache.get(tsPath)
        if (!cached) {
          cached = access(tsPath).then(
            () => tsPath,
            () => null,
          )
          tsPathCache.set(tsPath, cached)
        }
        const resolved = await cached
        if (resolved) {
          return resolved
        }
      }
      return null
    },
  }
}
