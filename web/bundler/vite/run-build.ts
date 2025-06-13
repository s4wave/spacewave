import { resolve } from 'path'
import { fileURLToPath } from 'url'
import { dirname } from 'path'
import { writeFileSync } from 'fs'
import { buildAndAnalyze, buildConfig } from './build.js'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Debug the build process:
// tsx run-build.ts

async function main() {
  try {
    const outputDir = resolve(__dirname, './vite-dist')
    const configFile = resolve(__dirname, '../../../vite.config.ts')
    const baseConfig = resolve(__dirname, './vite-base.config.ts')

    const config = await buildConfig(
      { mode: 'development', command: 'build' },
      configFile,
      baseConfig,
    )
    if (!config.build) {
      config.build = {}
    }
    config.build.outDir = outputDir

    // Run the build
    const result = await buildAndAnalyze(config)

    // Write the result to vite-result.json
    writeFileSync(
      resolve(outputDir, 'vite-result.json'),
      JSON.stringify(result, null, 2),
    )

    console.log('Build completed successfully')
    console.log('Result written to:', resolve(outputDir, 'vite-result.json'))
  } catch (e) {
    console.error('Build failed:', e)
    process.exit(1)
  }
}

main()
