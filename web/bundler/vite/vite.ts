import path, { resolve } from 'path'
import fs from 'fs'
import { Server, StreamConn, createHandler, createMux } from 'starpc'
import {
  buildPipeName,
  connectToPipe,
} from '../../../util/pipesock/pipesock.js'
import { ViteBundler, ViteBundlerDefinition } from './vite_srpc.pb.js'
import { BuildRequest, BuildResponse } from './vite.pb.js'
import { buildAndAnalyze, buildConfig, isRollupError } from './build.js'
import { createWebPkgRemapPlugin } from './plugin.js'
import { UserConfig } from 'vite'

// verboseDebug is the verbose debugging flag
const verboseDebug = true

// Parse command line arguments
function parseArgs() {
  const args = process.argv.slice(2)
  const result: { [key: string]: string } = {}

  for (let i = 0; i < args.length; i++) {
    const arg = args[i]
    if (arg.startsWith('--')) {
      const key = arg.substring(2)
      const value =
        args[i + 1] && !args[i + 1].startsWith('--') ? args[i + 1] : 'true'
      result[key] = value
      if (value !== 'true') {
        i++ // Skip the next item as it's the value
      }
    }
  }

  return result
}

// Implementation of the ViteBundler service
class ViteBundlerService implements ViteBundler {
  async Build(request: BuildRequest): Promise<BuildResponse> {
    return await buildBundle(request)
  }
}

/**
 * Core build function that processes Vite bundling requests
 * @param {BuildRequest} request - The build configuration request
 * @returns {Promise<BuildResponse>} The build results
 */
async function buildBundle(request: BuildRequest): Promise<BuildResponse> {
  if (verboseDebug) {
    console.log(`[vite] build request: ${JSON.stringify(request)}`)
  }

  let mergedConfig: UserConfig = {}
  try {
    const configPaths = request.configPaths || []
    const mode = request.mode || 'development'
    const rootDir = request.rootDir || process.cwd()
    const outDir = request.outDir || resolve(rootDir, 'dist')
    const distDir = request.distDir || resolve(rootDir, '.bldr/src')
    const publicPath = request.publicPath || null

    // Store web package references
    const webPkgRefs: Map<string, { root: string; subPaths: Set<string> }> =
      new Map()

    // set env vars to indicate the project root path
    // these are used in vite-base.config.ts
    process.env['BLDR_DIST_ROOT'] = distDir
    process.env['BLDR_PROJECT_ROOT'] = rootDir
    process.env['BLDR_OUT_ROOT'] = outDir

    // set node env
    process.env['NODE_ENV'] = mode

    // configPaths are relative to rootDir, make them absolute paths.
    const absoluteConfigPaths = configPaths.map((configPath) =>
      path.resolve(rootDir, configPath),
    )

    // Build the merged configuration
    mergedConfig = await buildConfig(
      { mode, command: 'build' },
      ...absoluteConfigPaths,
    )
    if (!mergedConfig.build) {
      mergedConfig.build = {}
    }
    mergedConfig.build.outDir = outDir

    // Set the root dir
    mergedConfig.root = rootDir

    // Ensure CSS is split into separate files per entry
    mergedConfig.build.cssCodeSplit = true
    // Write manifest.json to the output
    mergedConfig.build.manifest = true

    // Set the base path (public path for assets)
    if (publicPath != null) {
      mergedConfig.base = publicPath
    }

    // Set the cache dir
    if (request.cacheDir) {
      mergedConfig.cacheDir = request.cacheDir
    }

    // Add bldr external (importmap) packages.
    if (!mergedConfig.build.rollupOptions) {
      mergedConfig.build.rollupOptions = {}
    }
    mergedConfig.build.rollupOptions.external = request.externalPkgs ?? []

    // Add external packages for web pkg remapping
    const webPkgIDs: string[] = (request.webPkgs ?? [])
      .map((pkg) => pkg.id)
      .filter((pkg): pkg is string => !!pkg)

    // Add our web pkg remap plugin with the callback
    if (!mergedConfig.plugins) {
      mergedConfig.plugins = []
    }
    mergedConfig.plugins.push(
      createWebPkgRemapPlugin({
        webPkgIDs,
        addWebPkgImport: (webPkgID, webPkgRoot, webPkgSubPath) => {
          // Track the web package import similar to esbuild implementation
          const entry = webPkgRefs.get(webPkgID) || {
            root: webPkgRoot,
            subPaths: new Set<string>(),
          }
          entry.subPaths.add(webPkgSubPath)
          webPkgRefs.set(webPkgID, entry)
        },
        debug: verboseDebug,
      }),
    )

    // Add entrypoints if provided
    if (request.entrypoints && request.entrypoints.length > 0) {
      // Build a Rollup input map (name -> absolute path)
      const input: Record<string, string> = {} // InputOption
      for (const entrypoint of request.entrypoints) {
        if (entrypoint.inputPath) {
          const name =
            entrypoint.name ||
            path.basename(
              entrypoint.inputPath,
              path.extname(entrypoint.inputPath),
            )
          input[name] = resolve(rootDir, entrypoint.inputPath)
        }
      }

      // Ensure rollupOptions exists
      if (!mergedConfig.build.rollupOptions) {
        mergedConfig.build.rollupOptions = {}
      }

      // Merge the input map and guarantee we output ES-modules
      mergedConfig.build.rollupOptions = {
        ...mergedConfig.build.rollupOptions,
        input,
        preserveEntrySignatures: 'strict',
        output: {
          ...(mergedConfig.build.rollupOptions.output ?? {}),
          format: 'es',
          entryFileNames: (chunkInfo) => {
            // Preserve source directory structure for entry files
            const facadeModuleId = chunkInfo.facadeModuleId
            if (facadeModuleId) {
              const relativePath = path.relative(rootDir, facadeModuleId)
              const parsed = path.parse(relativePath)
              return `${parsed.dir}/${parsed.name}-[hash].mjs`
            }
            return '[name]-[hash].mjs'
          },
          chunkFileNames: '[name]-[hash].mjs',
          assetFileNames: '[name]-[hash][extname]',
        },
      }

      // Disable library mode entirely so assets are not inlined
      delete mergedConfig.build.lib
    }

    // Run the build process with the merged config
    const { analysis, viteOutput, result } = await buildAndAnalyze(mergedConfig, rootDir, webPkgRefs)

    if (verboseDebug) {
      // Ensure .vite directory exists
      const viteDir = path.join(outDir, '.vite')
      if (!fs.existsSync(viteDir)) {
        fs.mkdirSync(viteDir, { recursive: true })
      }

      // Write all JSON files to .vite/ subdirectory
      fs.writeFileSync(
        path.join(viteDir, 'vite-config.json'),
        JSON.stringify(mergedConfig, null, 2),
      )
      fs.writeFileSync(
        path.join(viteDir, 'vite-output.json'),
        JSON.stringify(viteOutput, null, 2),
      )
      fs.writeFileSync(
        path.join(viteDir, 'vite-analysis.json'),
        JSON.stringify(analysis, null, 2),
      )
      fs.writeFileSync(
        path.join(viteDir, 'vite-result.json'),
        JSON.stringify(result, null, 2),
      )
    }

    return result
  } catch (err) {
    console.error(`[vite] build error:`, err)
    const failureResp = {
      success: false,
      error: err instanceof Error ? err.message : String(err),
      inputFiles: isRollupError(err) ? err.watchFiles : [],
      webPkgRefs: [],
    }

    if (verboseDebug) {
      const errorOutDir = request.outDir || process.cwd()
      const viteDir = path.join(errorOutDir, '.vite')
      if (!fs.existsSync(viteDir)) {
        fs.mkdirSync(viteDir, { recursive: true })
      }

      fs.writeFileSync(
        path.join(viteDir, 'vite-config.json'),
        JSON.stringify(mergedConfig, null, 2),
      )
      fs.writeFileSync(
        path.join(viteDir, 'vite-error.json'),
        JSON.stringify(failureResp, null, 2),
      )
    }

    return failureResp
  }
}

