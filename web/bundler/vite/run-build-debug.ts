import { resolve } from 'path'
import { fileURLToPath } from 'url'
import { dirname } from 'path'
import { buildAndAnalyze, buildConfig } from './build.js'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Debug the build process to check moduleIds:
// npx tsx web/bundler/vite/run-build-debug.ts

async function main() {
  try {
    const outputDir = resolve(__dirname, './vite-dist-debug')
    const configFile = resolve(__dirname, '../../../vite.config.ts')
    const baseConfig = resolve(__dirname, './vite-base.config.ts')
    const rootDir = resolve(__dirname, '../../..')

    const config = await buildConfig(
      { mode: 'development', command: 'build' },
      configFile,
      baseConfig,
    )
    if (!config.build) {
      config.build = {}
    }
    config.build.outDir = outputDir

    // Create empty web package references map for this build
    const webPkgRefs = new Map<
      string,
      { root: string; subPaths: Set<string> }
    >()

    // Run the build
    const { viteOutput, result } = await buildAndAnalyze(
      config,
      rootDir,
      webPkgRefs,
    )

    console.log('\n=== DEBUG OUTPUT ===\n')

    // Check first chunk for moduleIds and modules
    const outputs = Array.isArray(viteOutput) ? viteOutput : [viteOutput]
    const firstChunk = outputs[0]?.output.find((o) => o.type === 'chunk')

    if (firstChunk && firstChunk.type === 'chunk') {
      console.log('First chunk fileName:', firstChunk.fileName)
      console.log('First chunk facadeModuleId:', firstChunk.facadeModuleId)
      console.log('Has moduleIds?', 'moduleIds' in firstChunk)
      console.log(
        'moduleIds length:',
        Array.isArray(firstChunk.moduleIds) ? firstChunk.moduleIds.length : 0,
      )
      console.log('Has modules?', 'modules' in firstChunk)
      console.log(
        'modules count:',
        firstChunk.modules ? Object.keys(firstChunk.modules).length : 0,
      )

      if (firstChunk.moduleIds && firstChunk.moduleIds.length > 0) {
        console.log('\nSample moduleIds (first 5):')
        firstChunk.moduleIds.slice(0, 5).forEach((id) => {
          console.log('  -', id)
        })
      }

      if (firstChunk.modules) {
        console.log('\nSample module keys (first 5):')
        Object.keys(firstChunk.modules)
          .slice(0, 5)
          .forEach((id) => {
            console.log('  -', id)
          })
      }
    }

    console.log('\n=== ANALYSIS RESULT ===\n')
    console.log('Total input files:', result.inputFiles.length)
    console.log('Entrypoint outputs:', result.entrypointOutputs.length)
    result.entrypointOutputs.forEach((ep, idx) => {
      console.log(`\nEntrypoint ${idx + 1}:`)
      console.log('  Path:', ep.entrypoint)
      console.log('  Input files count:', ep.inputFiles.length)
      console.log('  Input files:', ep.inputFiles.join(', '))
    })

    console.log('\nBuild completed successfully')
  } catch (e) {
    console.error('Build failed:', e)
    process.exit(1)
  }
}

main()
