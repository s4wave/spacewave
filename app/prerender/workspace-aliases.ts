import type { Plugin } from 'vite'
import { resolve } from 'path'

export interface WorkspaceAlias {
  find: string | RegExp
  replacement: string
}

// buildWorkspaceAliases maps the @s4wave workspace packages onto their repo
// paths so dev, hydrate, and SSR builds resolve identically.
export function buildWorkspaceAliases(projectRoot: string): WorkspaceAlias[] {
  return [
    {
      find: /^@s4wave\/core\/(.*)$/,
      replacement: resolve(projectRoot, './core/$1'),
    },
    {
      find: /^@s4wave\/sdk\/(.*)$/,
      replacement: resolve(projectRoot, './sdk/$1'),
    },
    {
      find: '@s4wave/sdk',
      replacement: resolve(projectRoot, './sdk'),
    },
    {
      find: /^@s4wave\/app\/(.*)$/,
      replacement: resolve(projectRoot, './app/$1'),
    },
    {
      find: '@s4wave/app',
      replacement: resolve(projectRoot, './app'),
    },
    {
      find: /^@s4wave\/web\/(.*)$/,
      replacement: resolve(projectRoot, './web/$1'),
    },
    {
      find: '@s4wave/web',
      replacement: resolve(projectRoot, './web'),
    },
  ]
}

// staticAssetPlugin resolves image imports to /static/assets/<basename> so
// the hydrate and SSR prerender bundles produce the same asset URLs as
// static serving.
export function staticAssetPlugin(): Plugin {
  return {
    name: 'prerender-static-assets',
    enforce: 'pre',
    resolveId(source) {
      if (/\.(png|svg|jpg|gif|ico)(\?.*)?$/.test(source)) {
        return { id: '\0static-asset:' + source, moduleSideEffects: false }
      }
      return null
    },
    load(id) {
      if (!id.startsWith('\0static-asset:')) return null
      const source = id.slice('\0static-asset:'.length)
      const basename = source.split('/').pop()?.replace(/\?.*$/, '') ?? ''
      return `export default "/static/assets/${basename}";`
    },
  }
}
