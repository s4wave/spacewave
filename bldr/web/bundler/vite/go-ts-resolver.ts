import { dirname, isAbsolute, resolve } from 'path'
import { access } from 'node:fs/promises'
import { readFileSync, readdirSync } from 'node:fs'
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

function vendorRoots(projectRoot: string, distRoot: string): string[] {
  const appVendorRoot = resolve(projectRoot, 'vendor')
  const distVendorRoot = resolve(distRoot, 'vendor')
  if (appVendorRoot === distVendorRoot) {
    return [appVendorRoot]
  }
  return [appVendorRoot, distVendorRoot]
}

function resolveMaterializedScopedPackagePaths(
  projectRoot: string,
  distRoot: string,
  source: string,
): string[] | null {
  if (!source.startsWith('@')) {
    return null
  }
  const parts = source.split('/')
  if (parts.length < 3) {
    return null
  }
  const scope = parts[0].slice(1)
  const name = parts[1]
  if (!scope || !name) {
    return null
  }
  const rel = parts.slice(2).join('/')
  const roots = [resolve(projectRoot, `.${scope}`, name)]
  const distRootCandidate = resolve(distRoot, `.${scope}`, name)
  if (distRootCandidate !== roots[0]) {
    roots.push(distRootCandidate)
  }
  return roots.map((root) => resolve(root, rel))
}

function resolveGoImportPaths(
  projectRoot: string,
  distRoot: string,
  localModule: string | null,
  source: string,
): string[] | null {
  if (!source.startsWith('@go/')) {
    return null
  }

  const importPath = source.slice('@go/'.length)
  if (localModule && importPath.startsWith(localModule + '/')) {
    return [resolve(projectRoot, importPath.slice(localModule.length + 1))]
  }

  return vendorRoots(projectRoot, distRoot).map((root) =>
    resolve(root, importPath),
  )
}

function buildMaterializedScopedPackageAliases(projectRoot: string): Alias[] {
  const aliases: Alias[] = []
  let entries: string[]
  try {
    entries = readdirSync(projectRoot)
  } catch {
    return aliases
  }
  for (const entry of entries) {
    if (!entry.startsWith('.') || entry === '.' || entry === '..') {
      continue
    }
    const scope = entry.slice(1)
    if (!scope) {
      continue
    }
    let packages: string[]
    try {
      packages = readdirSync(resolve(projectRoot, entry))
    } catch {
      continue
    }
    for (const pkg of packages) {
      aliases.push({
        find: `@${scope}/${pkg}`,
        replacement: resolve(projectRoot, entry, pkg),
      })
    }
  }
  return aliases
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
    return vendorRoots(projectRoot, distRoot).map((root) =>
      resolve(root, importPath),
    )
  }
  const materializedPackagePaths = resolveMaterializedScopedPackagePaths(
    projectRoot,
    distRoot,
    source,
  )
  if (materializedPackagePaths) {
    return materializedPackagePaths
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
  aliases.push(...buildMaterializedScopedPackageAliases(projectRoot))
  return aliases
}

/**
 * Creates a Vite plugin that resolves @go/ paths ending in .js to existing
 * generated .ts/.tsx siblings or checked-in .js files.
 */
export function goTsResolver(
  projectRoot: string,
  distRoot = projectRoot,
): Plugin {
  const localModule = readLocalModuleSync(projectRoot)
  const tsPathCache = new Map<string, Promise<string | null>>()
  return {
    name: 'go-ts-resolver',
    enforce: 'pre',
    buildStart() {
      tsPathCache.clear()
    },
    watchChange() {
      tsPathCache.clear()
    },
    async resolveId(source, importer) {
      // Handle only .js imports that may map to source .ts files.
      if (!source.endsWith('.js')) {
        return null
      }

      const sourcePaths =
        resolveGoImportPaths(projectRoot, distRoot, localModule, source) ??
        resolveSourcePaths(projectRoot, distRoot, source, importer)
      if (!sourcePaths) {
        return null
      }

      for (const sourcePath of sourcePaths) {
        const candidates = [
          sourcePath.replace(/\.js$/, '.ts'),
          sourcePath.replace(/\.js$/, '.tsx'),
          sourcePath,
        ]
        for (const candidatePath of candidates) {
          const resolvedPath = resolve(candidatePath)
          let cached = tsPathCache.get(resolvedPath)
          if (!cached) {
            cached = access(resolvedPath).then(
              () => resolvedPath,
              () => null,
            )
            tsPathCache.set(resolvedPath, cached)
          }
          const resolved = await cached
          if (resolved) {
            return resolved
          }
        }
      }
      return null
    },
  }
}
