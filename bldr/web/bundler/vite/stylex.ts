import { existsSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  parseSync,
  transformFromAstSync,
  type BabelFileResult,
} from '@babel/core'
import stylexPlugin from '@stylexjs/babel-plugin'
import type { Plugin, ResolvedConfig, ModuleNode } from 'vite'

import { stylexLayersBefore } from '../../bldr/stylex-priorities.js'

// cssSuffix gives extracted modules a native CSS extension.
const cssSuffix = '.bldr-stylex.css'
// sourceRuntimeFile initializes one shared document order in every browser entry path.
const sourceRuntimeFile = fileURLToPath(
  new URL('../../bldr/stylex-client.ts', import.meta.url),
)
// Rules are the official compiler metadata consumed by its CSS extractor.
type Rules = Parameters<typeof stylexPlugin.processStylexRules>[0]
// CompiledModule retains one compiler result and its resolved theme dependencies.
interface CompiledModule {
  source: string
  code: string
  map: BabelFileResult['map']
  rules: Rules
  dependencies: string[]
}

/** stylexCss exposes compiled StyleX rules through Vite's native CSS lifecycle. */
export function stylexCss(distRoot?: string): Plugin {
  // Bundled compiler services resolve their shipped sources independently of the host script.
  const runtimeFile = distRoot
    ? resolve(
        distRoot,
        existsSync(resolve(distRoot, 'bldr')) ? 'bldr' : '.',
        'web/bldr/stylex-client.ts',
      )
    : sourceRuntimeFile

  // The compiler retains current results; Vite owns assets and dependency updates.
  const modules = new Map<
    string,
    { source: string; promise: Promise<CompiledModule> } | null
  >()
  const owner = {}
  let enabled = true
  let config: ResolvedConfig
  let resolveTheme: ReturnType<ResolvedConfig['createResolver']>

  // compile resolves theme imports before running the official compiler once.
  const compileSource = async (
    source: string,
    file: string,
  ): Promise<CompiledModule> => {
    // Resolve this source snapshot without publishing into a newer request's cache slot.
    const ast = parseSync(source, {
      filename: file,
      configFile: false,
      babelrc: false,
      parserOpts: {
        plugins: /\.[cm]?ts$/.test(file)
          ? ['typescript']
          : ['typescript', 'jsx'],
      },
    })!
    const aliases: Record<string, string[]> = {}
    for (const node of ast.program.body) {
      if (node.type !== 'ImportDeclaration' || node.importKind === 'type')
        continue
      const specifier = node.source.value
      if (
        !/\.(?:stylex(?:\.const)?|transformed)(?:\.[cm]?[jt]s)?$/.test(
          specifier,
        )
      )
        continue
      const resolved = await resolveTheme(specifier, file)
      if (!resolved)
        throw new Error(`Cannot resolve StyleX theme ${specifier} from ${file}`)
      aliases[specifier] = [resolved.split('?')[0]!]
    }

    // Babel's constant placeholders need metadata from the referenced modules at extraction.
    const result = transformFromAstSync(ast, source, {
      filename: file,
      configFile: false,
      babelrc: false,
      cloneInputAst: false,
      sourceMaps: true,
      plugins: [
        [
          stylexPlugin,
          {
            dev: false,
            runtimeInjection: false,
            treeshakeCompensation: true,
            aliases,
            unstable_moduleResolution: {
              type: 'commonJS',
              rootDir: config.root,
            },
          },
        ],
      ],
    })!
    const compiled: CompiledModule = {
      source,
      code: result.code!,
      map: result.map,
      rules: (result.metadata as unknown as { stylex?: Rules }).stylex ?? [],
      dependencies: [...new Set(Object.values(aliases).flat())],
    }
    return compiled
  }

  // compile installs the promise before awaiting resolution; old completions cannot replace it.
  const compile = (source: string, file: string): Promise<CompiledModule> => {
    // Share the in-flight request and publish ownership before it can complete.
    const cached = modules.get(file)
    if (cached?.source === source) return cached.promise
    const promise = compileSource(source, file)
    modules.set(file, { source, promise })
    return promise
  }

  // readModule rechecks ownership after disk I/O so an old read cannot replace a new compile.
  const readModule = async (file: string): Promise<CompiledModule> => {
    const cached = modules.get(file)
    if (cached) return cached.promise
    const source = await readFile(file, 'utf8')
    return modules.get(file)?.promise ?? compile(source, file)
  }

  // extractCss includes only this module's rules and the constants its imports require.
  const extractCss = async (file: string): Promise<string> => {
    // Walk the theme graph once; cycles in variable references must not stall the loader.
    const rules: Rules = []
    const seen = new Set<string>()
    const pending = [file]
    while (pending.length) {
      const next = pending.pop()!
      if (seen.has(next)) continue
      seen.add(next)
      const module = await readModule(next)
      for (const rule of module.rules) {
        if (next === file || 'constKey' in rule[1]) rules.push(rule)
      }
      pending.push(...module.dependencies)
    }

    // The official extractor resolves constants, units and compiler priority groups.
    return stylexPlugin.processStylexRules(rules, {
      useLayers: { before: stylexLayersBefore, prefix: 'stylex' },
    })
  }

  // Bldr and a project config may both install the adapter; only the first compiles.
  const plugin: Plugin = {
    name: 'bldr-stylex-css',
    api: owner,
    enforce: 'pre',
    config: () => ({ optimizeDeps: { exclude: ['@stylexjs/stylex'] } }),
    configResolved(resolved) {
      config = resolved
      resolveTheme = resolved.createResolver()
      enabled =
        resolved.plugins.find((plugin) => plugin.name === 'bldr-stylex-css')
          ?.api === owner
    },
    buildStart() {
      modules.clear()
    },
    resolveId(source) {
      if (enabled && source.split('?')[0]!.endsWith(cssSuffix)) return source
    },
    async load(id) {
      // Only CSS payloads owned by the first adapter enter native extraction.
      if (!enabled) return
      const cssFile = id.split('?')[0]!
      if (!cssFile.endsWith(cssSuffix)) return
      const file = cssFile.slice(0, -cssSuffix.length)
      this.addWatchFile(file)
      return extractCss(file)
    },
    async transform(source, id) {
      // Duplicate adapters and CSS modules leave the owning pipeline untouched.
      if (!enabled || id.split('?')[0]!.endsWith(cssSuffix)) return
      const file = id.split('?')[0]!
      if (
        !/\.[cm]?[jt]sx?$/.test(file) ||
        (!source.includes('@stylexjs/stylex') && !modules.has(file))
      )
        return
      const compiled = await compile(source, file)
      const cssImports = [file, ...compiled.dependencies]
        .map(
          (dependency) => `import ${JSON.stringify(dependency + cssSuffix)};`,
        )
        .join('\n')
      return {
        code: `${compiled.code}\n${cssImports}\nimport ${JSON.stringify(runtimeFile)};`,
        map: compiled.map,
      }
    },
    async handleHotUpdate(context) {
      // Imported constants also reach consumers through their native CSS dependency edges.
      if (!enabled) return
      const affected = new Set<string>([context.file])
      const updates = new Set(context.modules)
      for (const module of context.server.moduleGraph.getModulesByFile(
        context.file + cssSuffix,
      ) ?? [])
        updates.add(module)
      if (/\.stylex(?:\.const)?\.[cm]?[jt]s$/.test(context.file)) {
        const seen = new Set<ModuleNode>()
        const visit = (module: ModuleNode) => {
          if (seen.has(module)) return
          seen.add(module)
          if (module.file && modules.has(module.file)) affected.add(module.file)
          for (const importer of module.importers) visit(importer)
        }
        for (const module of updates) visit(module)
      }

      // Invalid results stay pending so a subsequent correction can recover every module.
      for (const file of affected)
        if (modules.has(file)) modules.set(file, null)
      for (const file of affected) {
        if (!modules.has(file)) continue
        await compile(
          await (file === context.file
            ? context.read()
            : readFile(file, 'utf8')),
          file,
        )
        for (const changed of [file, file + cssSuffix]) {
          for (const module of context.server.moduleGraph.getModulesByFile(
            changed,
          ) ?? []) {
            context.server.moduleGraph.invalidateModule(module)
            updates.add(module)
          }
        }
      }
      return [...updates]
    },
  }
  return plugin
}
