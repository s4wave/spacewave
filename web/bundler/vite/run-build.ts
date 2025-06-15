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
    await buildAndAnalyze(config)

    console.log('Build completed successfully')
  } catch (e) {
    console.error('Build failed:', e)
    process.exit(1)
  }
}

main()
