import { resolve } from 'path'
import { access } from 'node:fs/promises'
import { Plugin } from 'vite'

/**
 * Creates a Vite plugin that resolves @go/ paths that end in .js to their .ts equivalents
 * when the .ts file exists but the .js file doesn't
 */
export function goTsResolver(projectRoot: string): Plugin {
  return {
    name: 'go-ts-resolver',
    async resolveId(source) {
      // Handle only @go/ paths that end in .js
      if (!source.startsWith('@go/') || !source.endsWith('.js')) {
        return null
      }

      // Convert @go/ path to vendor path
      const vendorPath = resolve(
        projectRoot,
        './vendor',
        source.slice('@go/'.length),
      )
      const tsPath = vendorPath.replace(/\.js$/, '.ts')

      try {
        await access(tsPath)
        return tsPath
      } catch {
        return null
      }
    },
  }
}
