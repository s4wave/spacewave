import { loadConfigFromFile, mergeConfig, build as viteBuild } from 'vite'
import type {
  InlineConfig,
  LogErrorOptions,
  LogOptions,
  Logger,
  Rollup,
  UserConfig,
} from 'vite'
import { existsSync, promises as fs } from 'node:fs'
import path from 'node:path'
import type { ConfigEnv } from 'vitest/config'

// isBundleError checks if an unknown error is a Vite 8 BundleError with an errors array.
export function isBundleError(
  err: unknown,
): err is Error & { errors: Rollup.RollupError[] } {
  return (
    err instanceof Error &&
    'errors' in err &&
    Array.isArray((err as Record<string, unknown>).errors)
  )
}

// isRollupError checks if an unknown error is a RollupError by checking for the watchFiles property.
export function isRollupError(err: unknown): err is Rollup.RollupError {
  return (
    typeof err === 'object' &&
    err !== null &&
    'watchFiles' in err &&
    Array.isArray((err as Rollup.RollupError).watchFiles)
  )
}

// createSilentViteLogger suppresses build output while retaining warning state.
export function createSilentViteLogger(): Logger {
  return {
    hasWarned: false,
    info(msg: string, options?: LogOptions): void {
      void msg
      void options
    },
    warn(_msg: string, _options?: LogOptions): void {
      this.hasWarned = true
    },
    warnOnce(_msg: string, _options?: LogOptions): void {
      this.hasWarned = true
    },
    error(msg: string, options?: LogErrorOptions): void {
      void msg
      void options
    },
    clearScreen(type: 'error' | 'warn' | 'info'): void {
      void type
    },
    hasErrorLogged(error: Error | Rollup.RollupError): boolean {
      void error
      return false
    },
  }
}

// loadOptionalConfig retains existing configuration files and their dependencies.
async function loadOptionalConfig(
  configEnv: ConfigEnv,
  configPath: string,
): Promise<UserConfig | null> {
  if (!existsSync(configPath)) {
    return null
  }

  // Resolve configuration imports at their source, without temporary modules in
  // the shared dependency install. The runner retains imported-file tracking.
  const loadedConfig = await loadConfigFromFile(
    configEnv,
    configPath,
    undefined,
    'silent',
    createSilentViteLogger(),
    'runner',
  )
  if (!loadedConfig) {
    return null
  }

  // Configuration imports affect output even when absent from the module graph.
  return {
    ...loadedConfig.config,
    plugins: [
      ...(loadedConfig.config.plugins ?? []),
      {
        name: 'bldr-config-inputs',
        configResolved(config) {
          config.configFileDependencies.push(
            loadedConfig.path,
            ...loadedConfig.dependencies,
          )
        },
      },
    ],
  }
}

// buildConfig merges configuration files in the supplied order.
export async function buildConfig(
  configEnv: ConfigEnv,
  ...additionalConfigPaths: string[]
): Promise<UserConfig> {
  let mergedConfig: UserConfig = {}
  for (const configPath of additionalConfigPaths) {
    const additionalConfig = await loadOptionalConfig(configEnv, configPath)
    if (additionalConfig) {
      mergedConfig = mergeConfig(mergedConfig, additionalConfig)
    }
  }
  return mergedConfig
}

interface ViteManifestEntry {
  file: string
  css?: string[]
  assets?: string[]
  imports?: string[]
  isEntry?: boolean
  src?: string
}

type ViteManifest = Record<string, ViteManifestEntry>

type ViteOutputChunkWithMetadata = Rollup.OutputChunk & {
  referencedFiles?: string[]
  viteMetadata?: {
    importedCss?: Set<string>
  }
}

// collectReferencedFiles collects CSS dependencies without revisiting cycles.
function collectReferencedFiles(
  entryKey: string,
  manifest: ViteManifest,
  seen = new Set<string>(),
  cssFiles = new Set<string>(),
): { cssFiles: Set<string> } {
  if (seen.has(entryKey)) return { cssFiles }
  seen.add(entryKey)

  const entry = manifest[entryKey]
  if (!entry) return { cssFiles }

  entry.css?.forEach((c) => cssFiles.add(c))

  entry.imports?.forEach((imp) =>
    collectReferencedFiles(imp, manifest, seen, cssFiles),
  )

  return { cssFiles }
}

