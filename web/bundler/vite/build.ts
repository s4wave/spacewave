import { loadConfigFromFile, mergeConfig, build as viteBuild } from 'vite'
import type { InlineConfig, UserConfig } from 'vite'
import { existsSync } from 'node:fs'
import { promises as fs } from 'node:fs'
import path from 'path'
import type { ConfigEnv } from 'vitest/config.js'
import type { OutputChunk, RollupOutput, OutputAsset } from 'rollup'
import type { RollupError } from 'rollup'

/**
 * Checks if an unknown error is a RollupError by checking for the watchFiles property
 */
export function isRollupError(err: unknown): err is RollupError {
  return (
    typeof err === 'object' &&
    err !== null &&
    'watchFiles' in err &&
    Array.isArray((err as RollupError).watchFiles)
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

// Analyze the manifest to extract entrypoints and their corresponding files
export async function analyzeManifest(
  outDir: string,
  outputChunks: (OutputChunk | OutputAsset)[],
  rootDir: string,
) {
  const manifestPath = path.join(outDir, '.vite/manifest.json')
  const manifest: ViteManifest = JSON.parse(
    await fs.readFile(manifestPath, 'utf-8'),
  )

  // 1. Map each JS chunk to its constituent source modules.
  const jsChunkToModules = new Map<string, Set<string>>()
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk' && chunk.fileName) {
      const modules = new Set<string>()
      // The facadeModuleId is the entry-point file for this chunk.
      if (chunk.facadeModuleId) {
        modules.add(
          path.normalize(path.relative(rootDir, chunk.facadeModuleId)),
        )
      }
      // The moduleIds are all the other files bundled into this chunk.
      if (chunk.moduleIds) {
        chunk.moduleIds.forEach((id) =>
          modules.add(path.normalize(path.relative(rootDir, id))),
        )
      }
      jsChunkToModules.set(chunk.fileName, modules)
    }
  }

  // 2. Prepare the primary output structure for each entrypoint.
  const entrypointOutputs = Object.entries(manifest)
    .filter(([, value]) => value.isEntry)
    .map(([key, value]) => {
      const modules = jsChunkToModules.get(value.file) ?? new Set<string>()
      const entrypointPath = value.src ?? key
      return {
        entrypoint: path.isAbsolute(entrypointPath) 
          ? path.relative(rootDir, entrypointPath)
          : entrypointPath,
        outputs: {
          js: value.file,
          css: new Set<string>(value.css ?? []),
        },
        inputs: modules,
      }
    })

  // 3. Associate CSS assets with entrypoints.
  const allCssAssets = new Set<string>()
  const handledCssAssets = new Set<string>()

  for (const chunk of outputChunks) {
    if (chunk.type !== 'asset' || !chunk.fileName.endsWith('.css')) continue
    allCssAssets.add(chunk.fileName)

    const asset = chunk as OutputAsset & { originalFileNames?: string[] }
    const referencers = (asset.originalFileNames ?? []).map((f) =>
      path.normalize(f),
    )

    for (const entry of entrypointOutputs) {
      if (referencers.some((ref) => entry.inputs.has(ref))) {
        entry.outputs.css.add(chunk.fileName)
        handledCssAssets.add(chunk.fileName)
      }
    }
  }

  // 4. Finalize the output structure.
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
    | RollupOutput[]
    | RollupOutput
  const rollupOutputs: RollupOutput[] =
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
  const analysis = await analyzeManifest(
    outDir,
    outputChunks,
    rootDir,
  )

  // Map the analysis results to the response format and make paths relative to rootDir
  const entrypointOutputs = analysis.entrypointOutputs.map((entry) => ({
    entrypoint: entry.entrypoint ? (
      path.isAbsolute(entry.entrypoint) 
        ? path.relative(rootDir, entry.entrypoint)
        : entry.entrypoint
    ) : '',
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