async function main() {
  const args = parseArgs()
  const bundleId = args['bundle-id'] || ''
  const pipeUuid = args['pipe-uuid'] || ''

  // Validate required parameters
  if (!bundleId) {
    console.error('[vite] Error: Missing required parameter --bundle-id')
    process.exit(1)
  }

  if (!pipeUuid) {
    console.error('[vite] Error: Missing required parameter --pipe-uuid')
    process.exit(1)
  }

  const workdir = process.cwd()

  console.log(
    `[vite] bundler starting with bundle-id: ${bundleId}, pipe-uuid: ${pipeUuid}, workdir: ${workdir}`,
  )

  // Create SRPC server with the ViteBundler service
  const service = new ViteBundlerService()
  const srpcMux = createMux()
  srpcMux.register(createHandler(ViteBundlerDefinition, service))
  const srpcServer = new Server(srpcMux.lookupMethod)

  // Connect to the pipe created by the Go process
  // Use the pipe UUID passed from the Go process
  const ipcPath = buildPipeName(workdir, pipeUuid)

  console.log(`[vite] connecting to pipe: ${ipcPath}`)

  // Create stream connection
  const streamConn = new StreamConn(srpcServer, { direction: 'inbound' })

  // Connect to the pipe and set up bidirectional communication
  const socket = connectToPipe(ipcPath, streamConn)

  // Handle connection errors
  socket.on('error', (err) => {
    console.error(`[vite] connection error: ${err}`)
    process.exit(1)
  })
}

main().catch((err) => {
  console.error(`[vite] fatal error: ${err}`)
  process.exit(1)
})
