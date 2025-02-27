import { loadConfigFromFile, mergeConfig, build as viteBuild } from 'vite'
import type { InlineConfig, UserConfig } from 'vite'
import { existsSync } from 'node:fs'
import type { ConfigEnv } from 'vitest/config.js'
import type { RollupOutput, OutputAsset, OutputChunk } from 'rollup'
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

type DeepPartial<T> =
  T extends object ?
    {
      [P in keyof T]?: DeepPartial<T[P]>
    }
  : T

type BuildOutput = [...(DeepPartial<OutputChunk> | DeepPartial<OutputAsset>)[]]

// Analyze the build output to extract entrypoints and their corresponding files
export function analyzeOutput(rollupOutput: BuildOutput) {
  // Get the entrypoints and their corresponding output files
  const entrypointOutputs = rollupOutput
    .filter(
      (output): output is DeepPartial<OutputChunk> =>
        output.type === 'chunk' &&
        (output.isEntry === true || output.isDynamicEntry === true),
    )
    .map((chunk) => {
      const facadeModuleId = chunk.facadeModuleId
      const jsFile = chunk.fileName

      // Find corresponding CSS files for this entry
      // Since viteMetadata.importedCss might contain the CSS files
      const cssFiles =
        (chunk.viteMetadata?.importedCss as Set<string>) ?? new Set<string>()

      // Track input files for this entrypoint
      const inputFiles = new Set<string>()

      // Add the entry module itself
      if (facadeModuleId) {
        inputFiles.add(facadeModuleId)
      }

      // Add referenced files for this chunk
      ;(chunk.referencedFiles || []).forEach((file) => {
        if (file) inputFiles.add(file)
      })

      return {
        entrypoint: facadeModuleId,
        outputs: {
          js: jsFile,
          css: Array.from(cssFiles),
        },
        inputs: Array.from(inputFiles),
      }
    })

  // Find the global CSS file if it exists (not directly tied to an entrypoint)
  const globalCssFiles = rollupOutput
    .filter(
      (output): output is DeepPartial<OutputAsset> =>
        !!(output.type === 'asset' && output.fileName?.endsWith('.css')),
    )
    .map((asset) => asset.fileName as string)

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
        watch: null, // {}
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
      watch: null, // {}
    },
    ...config,
  }

  const viteOutput = (await viteBuild(buildOptions)) as
    | RollupOutput[]
    | RollupOutput
  const rollupOutputs: RollupOutput[] =
    Array.isArray(viteOutput) ? viteOutput : [viteOutput]

  // merge the output chunks into one array
  const outputChunks: BuildOutput = rollupOutputs.flatMap(
    (output) => output.output,
  )

  // drop some unnecessary detail from the result(s)
  for (const chunk of outputChunks) {
    if (chunk.type === 'chunk') {
      delete chunk['code']
      delete chunk['modules']
      delete chunk['map']
      delete chunk['moduleIds']
      delete chunk['importedBindings']
    }
    if (chunk.type === 'asset') {
      delete chunk['source']
    }
  }

  return {
    outputChunks,
    analysis: analyzeOutput(outputChunks),
  }
}