// readManifest accepts both supported Vite manifest locations.
async function readManifest(outDir: string): Promise<ViteManifest | null> {
  const manifestPaths = [
    path.join(outDir, '.vite/manifest.json'),
    path.join(outDir, 'manifest.json'),
  ]
  for (const manifestPath of manifestPaths) {
    if (!existsSync(manifestPath)) {
      continue
    }
    return JSON.parse(await fs.readFile(manifestPath, 'utf-8'))
  }
  return null
}

// synthesizeManifest derives entry metadata when Vite emits no manifest.
function synthesizeManifest(
  outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[],
  rootDir: string,
): ViteManifest {
  const manifest: ViteManifest = {}
  for (const output of outputChunks) {
    if (output.type !== 'chunk' || !output.isEntry) {
      continue
    }
    const chunk = output as ViteOutputChunkWithMetadata
    const entrypoint = chunk.facadeModuleId
      ? normalizeModuleId(chunk.facadeModuleId, rootDir)
      : chunk.name
    if (!entrypoint) {
      continue
    }
    const css = [
      ...(chunk.viteMetadata?.importedCss ?? []),
      ...(chunk.referencedFiles ?? []).filter((file) => file.endsWith('.css')),
    ]
    manifest[entrypoint] = {
      file: chunk.fileName,
      css: Array.from(new Set(css)),
      isEntry: true,
      src: entrypoint,
    }
  }
  return manifest
}

// normalizeModuleId returns a source-relative path and excludes virtual modules.
function normalizeModuleId(id: string, rootDir: string): string | null {
  if (id.startsWith('\x00')) {
    return null
  }
  const withoutQuery = id.split('?')[0]
  if (withoutQuery.startsWith('__vite-browser-external')) {
    return null
  }

  let relPath = withoutQuery
  if (path.isAbsolute(withoutQuery)) {
    relPath = path.relative(rootDir, withoutQuery)
  }

  relPath = path.normalize(relPath)
  if (!relPath.startsWith('..')) {
    return relPath
  }

  return resolveEscapedModuleId(relPath, rootDir) ?? relPath
}

// resolveEscapedModuleId repairs emitted paths that traverse beyond the source root.
function resolveEscapedModuleId(
  relPath: string,
  rootDir: string,
): string | null {
  const parts = relPath.split(/[\\/]+/).filter((part) => part && part !== '.')
  for (let i = 0; i < parts.length; i++) {
    if (parts[i] === '..') {
      continue
    }
    const candidate = path.join(...parts.slice(i))
    if (existsSync(path.join(rootDir, candidate))) {
      return path.normalize(candidate)
    }
  }
  return null
}

// buildChunkImportsMap includes static and dynamic dependencies for source tracking.
// Lazy chunks remain build inputs even when they are not loaded at startup.
function buildChunkImportsMap(
  outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[],
): Map<string, string[]> {
  const chunkImports = new Map<string, string[]>()
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk' && chunk.fileName) {
      chunkImports.set(chunk.fileName, [
        ...(chunk.imports ?? []),
        ...(chunk.dynamicImports ?? []),
      ])
    }
  }
  return chunkImports
}

// collectAllModulesForChunk collects transitive source inputs without revisiting cycles.
function collectAllModulesForChunk(
  chunkFileName: string,
  jsChunkToModules: Map<string, Set<string>>,
  chunkImports: Map<string, string[]>,
  visited: Set<string> = new Set(),
): Set<string> {
  if (visited.has(chunkFileName)) {
    return new Set()
  }
  visited.add(chunkFileName)

  const allModules = new Set<string>()
  const directModules = jsChunkToModules.get(chunkFileName)
  if (directModules) {
    directModules.forEach((m) => allModules.add(m))
  }

  const imports = chunkImports.get(chunkFileName) || []
  for (const importedChunk of imports) {
    const importedModules = collectAllModulesForChunk(
      importedChunk,
      jsChunkToModules,
      chunkImports,
      visited,
    )
    importedModules.forEach((m) => allModules.add(m))
  }

  return allModules
}

