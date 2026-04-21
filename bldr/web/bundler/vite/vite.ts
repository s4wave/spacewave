import path, { resolve } from 'path'
import fs from 'fs'
import net from 'net'
import { Server, StreamConn, createHandler, createMux } from 'starpc'
import {
  buildPipeName,
  createSocketConnection,
  startSocketSender,
} from '@go/github.com/aperturerobotics/util/pipesock/pipesock.js'
import { ViteBundler, ViteBundlerDefinition } from './vite_srpc.pb.js'
import {
  BuildRequest,
  BuildResponse,
  BuildWebPkgRequest,
  BuildWebPkgResponse,
} from './vite.pb.js'
import {
  buildAndAnalyze,
  buildConfig,
  isBundleError,
  isRollupError,
} from './build.js'
import { createWebPkgRemapPlugin } from './plugin.js'
import { build as viteBuild, esmExternalRequirePlugin, type UserConfig, type InlineConfig, type Rollup } from 'vite'

// verboseDebug is the verbose debugging flag
const verboseDebug = process.env.BLDR_VITE_VERBOSE === 'true'

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

  async BuildWebPkg(
    request: BuildWebPkgRequest,
  ): Promise<BuildWebPkgResponse> {
    return await buildWebPkg(request)
  }
}

function resolveTrackedSourceFile(
  pkgRoot: string,
  moduleId: string,
): string | null {
  if (moduleId.startsWith('\x00')) {
    return null
  }
  if (moduleId.startsWith('__vite-browser-external')) {
    return null
  }

  const clean = moduleId.split('?')[0]
  const filePath = path.isAbsolute(clean)
    ? clean
    : path.resolve(pkgRoot, clean)

  try {
    const stat = fs.statSync(filePath)
    if (!stat.isFile()) {
      return null
    }
    return filePath
  } catch {
    return null
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

    // disable colors
    process.env['NO_COLOR'] = '1'
    process.env['NODE_DISABLE_COLORS'] = '1'
    process.env['CI'] = '1'

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

    if (!mergedConfig.build.rolldownOptions) {
      mergedConfig.build.rolldownOptions = {}
    }

    // Externalize BldrExternal packages (react, etc.) and webPkg packages
    // (@s4wave/web, etc.) via rolldownOptions.external.
    //
    // BldrExternal: served via import map, specifiers kept as-is. WebPkgs:
    // served at /b/pkg/ URLs, specifiers rewritten by the bldr-pkg-resolve
    // plugin's renderChunk hook.
    //
    // Both must use rolldownOptions.external because Rolldown's built-in
    // vite-resolve plugin resolves tsconfig paths before user plugin resolveId
    // hooks fire.
    const externalPkgs = request.externalPkgs ?? []
    const webPkgIDs: string[] = (request.webPkgs ?? [])
      .map((pkg) => pkg.id)
      .filter((pkg): pkg is string => !!pkg)

    // Asset extensions that must NOT be externalized, they need Vite's
    // CSS/asset pipeline (Tailwind, PostCSS, etc.).
    const assetExts = ['.css', '.scss', '.sass', '.less', '.styl', '.png', '.jpg', '.jpeg', '.gif', '.svg', '.webp', '.ico', '.woff', '.woff2', '.ttf', '.eot']

    // Only BldrExternal packages go in rolldownOptions.external.
    // Web pkg IDs are NOT externalized here -- the bldr-pkg-resolve
    // plugin's resolveId handles them, resolving to the actual file
    // path and rewriting to /b/pkg/{id}/{file}.mjs URLs.
    // If web pkg IDs are in rolldownOptions.external, Rolldown's
    // built-in resolver catches them first and the plugin never fires.
    if (externalPkgs.length > 0) {
      mergedConfig.build.rolldownOptions.external = (id: string) => {
        if (assetExts.some((ext) => id.endsWith(ext))) {
          return false
        }
        return externalPkgs.some(
          (pkg) => id === pkg || id.startsWith(pkg + '/'),
        )
      }
    }

    // Rewrite web pkg import specifiers to /b/pkg/ URLs in the output.
    // The plugin also reports discovered package roots back to the Go side.
    // Entry point discovery is handled by the Go side from config, not here.
    if (!mergedConfig.plugins) {
      mergedConfig.plugins = []
    }
    mergedConfig.plugins.push(
      createWebPkgRemapPlugin({
        webPkgIDs,
        addWebPkgRoot: (webPkgID, webPkgRoot) => {
          if (!webPkgRefs.has(webPkgID)) {
            webPkgRefs.set(webPkgID, {
              root: webPkgRoot,
              subPaths: new Set<string>(),
            })
          }
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

      // Ensure rolldownOptions exists
      if (!mergedConfig.build.rolldownOptions) {
        mergedConfig.build.rolldownOptions = {}
      }

      // Merge the input map and guarantee we output ES-modules.
      mergedConfig.build.rolldownOptions = {
        ...mergedConfig.build.rolldownOptions,
        input,
        preserveEntrySignatures: 'strict',
        output: {
          ...(mergedConfig.build.rolldownOptions.output ?? {}),
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
    const { analysis, viteOutput, result } = await buildAndAnalyze(
      mergedConfig,
      rootDir,
      webPkgRefs,
    )

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

    let errorMessage: string
    let inputFiles: string[] = []
    if (isBundleError(err)) {
      // Vite 8 BundleError: extract structured errors from the errors array
      const messages = err.errors.map((e) => {
        const loc = e.loc
          ? ` (${e.loc.file}:${e.loc.line}:${e.loc.column})`
          : ''
        return `${e.message}${loc}`
      })
      errorMessage = messages.join('\n')
    } else if (isRollupError(err)) {
      errorMessage = err.message
      inputFiles = err.watchFiles ?? []
    } else {
      errorMessage = err instanceof Error ? err.message : String(err)
    }

    const failureResp = {
      success: false,
      error: errorMessage,
      inputFiles,
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

// buildWebPkg builds a single web package using Vite.
async function buildWebPkg(
  request: BuildWebPkgRequest,
): Promise<BuildWebPkgResponse> {
  const pkgId = request.pkgId || ''
  const pkgRoot = request.pkgRoot || ''
  const imports = request.imports || []
  const siblingPkgIds = request.siblingPkgIds || []
  const externalPkgs = request.externalPkgs || []
  const outDir = request.outDir || ''
  const isRelease = request.isRelease || false
  const cacheDir = request.cacheDir || ''

  if (!pkgId || !pkgRoot || imports.length === 0 || !outDir) {
    return {
      success: false,
      error: 'pkgId, pkgRoot, imports, and outDir are required',
    }
  }

  if (verboseDebug) {
    console.log(`[vite] buildWebPkg: ${pkgId} imports=${imports}`)
  }

  try {
    // Build the Rolldown input map from imports.
    // Each import entry becomes a named input matching the package export
    // specifier (e.g. "index" from "index.js", "google/protobuf/empty"
    // from "google/protobuf/empty.pb.js"). Strip all trailing known
    // extensions so the input name matches what consumers import.
    //
    // Imports that end with .mjs are CJS ESM wrappers generated by the
    // Go side (determine-cjs-exports). They use absolute paths internally.
    const knownExts = new Set(['.js', '.cjs', '.mjs', '.ts', '.tsx', '.jsx', '.pb', '.css'])
    const input: Record<string, string> = {}

    // Detect the CJS wrapper directory from absolute imports.
    // All absolute imports share a common wrapper dir parent.
    let wrapperDir = ''
    for (const imp of imports) {
      if (path.isAbsolute(imp)) {
        // The wrapper dir is the .cjs-wrappers directory.
        // Walk up from the import path to find it.
        let dir = path.dirname(imp)
        while (dir && dir !== path.dirname(dir)) {
          if (path.basename(dir) === '.cjs-wrappers') {
            wrapperDir = dir
            break
          }
          dir = path.dirname(dir)
        }
        if (wrapperDir) break
      }
    }

    for (const imp of imports) {
      // For absolute paths (CJS wrappers from Go side), derive the name
      // relative to the wrapper directory to preserve path structure.
      let nameBase: string
      if (path.isAbsolute(imp) && wrapperDir) {
        nameBase = path.relative(wrapperDir, imp)
      } else {
        nameBase = imp
      }
      let name = nameBase
      // Strip leading "./" prefix (from package.json export paths).
      if (name.startsWith('./')) {
        name = name.substring(2)
      }
      while (true) {
        const ext = path.extname(name)
        if (!ext || !knownExts.has(ext)) break
        name = name.substring(0, name.length - ext.length)
      }
      input[name] = path.isAbsolute(imp) ? imp : path.resolve(pkgRoot, imp)
    }

    // All packages to externalize: BldrExternal + siblings (excluding self).
    const allExternal = [
      ...externalPkgs,
      ...siblingPkgIds.filter((id) => id !== pkgId),
    ]

    // Sibling IDs that are NOT BldrExternal need /b/pkg/ URL remapping.
    // BldrExternal packages use the import map (bare specifiers preserved).
    const externalSet = new Set(externalPkgs)
    const remapSiblingIds = siblingPkgIds.filter(
      (id) => id !== pkgId && !externalSet.has(id),
    )

    const config: InlineConfig = {
      root: pkgRoot,
      cacheDir: cacheDir || undefined,
      logLevel: 'warn',
      mode: isRelease ? 'production' : 'development',

      // Explicitly define process.env.NODE_ENV so Rolldown replaces it
      // correctly in CJS conditional entries (e.g. react's index.js).
      // Vite defaults NODE_ENV to "production" for build commands
      // regardless of the mode setting.
      define: {
        BLDR_DEBUG: JSON.stringify(!isRelease),
        'process.env.NODE_ENV': JSON.stringify(isRelease ? 'production' : 'development'),
      },

      // Disable config file lookup for web pkg builds.
      configFile: false,

      // Convert require() calls for external packages to ESM imports
      // and handle externalization. The esmExternalRequirePlugin MUST be
      // the sole externalizer for these packages: if they also appear in
      // rolldownOptions.external, Rolldown's built-in external resolution
      // runs first and the plugin never sees ImportKind::Require calls.
      // Also remap non-BldrExternal sibling web packages to /b/pkg/ URLs.
      plugins: [
        ...(allExternal.length > 0
          ? [esmExternalRequirePlugin({
              external: allExternal.map(
                (pkg) => new RegExp(`^${pkg.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(\\/.*)?$`),
              ),
            })]
          : []),
        ...(remapSiblingIds.length > 0
          ? [createWebPkgRemapPlugin({
              webPkgIDs: remapSiblingIds,
              debug: verboseDebug,
            })]
          : []),
      ],

      build: {
        outDir,
        emptyOutDir: true,
        manifest: true,
        cssCodeSplit: true,
        minify: isRelease ? 'esbuild' : false,
        sourcemap: isRelease ? false : 'inline',
        write: true,

        rolldownOptions: {
          input,
          preserveEntrySignatures: 'strict',

          output: {
            format: 'es',
            exports: 'named',
            entryFileNames: '[name].mjs',
            chunkFileNames: '[name]-[hash].mjs',
            assetFileNames: '[name]-[hash][extname]',
          },
        },
      },
    }

    // Disable color/noise.
    process.env['NO_COLOR'] = '1'
    process.env['NODE_DISABLE_COLORS'] = '1'
    process.env['CI'] = '1'
    process.env['FORCE_COLOR'] = '0'

    const viteOutput = (await viteBuild(config)) as
      | Rollup.RolldownOutput[]
      | Rollup.RolldownOutput
    const rollupOutputs: Rollup.RolldownOutput[] =
      Array.isArray(viteOutput) ? viteOutput : [viteOutput]
    const outputChunks = rollupOutputs.flatMap((o) => o.output)

    // Build import map entries from the output chunks.
    // Use chunk.name (the named input key) and chunk.fileName (hashed output).
    const importMapEntries: Array<{ specifier: string; outputPath: string }> = []
    for (const chunk of outputChunks) {
      if (chunk.type !== 'chunk' || !chunk.isEntry) continue

      // chunk.name is the named input key (e.g. "index", "jsx-runtime", "client").
      const baseName = chunk.name

      // Bare specifier: "react" for "index", "react/jsx-runtime" for subpaths.
      const specifier =
        baseName === 'index' ? pkgId : `${pkgId}/${baseName}`

      importMapEntries.push({
        specifier,
        outputPath: chunk.fileName,
      })
    }

    // Collect source files from output chunks.
    const sourceFiles = new Set<string>()
    for (const chunk of outputChunks) {
      if (chunk.type !== 'chunk') continue
      if (chunk.moduleIds) {
        for (const id of chunk.moduleIds) {
          const filePath = resolveTrackedSourceFile(pkgRoot, id)
          if (filePath) {
            sourceFiles.add(filePath)
          }
        }
      }
    }

    if (verboseDebug) {
      console.log(
        `[vite] buildWebPkg ${pkgId}: ${importMapEntries.length} import map entries, ${sourceFiles.size} source files`,
      )
    }

    return {
      success: true,
      sourceFiles: Array.from(sourceFiles),
      importMapEntries,
    }
  } catch (err) {
    console.error(`[vite] buildWebPkg error (${pkgId}):`, err)
    let errorMessage: string
    if (isBundleError(err)) {
      const messages = err.errors.map((e) => {
        const loc = e.loc
          ? ` (${e.loc.file}:${e.loc.line}:${e.loc.column})`
          : ''
        return `${e.message}${loc}`
      })
      errorMessage = messages.join('\n')
    } else if (isRollupError(err)) {
      errorMessage = err.message
    } else {
      errorMessage = err instanceof Error ? err.message : String(err)
    }
    return { success: false, error: errorMessage }
  }
}

// sleep returns a promise that resolves after the specified milliseconds.
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// connectWithRetry attempts to connect to the pipe with exponential backoff.
async function connectWithRetry(
  ipcPath: string,
  maxRetries = 5,
): Promise<net.Socket> {
  let lastError: Error | null = null

  for (let attempt = 0; attempt < maxRetries; attempt++) {
    // Exponential backoff: 100ms, 200ms, 400ms, 800ms, 1600ms (capped at 2000ms)
    const backoffMs = Math.min(100 * Math.pow(2, attempt), 2000)

    if (attempt > 0) {
      console.log(
        `[vite] retrying connection in ${backoffMs}ms (attempt ${attempt + 1}/${maxRetries})`,
      )
      await sleep(backoffMs)
    }

    const result = await new Promise<{ socket: net.Socket } | { error: Error }>(
      (resolve) => {
        const sock = net.connect(ipcPath, () => {
          resolve({ socket: sock })
        })
        sock.once('error', (err) => {
          lastError = err
          sock.destroy()
          resolve({ error: err })
        })
      },
    )

    if ('socket' in result) {
      console.log(`[vite] connected to pipe: ${ipcPath}`)
      return result.socket
    }

    console.warn(
      `[vite] connection attempt ${attempt + 1} failed: ${result.error}`,
    )
  }

  throw lastError || new Error('failed to connect after retries')
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

  // Connect to the pipe
  const socket = await connectWithRetry(ipcPath)

  // Set up bidirectional communication after successful connection
  const connection = createSocketConnection(socket, streamConn)
  startSocketSender(connection)

  // Handle socket errors after connection is established
  socket.on('error', (err) => {
    console.error(`[vite] socket error: ${err}`)
    process.exit(1)
  })
}

main().catch((err) => {
  console.error(`[vite] fatal error: ${err}`)
  process.exit(1)
})
