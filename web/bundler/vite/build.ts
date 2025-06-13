import { loadConfigFromFile, mergeConfig, build as viteBuild } from 'vite'
import type { InlineConfig, UserConfig } from 'vite'
import { existsSync } from 'node:fs'
import { promises as fs } from 'node:fs'
import path from 'path'
import type { ConfigEnv } from 'vitest/config.js'
import type { RollupOutput } from 'rollup'
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
  outputChunks: any[],
  rootDir: string,
) {
  const manifestPath = path.join(outDir, '.vite/manifest.json')
  const manifest: ViteManifest = JSON.parse(
    await fs.readFile(manifestPath, 'utf-8'),
  )

  // Build a map of JS output files to their input files from rollup chunks
  const jsFileToInputs = new Map<string, string[]>()
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk' && chunk.fileName) {
      // Track input files for this chunk
      const inputFiles = new Set<string>()

      // Add the entry module itself
      if (chunk.facadeModuleId) {
        const relativePath = path.relative(rootDir, chunk.facadeModuleId)
        inputFiles.add(relativePath)
      }

      // Add referenced files for this chunk
      if (chunk.moduleIds) {
        chunk.moduleIds.forEach((file: string) => {
          if (file) {
            const relativePath = path.relative(rootDir, file)
            inputFiles.add(relativePath)
          }
        })
      }

      jsFileToInputs.set(chunk.fileName, Array.from(inputFiles))
    }
  }

  // Build a lookup from output file -> source file for ALL types of files
  const outputToSrc = new Map<string, string>()
  Object.values(manifest).forEach((entry) => {
    if (entry.src && entry.file) {
      outputToSrc.set(entry.file, entry.src)
    }
  })

  // Build CSS source file mapping by finding CSS files in JS chunk moduleIds
  // CSS asset chunks don't have proper source paths, but JS chunks that import them do
  const cssOutputToSrc = new Map<string, string>()
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk' && chunk.moduleIds) {
      chunk.moduleIds.forEach((moduleId: string) => {
        if (moduleId && moduleId.endsWith('.css')) {
          const relativePath = path.relative(rootDir, moduleId)
          // Find the CSS output file that corresponds to this source file
          // We need to match by the base filename since we don't have a direct mapping
          const baseFileName = path.basename(relativePath, '.css')
          for (const assetChunk of outputChunks) {
            if (assetChunk.type === 'asset' && 
                assetChunk.fileName?.endsWith('.css') &&
                assetChunk.names?.includes(baseFileName + '.css')) {
              cssOutputToSrc.set(assetChunk.fileName, relativePath)
              break
            }
          }
        }
      })
    }
  }

  const entrypointOutputs = Object.entries(manifest)
    .filter(([, v]) => v.isEntry)
    .map(([key, v]) => {
      const inputSet = new Set<string>(jsFileToInputs.get(v.file) || [])

      // Collect all referenced files (CSS and assets)
      const { cssFiles, assetFiles } = collectReferencedFiles(key, manifest)

      // Add source files for CSS using rollup chunk mapping
      cssFiles.forEach((cssFile) => {
        const src = cssOutputToSrc.get(cssFile)
        if (src) inputSet.add(src)
      })

      // Add source files for other assets using manifest mapping
      assetFiles.forEach((assetFile) => {
        const src = outputToSrc.get(assetFile)
        if (src) inputSet.add(src)
      })

      return {
        entrypoint: v.src ?? key,
        outputs: {
          js: v.file,
          css: Array.from(cssFiles),
        },
        inputs: Array.from(inputSet),
      }
    })

  // Global CSS files are any CSS assets not tied to specific entries
  const allCssFromEntries = new Set<string>()
  entrypointOutputs.forEach((entry) => {
    entry.outputs.css.forEach((css) => allCssFromEntries.add(css))
  })

  const globalCssFiles = Object.values(manifest)
    .filter((entry) => entry.file.endsWith('.css'))
    .map((entry) => entry.file)
    .filter((css) => !allCssFromEntries.has(css))

  return {
    entrypointOutputs,
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
export async function buildAndAnalyze(config: UserConfig) {
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
    config.root ?? process.cwd(),
  )

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
  }
}