// analyzeManifest extracts entrypoints and their corresponding files from the build output.
export async function analyzeManifest(
  outDir: string,
  outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[],
  rootDir: string,
) {
  const manifest =
    (await readManifest(outDir)) ?? synthesizeManifest(outputChunks, rootDir)

  const jsChunkToModules = new Map<string, Set<string>>()
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk' && chunk.fileName) {
      const modules = new Set<string>()

      if (chunk.facadeModuleId) {
        const normalized = normalizeModuleId(chunk.facadeModuleId, rootDir)
        if (normalized) {
          modules.add(normalized)
        }
      }

      let foundModules = false
      if (chunk.moduleIds && chunk.moduleIds.length > 0) {
        foundModules = true
        chunk.moduleIds.forEach((id) => {
          const normalized = normalizeModuleId(id, rootDir)
          if (normalized) {
            modules.add(normalized)
          }
        })
      } else if (chunk.modules && Object.keys(chunk.modules).length > 0) {
        foundModules = true
        Object.keys(chunk.modules).forEach((id) => {
          const normalized = normalizeModuleId(id, rootDir)
          if (normalized) {
            modules.add(normalized)
          }
        })
      }

      if (process.env.DEBUG_VITE_MODULES && !foundModules) {
        console.warn(
          `[vite] Warning: chunk ${chunk.fileName} has no moduleIds - transitive deps may not be tracked`,
        )
      }

      jsChunkToModules.set(chunk.fileName, modules)
    }
  }

  const chunkImports = buildChunkImportsMap(outputChunks)

  const entrypointOutputs = Object.entries(manifest).flatMap(([key, value]) => {
    if (!value.isEntry) return []
    const allModules = collectAllModulesForChunk(
      value.file,
      jsChunkToModules,
      chunkImports,
    )
    const entrypointPath = value.src ?? key
    return [
      {
        entrypoint: path.isAbsolute(entrypointPath)
          ? path.relative(rootDir, entrypointPath)
          : entrypointPath,
        outputs: {
          js: value.file,
          css: new Set<string>(value.css ?? []),
        },
        inputs: allModules,
      },
    ]
  })

  const allCssAssets = new Set<string>()
  const handledCssAssets = new Set<string>()
  const entrypointOutputByEntrypoint = new Map(
    entrypointOutputs.map((entry) => [entry.entrypoint, entry]),
  )

  for (const chunk of outputChunks) {
    if (chunk.type === 'asset' && chunk.fileName.endsWith('.css')) {
      allCssAssets.add(chunk.fileName)
    }
  }

  // Associate CSS files with entrypoints using manifest data. Non-CSS Vite
  // assets are copied from the output directory, but they must not share the
  // entrypoint metadata path: JS assets there replace the real React entrypoint.
  for (const [key, value] of Object.entries(manifest)) {
    if (!value.isEntry) continue

    const { cssFiles } = collectReferencedFiles(key, manifest)

    const entrypointPath = value.src ?? key
    const normalizedEntrypoint = path.isAbsolute(entrypointPath)
      ? path.relative(rootDir, entrypointPath)
      : entrypointPath
    const entryOutput = entrypointOutputByEntrypoint.get(normalizedEntrypoint)

    if (entryOutput) {
      cssFiles.forEach((cssFile) => {
        entryOutput.outputs.css.add(cssFile)
        handledCssAssets.add(cssFile)
      })
    }
  }

  const finalEntrypointOutputs = entrypointOutputs.map((entry) => ({
    entrypoint: entry.entrypoint,
    outputs: {
      js: entry.outputs.js,
      css: Array.from(entry.outputs.css),
    },
    inputs: Array.from(entry.inputs),
  }))

  const globalCssFiles = [...allCssAssets].filter(
    (css) => !handledCssAssets.has(css),
  )

  return {
    entrypointOutputs: finalEntrypointOutputs,
    globalCssFiles,
  }
}

