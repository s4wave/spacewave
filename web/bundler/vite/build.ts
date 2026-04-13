import { loadConfigFromFile, mergeConfig, build as viteBuild } from 'vite'
import type { InlineConfig, Rollup, UserConfig } from 'vite'
import { existsSync } from 'node:fs'
import { promises as fs } from 'node:fs'
import path from 'path'
import type { ConfigEnv } from 'vitest/config'

/**
 * Checks if an unknown error is a RollupError by checking for the watchFiles property
 */
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

// Load and merge configuration from a specified path if it exists
async function loadOptionalConfig(
  configEnv: ConfigEnv,
  configPath: string,
): Promise<UserConfig | null> {
  if (!existsSync(configPath)) {
    console.warn('[vite] ignoring not-existing config file: ' + configPath)
    return null
  }

  const loadedConfig = await loadConfigFromFile(configEnv, configPath)
  return loadedConfig?.config || null
}

// Builds a merged vite config from base config and optional additional configs
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
  viteMetadata?: {
    importedCss?: Set<string>
  }
}

function collectReferencedFiles(
  entryKey: string,
  manifest: ViteManifest,
  seen = new Set<string>(),
  cssFiles = new Set<string>(),
  assetFiles = new Set<string>(),
): { cssFiles: Set<string>; assetFiles: Set<string> } {
  if (seen.has(entryKey)) return { cssFiles, assetFiles }
  seen.add(entryKey)

  const entry = manifest[entryKey]
  if (!entry) return { cssFiles, assetFiles }

  // Collect CSS files
  entry.css?.forEach((c) => cssFiles.add(c))

  // Collect asset files
  entry.assets?.forEach((a) => assetFiles.add(a))

  // Recursively collect from imports
  entry.imports?.forEach((imp) =>
    collectReferencedFiles(imp, manifest, seen, cssFiles, assetFiles),
  )

  return { cssFiles, assetFiles }
}

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
    const entrypoint =
      chunk.facadeModuleId ?
        normalizeModuleId(chunk.facadeModuleId, rootDir)
      : chunk.name
    if (!entrypoint) {
      continue
    }
    const css = [
      ...(chunk.viteMetadata?.importedCss ?? []),
      ...chunk.referencedFiles.filter((file) => file.endsWith('.css')),
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

/**
 * Normalize a module ID to a clean relative path.
 * - Strips query strings (e.g., ?commonjs-module)
 * - Strips null byte prefixes (Rollup virtual modules)
 * - Converts absolute paths to relative paths
 * - Returns null for virtual/special modules that shouldn't be watched
 */
function normalizeModuleId(id: string, rootDir: string): string | null {
  if (id.startsWith('\x00')) {
    return null
  }
  const withoutQuery = id.split('?')[0]
  if (!path.isAbsolute(withoutQuery)) {
    return path.normalize(withoutQuery)
  }
  return path.normalize(path.relative(rootDir, withoutQuery))
}

/**
 * Build a map of chunk fileName to its imported chunk fileNames.
 * This allows us to traverse the chunk dependency graph.
 */
function buildChunkImportsMap(
  outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[],
): Map<string, string[]> {
  const chunkImports = new Map<string, string[]>()
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk' && chunk.fileName) {
      chunkImports.set(chunk.fileName, chunk.imports || [])
    }
  }
  return chunkImports
}

/**
 * Collect all modules from a chunk and all its imported chunks (transitive).
 * This ensures we track dependencies in shared/split chunks.
 */
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

  // 1. Map each JS chunk to its direct source modules.
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

  // 2. Build chunk import graph for traversing shared chunks.
  const chunkImports = buildChunkImportsMap(outputChunks)

  // 3. Prepare the primary output structure for each entrypoint.
  // Collect modules from the entry chunk AND all its imported chunks (transitive).
  const entrypointOutputs = Object.entries(manifest)
    .filter(([, value]) => value.isEntry)
    .map(([key, value]) => {
      const allModules = collectAllModulesForChunk(
        value.file,
        jsChunkToModules,
        chunkImports,
      )
      const entrypointPath = value.src ?? key
      return {
        entrypoint:
          path.isAbsolute(entrypointPath) ?
            path.relative(rootDir, entrypointPath)
          : entrypointPath,
        outputs: {
          js: value.file,
          css: new Set<string>(value.css ?? []),
        },
        inputs: allModules,
      }
    })

  // 4. Associate CSS assets with entrypoints using manifest data.
  const allCssAssets = new Set<string>()
  const handledCssAssets = new Set<string>()

  // Collect all CSS assets
  for (const chunk of outputChunks) {
    if (chunk.type === 'asset' && chunk.fileName.endsWith('.css')) {
      allCssAssets.add(chunk.fileName)
    }
  }

  // Associate CSS files with entrypoints using the manifest
  for (const [key, value] of Object.entries(manifest)) {
    if (!value.isEntry) continue

    const { cssFiles } = collectReferencedFiles(key, manifest)

    // Find the corresponding entrypoint output
    const entryOutput = entrypointOutputs.find((entry) => {
      const entrypointPath = value.src ?? key
      const normalizedEntrypoint =
        path.isAbsolute(entrypointPath) ?
          path.relative(rootDir, entrypointPath)
        : entrypointPath
      return entry.entrypoint === normalizedEntrypoint
    })

    if (entryOutput) {
      cssFiles.forEach((cssFile) => {
        entryOutput.outputs.css.add(cssFile)
        handledCssAssets.add(cssFile)
      })
    }
  }

  // 5. Finalize the output structure.
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

