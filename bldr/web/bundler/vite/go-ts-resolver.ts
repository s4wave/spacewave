import { dirname, isAbsolute, resolve } from 'path'
import { access } from 'node:fs/promises'
import { type Alias, type Plugin } from 'vite'

const localModulePrefix = 'github.com/s4wave/spacewave/'

function resolveGoImportPath(projectRoot: string, source: string): string | null {
  if (!source.startsWith('@go/')) {
    return null
  }

  const importPath = source.slice('@go/'.length)
  if (importPath.startsWith(localModulePrefix)) {
    return resolve(projectRoot, importPath.slice(localModulePrefix.length))
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

// buildGoAliases builds Vite aliases for vendored and monorepo-local @go imports.
export function buildGoAliases(projectRoot: string): Alias[] {
  return [
    {
      find: /^@go\/github\.com\/s4wave\/spacewave\/(.*)$/,
      replacement: resolve(projectRoot, '$1'),
    },
    {
      find: /^@go\/(.*)$/,
      replacement: resolve(projectRoot, 'vendor', '$1'),
    },
  ]
}

/**
 * Creates a Vite plugin that resolves @go/ paths that end in .js to their .ts equivalents
 * when the .ts file exists but the .js file doesn't
 */
export function goTsResolver(projectRoot: string): Plugin {
  return {
    name: 'go-ts-resolver',
    enforce: 'pre',
    async resolveId(source, importer) {
      // Handle only .js imports that may map to source .ts files.
      if (!source.endsWith('.js')) {
        return null
      }

      const sourcePath =
        resolveGoImportPath(projectRoot, source) ??
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