// runBuild builds once using the supplied configuration file.
export async function runBuild(
  configFile: string,
  mode: string = 'development',
) {
  return await viteBuild({
    configFile,
    customLogger: createSilentViteLogger(),
    mode,
    build: {
      watch: null,
    },
  })
}

// buildAndAnalyze returns emitted files and the complete source dependency set.
export async function buildAndAnalyze(
  config: UserConfig,
  rootDir: string,
  webPkgRefs: Map<string, { root: string; subPaths: Set<string> }>,
) {
  const allInputFiles = new Set<string>()
  const buildOptions: InlineConfig = {
    ...config,
    configFile: false,
    plugins: [
      ...(config.plugins ?? []),
      {
        name: 'bldr-build-inputs',
        enforce: 'post',
        configResolved(resolvedConfig) {
          for (const file of resolvedConfig.configFileDependencies) {
            const normalized = normalizeModuleId(file, rootDir)
            if (normalized) {
              allInputFiles.add(normalized)
            }
          }
        },
      },
    ],
    build: {
      ...config.build,
      watch: null,
    },
    customLogger: config.customLogger ?? createSilentViteLogger(),
  }

  const viteOutput = (await viteBuild(buildOptions)) as
    | Rollup.RollupOutput[]
    | Rollup.RollupOutput
  const rollupOutputs: Rollup.RollupOutput[] = Array.isArray(viteOutput)
    ? viteOutput
    : [viteOutput]

  const outputChunks = rollupOutputs.flatMap((output) => output.output)

  const outDir = config.build?.outDir
  if (!outDir) {
    throw new Error('outDir is required')
  }

  const analysis = await analyzeManifest(outDir, outputChunks, rootDir)

  const entrypointOutputs = analysis.entrypointOutputs.map((entry) => ({
    entrypoint: entry.entrypoint
      ? path.isAbsolute(entry.entrypoint)
        ? path.relative(rootDir, entry.entrypoint)
        : entry.entrypoint
      : '',
    inputFiles: (entry.inputs || []).map((file) =>
      path.isAbsolute(file) ? path.relative(rootDir, file) : file,
    ),

    jsOutput: entry.outputs.js,
    cssOutputs: entry.outputs.css,
  }))

  entrypointOutputs.forEach((entry) => {
    entry.inputFiles?.forEach((file) => allInputFiles.add(file))
  })

  const allGlobalCssFiles = new Set<string>()
  analysis.globalCssFiles?.forEach((file) => allGlobalCssFiles.add(file))
  const outputFiles = new Set<string>()
  for (const output of outputChunks) {
    if (
      output.fileName === '.vite/manifest.json' ||
      output.fileName === 'manifest.json'
    ) {
      continue
    }
    outputFiles.add(output.fileName)
    if (output.type === 'chunk' && output.sourcemapFileName) {
      outputFiles.add(output.sourcemapFileName)
    }
  }

  const result = {
    success: true,
    entrypointOutputs,
    inputFiles: Array.from(allInputFiles),
    globalCssFiles: Array.from(allGlobalCssFiles),
    outputFiles: Array.from(outputFiles),
    webPkgRefs: Array.from(webPkgRefs.entries()).map(([pkgId, entry]) => ({
      pkgId,
      pkgRoot: entry.root,
      subPaths: Array.from(entry.subPaths),
    })),
  }

  // Source text and module tables are unnecessary once input provenance is captured.
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk') {
      const mutableChunk: Partial<Rollup.OutputChunk> = chunk
      delete mutableChunk.code
      delete mutableChunk.map
      delete mutableChunk.modules
      delete mutableChunk.moduleIds
    }
    if (chunk.type === 'asset') {
      const mutableChunk: Partial<Rollup.OutputAsset> = chunk
      delete mutableChunk.source
    }
  }

  return {
    viteOutput,
    outputChunks,
    analysis,
    result,
  }
}
