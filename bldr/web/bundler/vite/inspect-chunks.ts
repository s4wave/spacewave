import { resolve } from 'path'
import { fileURLToPath } from 'url'
import { dirname } from 'path'
import { buildAndAnalyze, buildConfig } from './build.js'
import { promises as fs } from 'fs'

const __dirname = dirname(fileURLToPath(import.meta.url))

async function main() {
  try {
    const outputDir = resolve(__dirname, './vite-dist-inspect')
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

    const webPkgRefs = new Map<
      string,
      { root: string; subPaths: Set<string> }
    >()

    const { viteOutput } = await buildAndAnalyze(
      config,
      rootDir,
      webPkgRefs,
    )

    // Get the raw chunks BEFORE they're cleaned up
    const outputs = Array.isArray(viteOutput) ? viteOutput : [viteOutput]
    const rawChunks = outputs.flatMap((o) =>
      o.output.filter((c) => c.type === 'chunk'),
    )

    console.log('\n=== RAW CHUNK INSPECTION ===\n')

    for (const chunk of rawChunks) {
      if (chunk.type !== 'chunk') continue

      console.log(`\nChunk: ${chunk.fileName}`)
      console.log(`  facadeModuleId: ${chunk.facadeModuleId}`)
      console.log(`  moduleIds exists: ${'moduleIds' in chunk}`)
      console.log(`  modules exists: ${'modules' in chunk}`)

      if ('moduleIds' in chunk && chunk.moduleIds) {
        console.log(`  moduleIds length: ${chunk.moduleIds.length}`)
        if (chunk.moduleIds.length > 0) {
          console.log(`  First 3 moduleIds:`)
          chunk.moduleIds
            .slice(0, 3)
            .forEach((id) => console.log(`    - ${id}`))
        }
      }

      if ('modules' in chunk && chunk.modules) {
        const moduleKeys = Object.keys(chunk.modules)
        console.log(`  modules count: ${moduleKeys.length}`)
        if (moduleKeys.length > 0) {
          console.log(`  First 3 module keys:`)
          moduleKeys.slice(0, 3).forEach((id) => console.log(`    - ${id}`))

          // Check if CSS files are in modules
          const cssModules = moduleKeys.filter((k) => k.endsWith('.css'))
          if (cssModules.length > 0) {
            console.log(`  CSS modules found (${cssModules.length}):`)
            cssModules.forEach((id) => console.log(`    - ${id}`))
          }
        }
      }
    }

    // Write full chunk data to file for inspection
    await fs.mkdir(resolve(outputDir, '.vite'), { recursive: true })
    await fs.writeFile(
      resolve(outputDir, '.vite/raw-chunks.json'),
      JSON.stringify(rawChunks, null, 2),
    )

    console.log(`\n\nRaw chunks written to: ${outputDir}/.vite/raw-chunks.json`)
    console.log('Inspect this file to see the full chunk structure')
  } catch (e) {
    console.error('Build failed:', e)
    process.exit(1)
  }
}

main()