// Run a Vite build with the specified config file
export async function runBuild(
  configFile: string,
  mode: string = 'development',
) {
  try {
    return await viteBuild({
      configFile,
      mode,
      build: {
        watch: null,
      },
    })
  } catch (e) {
    console.error(e)
    throw e
  }
}

// Build and analyze the output
export async function buildAndAnalyze(
  config: UserConfig,
  rootDir: string,
  webPkgRefs: Map<string, { root: string; subPaths: Set<string> }>,
) {
  const buildOptions: InlineConfig = {
    build: {
      ...config.build,
      watch: null,
    },
    ...config,
  }

  const viteOutput = (await viteBuild(buildOptions)) as
    | Rollup.RollupOutput[]
    | Rollup.RollupOutput
  const rollupOutputs: Rollup.RollupOutput[] =
    Array.isArray(viteOutput) ? viteOutput : [viteOutput]

  // merge the output chunks into one array
  const outputChunks = rollupOutputs.flatMap((output) => output.output)

  // determine the output directory
  const outDir = config.build?.outDir
  if (!outDir) {
    throw new Error('outDir is required')
  }

  // Analyze the manifest to extract entrypoints and their corresponding files
  // This must happen BEFORE cleanup since we need access to moduleIds
  const analysis = await analyzeManifest(outDir, outputChunks, rootDir)

  // Map the analysis results to the response format and make paths relative to rootDir
  const entrypointOutputs = analysis.entrypointOutputs.map((entry) => ({
    entrypoint:
      entry.entrypoint ?
        path.isAbsolute(entry.entrypoint) ?
          path.relative(rootDir, entry.entrypoint)
        : entry.entrypoint
      : '',
    inputFiles: (entry.inputs || []).map((file) =>
      path.isAbsolute(file) ? path.relative(rootDir, file) : file,
    ),

    jsOutput: entry.outputs.js,
    cssOutputs: entry.outputs.css,
  }))

  // Collect all input files (as relative paths)
  const allInputFiles = new Set<string>()
  entrypointOutputs.forEach((entry) => {
    entry.inputFiles?.forEach((file) => allInputFiles.add(file))
  })

  const allGlobalCssFiles = new Set<string>()
  analysis.globalCssFiles?.forEach((file) => allGlobalCssFiles.add(file))

  const result = {
    success: true,
    entrypointOutputs,
    inputFiles: Array.from(allInputFiles),
    globalCssFiles: Array.from(allGlobalCssFiles),
    webPkgRefs: Array.from(webPkgRefs.entries()).map(([pkgId, entry]) => ({
      pkgId,
      pkgRoot: entry.root,
      subPaths: Array.from(entry.subPaths),
    })),
  }

  // drop some unnecessary detail from the result(s)
  for (const chunk of outputChunks) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const mutableChunk = chunk as Record<string, any> // otherwise typescript complains
    if (chunk.type === 'chunk') {
      // the source code, too much info
      delete mutableChunk.code
      // sourcemap
      delete mutableChunk.map
      // list of modules that were bundled (source files)
      delete mutableChunk.modules
      // list of keys in the modules map
      delete mutableChunk.moduleIds
      // could be useful to know which variables were imported from each module
      delete mutableChunk.importedBindings
    }
    if (chunk.type === 'asset') {
      delete mutableChunk.source
    }
  }

  // Return the results
  return {
    viteOutput,
    outputChunks,
    analysis,
    result,
  }
}
