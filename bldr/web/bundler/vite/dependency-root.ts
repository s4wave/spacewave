import path from 'node:path'
import type { InlineConfig } from 'vite'

// withDependencyRoot adds the installed Bldr dependency tree to Rolldown's
// module search roots without changing Vite's package or web-package routing.
export function withDependencyRoot(
  config: InlineConfig,
  dependencyRoot = process.env['BLDR_DEPENDENCY_ROOT'],
): InlineConfig {
  if (!dependencyRoot) return config

  const dependencyModules = path.resolve(dependencyRoot, 'node_modules')
  const build = config.build ?? {}
  const rolldownOptions = build.rolldownOptions ?? {}
  const resolveOptions = rolldownOptions.resolve ?? {}
  const modules = resolveOptions.modules ?? []
  if (modules.includes(dependencyModules)) return config

  return {
    ...config,
    build: {
      ...build,
      rolldownOptions: {
        ...rolldownOptions,
        resolve: {
          ...resolveOptions,
          modules: [...modules, dependencyModules],
        },
      },
    },
  }
}
