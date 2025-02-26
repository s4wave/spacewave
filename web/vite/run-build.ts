import { build as viteBuild } from 'vite'
import type { RollupOutput, OutputAsset, OutputChunk } from 'rollup'
import { resolve } from 'path'
import { fileURLToPath } from 'url'
import { dirname } from 'path'
import { writeFileSync } from 'fs'

const __dirname = dirname(fileURLToPath(import.meta.url))

async function build() {
  try {
    return await viteBuild({
      configFile: resolve(__dirname, '../../vite.config.ts'),
      mode: 'development',
      build: {
        watch: null, // {}
      },
    })
  } catch (e) {
    console.error(e)
    process.exit(1)
  }
}

type DeepPartial<T> =
  T extends object ?
    {
      [P in keyof T]?: DeepPartial<T[P]>
    }
  : T

type BuildOutput = [...(DeepPartial<OutputChunk> | DeepPartial<OutputAsset>)[]]

function analyzeOutput(rollupOutput: BuildOutput) {
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
        (chunk.viteMetadata?.importedCss as string[] | undefined) || []

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
          css: cssFiles,
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

async function buildAndWrite() {
  const viteOutput = (await build()) as RollupOutput[] | RollupOutput
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

      /*
      const map = chunk.map
      if (map) {
        delete map["sourcesContent"]
        delete map["mappings"]
        delete (map as any)["x_google_ignoreList"]
      }
      */
    }
    if (chunk.type === 'asset') {
      delete chunk['source']
    }
  }

  writeFileSync(
    resolve(__dirname, '../../vite-output.json'),
    JSON.stringify(outputChunks, null, 2),
  )

  writeFileSync(
    resolve(__dirname, '../../vite-analysis.json'),
    JSON.stringify(analyzeOutput(outputChunks), null, 2),
  )
}

buildAndWrite()
