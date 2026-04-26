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

function resolveGoImportPath(
  projectRoot: string,
  localModule: string | null,
  source: string,
): string | null {
  if (!source.startsWith('@go/')) {
    return null
  }

  const importPath = source.slice('@go/'.length)
  if (localModule && importPath.startsWith(localModule + '/')) {
    return resolve(projectRoot, importPath.slice(localModule.length + 1))
  }

  return resolve(projectRoot, 'vendor', importPath)
}

function resolveSourcePath(source: string, importer?: string): string | null {
  if (source.startsWith('@go/')) {
    return null
  }
  if (isAbsolute(source)) {
    return source
  }
  if (!source.startsWith('.')) {
    return null
  }
  if (!importer) {
    return null
  }

  return resolve(dirname(importer), source)
}

// buildGoAliases builds Vite aliases for vendored and monorepo-local @go
// imports. The local module path is read from projectRoot/go.mod so the same
// helper works for any repo consuming bldr; @go/<module>/* maps to project
// source while every other @go/* falls back to projectRoot/vendor.
export function buildGoAliases(projectRoot: string): Alias[] {
  const aliases: Alias[] = []
  const localModule = readLocalModuleSync(projectRoot)
  if (localModule) {
    const escaped = localModule.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    aliases.push({
      find: new RegExp(`^@go\\/${escaped}\\/(.*)$`),
      replacement: resolve(projectRoot, '$1'),
    })
  }
  aliases.push({
    find: /^@go\/(.*)$/,
    replacement: resolve(projectRoot, 'vendor', '$1'),
  })
  return aliases
}

/**
 * Creates a Vite plugin that resolves @go/ paths that end in .js to their .ts equivalents
 * when the .ts file exists but the .js file doesn't
 */
export function goTsResolver(projectRoot: string): Plugin {
  const localModule = readLocalModuleSync(projectRoot)
  return {
    name: 'go-ts-resolver',
    enforce: 'pre',
    async resolveId(source, importer) {
      // Handle only .js imports that may map to source .ts files.
      if (!source.endsWith('.js')) {
        return null
      }

      const sourcePath =
        resolveGoImportPath(projectRoot, localModule, source) ??
        resolveSourcePath(source, importer)
      if (!sourcePath) {
        return null
      }

      const tsPath = sourcePath.replace(/\.js$/, '.ts')

      try {
        await access(tsPath)
        return tsPath
      } catch {
        return null
      }
    },
  }
}
