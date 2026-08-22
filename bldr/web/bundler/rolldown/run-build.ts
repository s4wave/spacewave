import { createHash } from 'node:crypto'
import { existsSync, readFileSync, realpathSync, statSync } from 'node:fs'
import { promises as fs } from 'node:fs'
import { gzipSync } from 'node:zlib'
import {
  basename,
  dirname,
  isAbsolute,
  join,
  normalize,
  posix,
  relative,
  resolve,
  sep,
} from 'node:path'
import { pathToFileURL } from 'node:url'
import type {
  BuildOutput,
  BuildRequest,
  BuildResult,
  Diagnostic,
  Entrypoint,
  ToolIdentity,
} from './rolldown.pb.js'

type LogLocation = {
  file?: string
  line?: number
  column?: number
}

type RolldownLog = {
  code?: string
  message?: string
  id?: string
  loc?: LogLocation
  frame?: string
}

type OutputChunk = {
  type: 'chunk'
  fileName: string
  code: string
  isEntry: boolean
  facadeModuleId: string | null
  sourcemapFileName: string | null
}

type OutputAsset = {
  type: 'asset'
  fileName: string
  source: string | Uint8Array
}

type RolldownOutput = {
  output: Array<OutputChunk | OutputAsset>
}

type PluginLoadResult = {
  code: string | Uint8Array
  moduleType?: string
}
type Plugin = {
  name: string
  buildStart?: () => void
  resolveId?: (
    source: string,
    importer?: string,
  ) => string | { id: string; external?: boolean } | null
  load?: (id: string) => string | Uint8Array | PluginLoadResult | null
  transform?: (code: string, id: string) => string | PluginLoadResult | null
  renderChunk?: (code: string) => string
  resolveFileUrl?: (args: { fileName: string }) => string
}

type BuildOptions = {
  input: Record<string, string>
  cwd: string
  write: true
  platform: 'browser' | 'node' | 'neutral'
  external: string[]
  treeshake: boolean
  checks: { importIsUndefined: boolean }
  moduleTypes: Record<string, string>
  resolve: {
    alias: Record<string, string>
    extensionAlias: Record<string, string[]>
    modules: string[]
  }
  transform: {
    target?: string
    define: Record<string, string>
    inject?: Record<string, string | [string, string]>
  }
  plugins: Plugin[]
  onLog: (
    level: string,
    log: RolldownLog,
    defaultHandler: (level: string, log: RolldownLog) => void,
  ) => void
  logLevel: 'warn'
  output: {
    dir: string
    format: 'es' | 'cjs' | 'iife'
    name?: string
    entryFileNames: string
    chunkFileNames: string
    assetFileNames: string
    sourcemap: false | true | 'inline'
    codeSplitting: boolean
    minify: false | { compress: true; mangle: true }
    comments: false
    banner?: string
    cleanDir: boolean
  }
}

type RolldownModule = {
  build: (options: BuildOptions) => Promise<RolldownOutput>
}

type RunnerError = Error & { diagnostic?: Diagnostic }

const FORMAT_VALUES = new Set(['es', 'cjs', 'iife'])
const PLATFORM_VALUES = new Set(['browser', 'node', 'neutral'])
const SOURCEMAP_VALUES = new Set(['none', 'inline', 'external', 'both'])
const LOADER_VALUES = new Set([
  'js',
  'jsx',
  'ts',
  'tsx',
  'json',
  'text',
  'dataurl',
  'base64',
  'binary',
  'asset',
])
const DIST_SOURCE_PREFIXES = [
  'devtool/',
  'manifest/',
  'plugin/',
  'resource/',
  'sdk/',
  'web/',
]
const LOCAL_MODULE_PREFIX = 'github.com/s4wave/spacewave/'
const NODE_EVENTS_ID = '\0goscript-node-events'

const CSS_IMPORT_PATTERN =
  /\.(?:css|less|sass|scss|styl|stylus|pcss|postcss)(?:[?#].*)?$/i
const IMPORT_SPECIFIER_PATTERN =
  /(?:\b(?:import|export)\s+(?:[^'"]*?\sfrom\s*)?|\bimport\s*\(\s*|\brequire\s*\(\s*)['"]([^'"]+)['"]/g

function sourceHasCssImport(code: string): boolean {
  IMPORT_SPECIFIER_PATTERN.lastIndex = 0
  for (const match of code.matchAll(IMPORT_SPECIFIER_PATTERN)) {
    if (CSS_IMPORT_PATTERN.test(match[1] ?? '')) return true
  }
  return false
}
export class BuildRequestError extends Error {
  public readonly diagnostic: Diagnostic

  public constructor(message: string) {
    super(message)
    this.name = 'BuildRequestError'
    this.diagnostic = { severity: 'error', message }
  }
}

export function validateBuildRequest(request: BuildRequest): void {
  const format = request.format ?? ''
  if (format && !FORMAT_VALUES.has(format)) {
    throw new BuildRequestError(`unsupported format ${JSON.stringify(format)}`)
  }
  if (format === 'iife' && !request.globalName) {
    throw new BuildRequestError('format iife requires global_name')
  }
  const platform = request.platform ?? ''
  if (platform && !PLATFORM_VALUES.has(platform)) {
    throw new BuildRequestError(
      `unsupported platform ${JSON.stringify(platform)}`,
    )
  }
  const sourcemap = request.sourcemap ?? ''
  if (sourcemap && !SOURCEMAP_VALUES.has(sourcemap)) {
    throw new BuildRequestError(
      `unsupported sourcemap ${JSON.stringify(sourcemap)}`,
    )
  }
  for (const [extension, loader] of Object.entries(request.loaders ?? {})) {
    if (extension.toLowerCase().endsWith('.css') || loader === 'css') {
      throw new BuildRequestError(
        'CSS loader configuration is not supported by Rolldown owner',
      )
    }
    if (!LOADER_VALUES.has(loader)) {
      throw new BuildRequestError(
        `unsupported loader ${JSON.stringify(loader)}`,
      )
    }
  }
}

export async function runBuild(
  request: BuildRequest,
  dependencyRoot: string,
): Promise<BuildResult> {
  validateBuildRequest(request)

  const workingDir = resolve(request.workingDir || process.cwd())
  const sourceRoot = resolve(workingDir, request.sourceRoot || '.')
  const outputRoot = resolve(workingDir, request.outputRoot || 'dist')
  const format = (request.format || 'es') as 'es' | 'cjs' | 'iife'
  const platform = (request.platform || 'browser') as
    | 'browser'
    | 'node'
    | 'neutral'
  const sourcemap = request.sourcemap || 'none'
  const entrypoints = request.entrypoints ?? []
  const inputs: Set<string> = new Set()
  const diagnostics: Diagnostic[] = []
  const inputEntries: Record<string, string> = {}
  let hasCssImports = false
  const virtualModules = new Map<string, string>()
  const canonicalPath = (filePath: string): string => {
    const absolute = normalize(resolve(workingDir, filePath))
    try {
      return normalize(realpathSync.native(absolute))
    } catch {
      return absolute
    }
  }

  const sourceOverrides = new Map(
    Object.entries(request.sourceOverrides ?? {}).map(([filePath, source]) => [
      canonicalPath(filePath),
      source,
    ]),
  )
  const injectedPaths = (request.inject ?? []).map(canonicalPath)
  // entryInjects maps each canonical entry path to its injected module
  // paths in request order.
  const entryInjects = new Map<string, string[]>()
  const entrypointPaths = new Set<string>()
  for (const entrypoint of entrypoints) {
    const name = entrypoint.name || basename(entrypoint.inputPath || 'entry')
    if (!entrypoint.inputPath) {
      throw new BuildRequestError(
        `entrypoint ${JSON.stringify(name)} has no input_path`,
      )
    }
    const inputPath = canonicalPath(entrypoint.inputPath)
    entrypointPaths.add(inputPath)
    if (injectedPaths.length > 0) {
      // esbuild-style inject: the injected modules become side-effect
      // imports prepended ahead of the real entry source, so the entry
      // keeps its own exports, including arbitrary named exports, its
      // default export, or none at all.
      entryInjects.set(inputPath, injectedPaths)
    }
    inputEntries[name] = inputPath
  }
  for (const [name, source] of Object.entries(request.virtualModules ?? {})) {
    virtualModules.set(name, source)
  }

  const dependencyModulesRoot = canonicalPath(
    join(dependencyRoot, 'node_modules'),
  )
  const trackInput = (filePath: string | undefined) => {
    if (!filePath || filePath.startsWith('\0') || !isAbsolute(filePath)) return
    if (!existsSync(filePath)) return
    let normalizedPath: string
    try {
      if (!statSync(filePath).isFile()) return
      normalizedPath = normalize(realpathSync.native(filePath))
    } catch {
      normalizedPath = normalize(filePath)
    }
    const dependencyRelative = relative(dependencyModulesRoot, normalizedPath)
    if (
      dependencyRelative === '' ||
      (!isAbsolute(dependencyRelative) &&
        dependencyRelative !== '..' &&
        !dependencyRelative.startsWith(`..${sep}`))
    ) {
      return
    }
    inputs.add(normalizedPath)
  }
  const existingFile = (filePath: string): boolean => {
    try {
      return statSync(filePath).isFile()
    } catch {
      return false
    }
  }
  const existingSourcePath = (filePath: string): string | null => {
    if (existingFile(filePath)) return filePath
    if (filePath.endsWith('.js')) {
      for (const extension of ['.ts', '.tsx']) {
        const candidate = filePath.slice(0, -3) + extension
        if (existingFile(candidate)) return candidate
      }
    }
    return null
  }
  const existingTypeScriptSibling = (filePath: string): string | null => {
    if (filePath.endsWith('.js')) {
      const candidate = filePath.slice(0, -3) + '.ts'
      if (existingFile(candidate)) return candidate
    }
    return existingFile(filePath) ? filePath : null
  }
  const localModule = readLocalModule(sourceRoot)
  const goscript = request.goscript
  const goScriptOutputRoot = resolve(
    workingDir,
    goscript?.outputRoot || outputRoot,
  )
  const bldrDistRoot = canonicalPath(request.bldrDistRoot || sourceRoot)
  for (const configFile of [
    join(bldrDistRoot, 'dist', 'deps', 'package.json'),
    join(bldrDistRoot, 'dist', 'deps', 'bun.lock'),
    join(bldrDistRoot, 'bldr', 'dist', 'deps', 'package.json'),
    join(bldrDistRoot, 'bldr', 'dist', 'deps', 'bun.lock'),
  ]) {
    trackInput(configFile)
  }

  const resolveBldrSourcePath = (sourceRel: string): string | null => {
    return (
      existingSourcePath(join(bldrDistRoot, 'bldr', sourceRel)) ??
      existingSourcePath(join(bldrDistRoot, sourceRel))
    )
  }
  const resolveBldrAlias = (source: string): string | null => {
    if (source === '@aptre/bldr-sdk')
      return resolveBldrSourcePath('sdk/plugin.ts')
    const sdkPrefix = '@aptre/bldr-sdk/'
    if (source.startsWith(sdkPrefix)) {
      return resolveBldrSourcePath(join('sdk', source.slice(sdkPrefix.length)))
    }
    if (source === '@aptre/bldr')
      return resolveBldrSourcePath('web/bldr/index.js')
    if (source === '@aptre/bldr-react') {
      return resolveBldrSourcePath('web/bldr-react/index.js')
    }
    return null
  }
  const resolveGoImport = (source: string): string | null => {
    if (!source.startsWith('@go/') || !source.endsWith('.js')) return null
    const importPath = source.slice('@go/'.length)
    if (localModule && importPath.startsWith(`${localModule.name}/`)) {
      return existingSourcePath(
        join(localModule.root, importPath.slice(localModule.name.length + 1)),
      )
    }
    if (!localModule && importPath.startsWith(LOCAL_MODULE_PREFIX)) {
      return existingSourcePath(
        join(sourceRoot, importPath.slice(LOCAL_MODULE_PREFIX.length)),
      )
    }
    for (const root of [localModule?.root, bldrDistRoot]) {
      if (!root) continue
      const resolved = existingSourcePath(join(root, 'vendor', importPath))
      if (resolved) return resolved
    }
    return null
  }
  const resolveDistSourceImport = (source: string): string | null => {
    if (!source.endsWith('.js')) return null
    if (!DIST_SOURCE_PREFIXES.some((prefix) => source.startsWith(prefix))) {
      return null
    }
    return existingSourcePath(join(bldrDistRoot, source))
  }
  const resolveEscapedRelativeImport = (
    source: string,
    importer: string | undefined,
  ): string | null => {
    if (
      !importer ||
      importer.startsWith('\0') ||
      !source.endsWith('.js') ||
      (!source.startsWith('./') && !source.startsWith('../'))
    ) {
      return null
    }
    const relativeImporter = relative(bldrDistRoot, importer)
    if (
      relativeImporter === '..' ||
      relativeImporter.startsWith(`..${sep}`) ||
      isAbsolute(relativeImporter)
    ) {
      return null
    }
    const target = normalize(join(dirname(importer), source))
    const relativeTarget = relative(bldrDistRoot, target)
    if (relativeTarget !== '..' && !relativeTarget.startsWith(`..${sep}`)) {
      return null
    }
    const moduleImporter = posix.join(
      LOCAL_MODULE_PREFIX,
      'bldr',
      relativeImporter.split(sep).join('/'),
    )
    const modulePath = posix.normalize(
      posix.join(posix.dirname(moduleImporter), source),
    )
    return resolveGoImport(`@go/${modulePath}`)
  }
  const isConfiguredExternal = (source: string): boolean =>
    (request.external ?? []).some(
      (specifier) => source === specifier || source.startsWith(`${specifier}/`),
    )
  const configuredAliases = Object.entries(request.aliases ?? {})
  const prefixAliases = Object.entries(request.prefixAliases ?? {}).sort(
    ([left], [right]) => right.length - left.length,
  )
  const resolveConfiguredAlias = (source: string): string | null => {
    for (const [specifier, target] of configuredAliases) {
      if (source === specifier) return existingSourcePath(canonicalPath(target))
    }
    for (const [prefix, aliasRoot] of prefixAliases) {
      if (!source.startsWith(prefix)) continue
      const rest = source.slice(prefix.length).replace(/^[/\\]+/, '')
      const resolved = existingSourcePath(join(canonicalPath(aliasRoot), rest))
      if (resolved) return resolved
    }
    return null
  }
  const sharedImportURL = (relativePath: string): string => {
    const prefix = goscript?.sharedImportUrlPrefix || ''
    return `${prefix}${relativePath.slice(0, -3)}.mjs`
  }
  const sharedGoScriptRel = (relativePath: string): boolean =>
    relativePath !== '' && !relativePath.startsWith('github.com/s4wave/')
  const resolveSharedGoScriptImport = (
    source: string,
    importer: string | undefined,
  ): string | null => {
    if (!goscript?.sharedExternalImports) return null
    if (source.startsWith('@goscript/')) {
      const rel = source.slice('@goscript/'.length)
      if (!rel.endsWith('.js') || !sharedGoScriptRel(rel)) return null
      return sharedImportURL(rel)
    }
    if (
      !importer ||
      importer.startsWith('\0') ||
      !source.endsWith('.js') ||
      (!source.startsWith('./') && !source.startsWith('../'))
    ) {
      return null
    }
    const outputRoot = join(goScriptOutputRoot, '@goscript')
    const targetPath = normalize(join(dirname(importer), source))
    const rel = relative(outputRoot, targetPath).split(sep).join('/')
    if (rel === '' || rel.startsWith('..') || isAbsolute(rel)) return null
    if (!sharedGoScriptRel(rel) || !existingTypeScriptSibling(targetPath))
      return null
    return sharedImportURL(rel)
  }
  const resolveGoScriptOverrideSourceImport = (
    source: string,
    importer: string | undefined,
  ): string | null => {
    if (
      !importer ||
      importer.startsWith('\0') ||
      !source.endsWith('.js') ||
      (!source.startsWith('./') && !source.startsWith('../'))
    ) {
      return null
    }
    const outputRoot = join(goScriptOutputRoot, '@goscript')
    const targetPath = normalize(join(dirname(importer), source))
    const rel = relative(outputRoot, targetPath)
    if (rel === '' || rel.startsWith('..') || isAbsolute(rel)) return null
    return existingSourcePath(
      join(sourceRoot, 'vendor', 'github.com', 's4wave', 'goscript', 'gs', rel),
    )
  }

  const internalResolver: Plugin = {
    name: 'bldr-internal-resolver',
    buildStart() {
      for (const input of Object.values(inputEntries)) trackInput(input)
      for (const input of entrypointPaths) trackInput(input)
      for (const input of injectedPaths) trackInput(input)
      for (const input of sourceOverrides.keys()) trackInput(input)
    },
    resolveId(source, importer) {
      if (virtualModules.has(source)) return `\0virtual:${source}`
      if (goscript && source === 'node:events') return NODE_EVENTS_ID
      if (isConfiguredExternal(source)) {
        return { id: source, external: true }
      }
      const configuredAlias = resolveConfiguredAlias(source)
      if (configuredAlias) {
        trackInput(configuredAlias)
        return configuredAlias
      }
      const bldrAlias = resolveBldrAlias(source)
      if (bldrAlias) {
        trackInput(bldrAlias)
        return bldrAlias
      }
      const goImport = resolveGoImport(source)
      if (goImport) {
        trackInput(goImport)
        return goImport
      }
      const distSourceImport = resolveDistSourceImport(source)
      if (distSourceImport) {
        trackInput(distSourceImport)
        return distSourceImport
      }
      const escapedRelativeImport = resolveEscapedRelativeImport(
        source,
        importer,
      )
      if (escapedRelativeImport) {
        trackInput(escapedRelativeImport)
        return escapedRelativeImport
      }
      if (request.externalPackages && isBarePackageImport(source)) {
        return { id: source, external: true }
      }
      if (!goscript) return null
      if (source.startsWith('@goscript/')) {
        const sharedImport = resolveSharedGoScriptImport(source, importer)
        if (sharedImport) return { id: sharedImport, external: true }
        const resolved = existingTypeScriptSibling(
          join(
            goScriptOutputRoot,
            '@goscript',
            source.slice('@goscript/'.length),
          ),
        )
        if (resolved) {
          trackInput(resolved)
          return resolved
        }
        return null
      }
      if (
        importer &&
        !importer.startsWith('\0') &&
        source.endsWith('.js') &&
        (source.startsWith('./') || source.startsWith('../'))
      ) {
        const sharedImport = resolveSharedGoScriptImport(source, importer)
        if (sharedImport) return { id: sharedImport, external: true }
        const resolved = existingTypeScriptSibling(
          join(dirname(importer), source),
        )
        if (resolved) {
          trackInput(resolved)
          return resolved
        }
        const overrideSource = resolveGoScriptOverrideSourceImport(
          source,
          importer,
        )
        if (overrideSource) {
          trackInput(overrideSource)
          return overrideSource
        }
      }
      return null
    },
    load(id) {
      if (id === NODE_EVENTS_ID) return 'export function setMaxListeners() {}\n'
      if (id.startsWith('\0virtual:'))
        return virtualModules.get(id.slice(9)) ?? null
      const normalizedID = normalize(id)
      trackInput(normalizedID)
      const overrideSource = sourceOverrides.get(normalizedID)
      const injects = entryInjects.get(normalizedID)
      if (!injects) return overrideSource ?? null
      const original = overrideSource ?? readFileSync(normalizedID, 'utf8')
      const imports = injects
        .map((filePath) => `import ${JSON.stringify(filePath)};`)
        .join('\n')
      return `${imports}\n${original}`
    },
    transform(code, id) {
      if (request.routeCssImports && sourceHasCssImport(code)) {
        hasCssImports = true
        trackInput(id)
      }
      return null
    },
  }
  const plugins: Plugin[] = [
    {
      name: 'virtual-modules',
      resolveId(source) {
        return virtualModules.has(source) ? `\0virtual:${source}` : null
      },
      load(id) {
        return id.startsWith('\0virtual:')
          ? (virtualModules.get(id.slice(9)) ?? null)
          : null
      },
    },
    internalResolver,
  ]
  if (!request.sourcemap || request.sourcemap === 'none') {
    plugins.unshift({
      name: 'strip-code-regions',
      renderChunk(code) {
        return code.replace(/^\/\/#(?:end)?region.*(?:\r?\n|$)/gm, '')
      },
    })
  }
  if (request.publicPath) {
    plugins.push({
      name: 'public-path',
      resolveFileUrl({ fileName }) {
        return JSON.stringify(joinURL(request.publicPath || '', fileName))
      },
    })
  }

  const recordLog = (level: string, log: RolldownLog) => {
    const loc = log.loc
    const file = loc?.file || log.id || ''
    const line = loc?.line || 0
    const column = loc?.column || 0
    let lineText = ''
    if (file && line > 0 && existsSync(file)) {
      try {
        lineText = readFileSync(file, 'utf8').split(/\r?\n/)[line - 1] || ''
      } catch {
        lineText = ''
      }
    }
    diagnostics.push({
      severity: level === 'warn' ? 'warning' : level,
      message: log.message || log.code || 'Rolldown diagnostic',
      code: log.code || '',
      file,
      line,
      column,
      lineText,
    })
  }
  const isUndefinedImport = (log: RolldownLog): boolean => {
    const message = log.message || ''
    return (
      log.code === 'IMPORT_IS_UNDEFINED' ||
      log.code === 'import-is-undefined' ||
      log.code === 'ImportIsUndefined' ||
      message.includes(
        'will always be undefined because there is no matching export',
      )
    )
  }

  const outputSourcemap: false | true | 'inline' =
    sourcemap === 'none' || sourcemap === 'both'
      ? sourcemap === 'both'
        ? true
        : false
      : sourcemap === 'inline'
        ? 'inline'
        : true

  const options: BuildOptions = {
    input: inputEntries,
    cwd: workingDir,
    write: true,
    platform,
    external: [...(request.external ?? [])],
    treeshake: request.treeShaking ?? true,
    checks: { importIsUndefined: true },
    moduleTypes: { ...(request.loaders ?? {}) },
    resolve: {
      alias: { ...(request.aliases ?? {}) },
      extensionAlias: { '.js': ['.ts', '.tsx', '.js'] },
      modules: [join(dependencyRoot, 'node_modules')],
    },
    transform: {
      target: request.target || undefined,
      define: { ...(request.defines ?? {}) },
    },
    plugins,
    onLog(level, log, _defaultHandler) {
      recordLog(level, log)
      if (isUndefinedImport(log)) {
        const error = new Error(
          `undefined GoScript import${log.id ? ` in ${log.id}` : ''}: ${log.message || log.code || 'missing export'}`,
        ) as RunnerError
        error.diagnostic = {
          ...(diagnostics[diagnostics.length - 1] ?? {}),
          severity: 'error',
          message: error.message,
        }
        throw error
      }
    },
    logLevel: 'warn',
    output: {
      dir: outputRoot,
      format,
      name: request.globalName || undefined,
      entryFileNames: request.entryFileNames || '[name].js',
      chunkFileNames: request.chunkFileNames || '[name]-[hash].js',
      assetFileNames: request.assetFileNames || '[name]-[hash][extname]',
      sourcemap: outputSourcemap,
      codeSplitting: request.codeSplitting ?? false,
      minify: request.minify ? { compress: true, mangle: true } : false,
      comments: false,
      banner: request.banner || undefined,
      cleanDir: request.cleanOutputDir ?? false,
    },
  }

  try {
    await fs.mkdir(outputRoot, { recursive: true })
    const modulePath = join(
      dependencyRoot,
      'node_modules',
      'rolldown',
      'dist',
      'index.mjs',
    )
    const moduleNamespace = (await import(
      pathToFileURL(modulePath).href
    )) as unknown as RolldownModule
    const output = await moduleNamespace.build(options)
    await appendBothSourcemaps(outputRoot, output.output, sourcemap === 'both')
    const buildOutputs = await collectOutputs(
      outputRoot,
      output.output,
      entrypoints,
    )
    const tool = await identifyTool(dependencyRoot)
    return {
      inputs: [...inputs].sort(),
      outputs: buildOutputs,
      entrypointOutputs: entrypointOutputMap(buildOutputs),
      tool,
      diagnostics,
      hasCssImports,
    }
  } catch (value) {
    const error = toError(value)
    if (request.routeCssImports && hasCssImports) {
      return {
        inputs: [...inputs].sort(),
        outputs: [],
        entrypointOutputs: {},
        tool: await identifyTool(dependencyRoot),
        diagnostics,
        hasCssImports: true,
      }
    }
    if (error.diagnostic) diagnostics.push(error.diagnostic)
    else if (
      !diagnostics.some((diagnostic) => diagnostic.message === error.message)
    ) {
      diagnostics.push(diagnosticFromError(value, error))
    }
    return {
      inputs: [...inputs].sort(),
      outputs: [],
      entrypointOutputs: {},
      tool: await identifyTool(dependencyRoot),
      diagnostics,
      hasCssImports,
    }
  }
}

function isBarePackageImport(source: string): boolean {
  return (
    source !== '' &&
    !source.startsWith('.') &&
    !source.startsWith('/') &&
    !source.startsWith('\0') &&
    !/^[A-Za-z]:[\\/]/.test(source)
  )
}

type LocalModule = {
  name: string
  root: string
}

function readLocalModule(sourceRoot: string): LocalModule | null {
  let root = resolve(sourceRoot)
  for (;;) {
    let contents: string
    try {
      contents = readFileSync(join(root, 'go.mod'), 'utf8')
    } catch {
      const parent = dirname(root)
      if (parent === root) return null
      root = parent
      continue
    }
    const name = contents.match(/^\s*module\s+(\S+)/m)?.[1] || ''
    if (name) return { name, root }
    const parent = dirname(root)
    if (parent === root) return null
    root = parent
  }
}

function joinURL(prefix: string, fileName: string): string {
  if (!prefix) return fileName
  return `${prefix.replace(/\/$/, '')}/${fileName.replace(/^\//, '')}`
}

async function appendBothSourcemaps(
  outputRoot: string,
  outputs: Array<OutputChunk | OutputAsset>,
  enabled: boolean,
): Promise<void> {
  if (!enabled) return
  for (const output of outputs) {
    if (output.type !== 'chunk') continue
    const mapName = output.sourcemapFileName || `${output.fileName}.map`
    const mapPath = join(outputRoot, mapName)
    if (!existsSync(mapPath)) continue
    const mapBytes = await fs.readFile(mapPath)
    const outputPath = join(outputRoot, output.fileName)
    const contents = await fs.readFile(outputPath, 'utf8')
    const body = contents
      .replace(/\s*\/\/# sourceMappingURL=.*$/s, '')
      .replace(/\s*$/, '')
    await fs.writeFile(
      outputPath,
      `${body}\n//# sourceMappingURL=data:application/json;base64,${mapBytes.toString('base64')}\n`,
    )
  }
}

async function collectOutputs(
  outputRoot: string,
  outputs: Array<OutputChunk | OutputAsset>,
  entrypoints: Entrypoint[],
): Promise<BuildOutput[]> {
  const result: BuildOutput[] = []
  for (const output of outputs) {
    const path = normalizeOutputPath(outputRoot, output.fileName)
    const contents = await fs.readFile(join(outputRoot, path))
    const type = output.fileName.endsWith('.map')
      ? 'map'
      : output.type === 'chunk'
        ? 'javascript'
        : 'asset'
    const entrypointName =
      output.type === 'chunk' && output.isEntry
        ? entrypointForFacade(output.facadeModuleId, entrypoints)
        : ''
    result.push({
      path,
      type,
      entrypointName,
      bytes: BigInt(contents.byteLength),
      gzipBytes: BigInt(gzipSync(contents, { level: 9 }).byteLength),
      sha256: createHash('sha256').update(contents).digest('hex'),
    })
  }
  return result.sort((left, right) =>
    (left.path ?? '').localeCompare(right.path ?? ''),
  )
}

function normalizeOutputPath(outputRoot: string, outputPath: string): string {
  const absolute = resolve(outputRoot, outputPath)
  const relativePath = normalize(relative(outputRoot, absolute))
  if (
    relativePath === '..' ||
    relativePath.startsWith(`..${sep}`) ||
    isAbsolute(relativePath)
  ) {
    throw new Error(
      `Rolldown emitted output outside output root: ${outputPath}`,
    )
  }
  return relativePath.split(sep).join('/')
}

function entrypointForFacade(
  facade: string | null,
  entrypoints: Entrypoint[],
): string {
  if (!facade) return ''
  const normalizedFacade = normalizeExistingPath(facade)
  return (
    entrypoints.find(
      (entrypoint) =>
        normalizeExistingPath(entrypoint.inputPath || '') === normalizedFacade,
    )?.name || ''
  )
}

function normalizeExistingPath(filePath: string): string {
  if (!filePath) return ''
  const absolute = normalize(resolve(filePath))
  try {
    return normalize(realpathSync.native(absolute))
  } catch {
    return absolute
  }
}

function entrypointOutputMap(outputs: BuildOutput[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const output of outputs) {
    if (output.entrypointName && output.path && output.type === 'javascript') {
      result[output.entrypointName] = output.path
    }
  }
  return result
}

async function identifyTool(dependencyRoot: string): Promise<ToolIdentity> {
  let rolldownVersion = ''
  try {
    const packageJSON = JSON.parse(
      await fs.readFile(
        join(dependencyRoot, 'node_modules', 'rolldown', 'package.json'),
        'utf8',
      ),
    ) as { version?: string }
    rolldownVersion = packageJSON.version || ''
  } catch {
    rolldownVersion = ''
  }
  return {
    rolldownVersion,
    bunVersion: process.versions.bun ?? '',
    platform: process.platform,
    arch: process.arch,
  }
}

function toError(value: unknown): RunnerError {
  if (value instanceof Error) return value as RunnerError
  return Object.assign(new Error(String(value)), { diagnostic: undefined })
}

function diagnosticFromError(value: unknown, error: Error): Diagnostic {
  const candidate =
    typeof value === 'object' && value !== null ? value : undefined
  const log = candidate as RolldownLog | undefined
  const loc = log?.loc
  return {
    severity: 'error',
    message: error.message,
    code: log?.code || '',
    file: loc?.file || log?.id || '',
    line: loc?.line || 0,
    column: loc?.column || 0,
    lineText: log?.frame || '',
  }
}

type JsonValue = Record<string, unknown>

function requestFromJSON(value: unknown): BuildRequest {
  const input = (value && typeof value === 'object' ? value : {}) as JsonValue
  const read = (camel: string, snake: string): unknown =>
    input[snake] ?? input[camel]
  const entrypoints = (
    read('entrypoints', 'entrypoints') as unknown[] | undefined
  )?.map((entrypoint) => {
    const item = entrypoint as JsonValue
    return {
      name: (item.name as string | undefined) || '',
      inputPath: (item.input_path ?? item.inputPath ?? '') as string,
    }
  })
  return {
    workingDir: read('workingDir', 'working_dir') as string | undefined,
    sourceRoot: read('sourceRoot', 'source_root') as string | undefined,
    outputRoot: read('outputRoot', 'output_root') as string | undefined,
    entrypoints,
    format: read('format', 'format') as string | undefined,
    globalName: read('globalName', 'global_name') as string | undefined,
    platform: read('platform', 'platform') as string | undefined,
    target: read('target', 'target') as string | undefined,
    entryFileNames: read('entryFileNames', 'entry_file_names') as
      | string
      | undefined,
    chunkFileNames: read('chunkFileNames', 'chunk_file_names') as
      | string
      | undefined,
    assetFileNames: read('assetFileNames', 'asset_file_names') as
      | string
      | undefined,
    publicPath: read('publicPath', 'public_path') as string | undefined,
    codeSplitting: read('codeSplitting', 'code_splitting') as
      | boolean
      | undefined,
    sourcemap: read('sourcemap', 'sourcemap') as string | undefined,
    minify: read('minify', 'minify') as boolean | undefined,
    treeShaking: read('treeShaking', 'tree_shaking') as boolean | undefined,
    banner: read('banner', 'banner') as string | undefined,
    defines: read('defines', 'defines') as Record<string, string> | undefined,
    external: read('external', 'external') as string[] | undefined,
    aliases: read('aliases', 'aliases') as Record<string, string> | undefined,
    loaders: read('loaders', 'loaders') as Record<string, string> | undefined,
    virtualModules: read('virtualModules', 'virtual_modules') as
      | Record<string, string>
      | undefined,
    goscript: (() => {
      const policy = read('goscript', 'goscript') as JsonValue | undefined
      if (!policy) return undefined
      return {
        outputRoot: (policy.output_root ?? policy.outputRoot ?? '') as string,
        sharedExternalImports: (policy.shared_external_imports ??
          policy.sharedExternalImports ??
          false) as boolean,
        sharedImportUrlPrefix: (policy.shared_import_url_prefix ??
          policy.sharedImportUrlPrefix ??
          '') as string,
      }
    })(),
    inject: read('inject', 'inject') as string[] | undefined,
    bldrDistRoot: read('bldrDistRoot', 'bldr_dist_root') as string | undefined,
    cleanOutputDir: read('cleanOutputDir', 'clean_output_dir') as
      | boolean
      | undefined,
    externalPackages: read('externalPackages', 'external_packages') as
      | boolean
      | undefined,
    sourceOverrides: read('sourceOverrides', 'source_overrides') as
      | Record<string, string>
      | undefined,
    prefixAliases: read('prefixAliases', 'prefix_aliases') as
      | Record<string, string>
      | undefined,
    routeCssImports: read('routeCssImports', 'route_css_imports') as
      | boolean
      | undefined,
  }
}

function resultToJSON(result: BuildResult): JsonValue {
  return {
    inputs: result.inputs ?? [],
    outputs: (result.outputs ?? []).map((output) => ({
      path: output.path || '',
      type: output.type || '',
      entrypoint_name: output.entrypointName || '',
      bytes: String(output.bytes ?? 0n),
      gzip_bytes: String(output.gzipBytes ?? 0n),
      sha256: output.sha256 || '',
    })),
    entrypoint_outputs: result.entrypointOutputs ?? {},
    tool: {
      rolldown_version: result.tool?.rolldownVersion || '',
      bun_version: result.tool?.bunVersion || '',
      platform: result.tool?.platform || '',
      arch: result.tool?.arch || '',
    },
    diagnostics: (result.diagnostics ?? []).map((diagnostic) => ({
      severity: diagnostic.severity || '',
      message: diagnostic.message || '',
      code: diagnostic.code || '',
      file: diagnostic.file || '',
      line: diagnostic.line || 0,
      column: diagnostic.column || 0,
      line_text: diagnostic.lineText || '',
    })),
    has_css_imports: result.hasCssImports ?? false,
  }
}

function failureResult(value: unknown): BuildResult {
  const error = toError(value)
  return {
    inputs: [],
    outputs: [],
    entrypointOutputs: {},
    tool: {
      rolldownVersion: '',
      bunVersion: process.versions.bun ?? '',
      platform: process.platform,
      arch: process.arch,
    },
    diagnostics: [error.diagnostic || diagnosticFromError(value, error)],
  }
}

async function main(): Promise<void> {
  const [, , requestPath, resultPath, dependencyRoot] = process.argv
  if (!requestPath || !resultPath || !dependencyRoot) {
    throw new BuildRequestError(
      'usage: run-build.ts <request.json> <result.json> <dependency-root>',
    )
  }
  let result: BuildResult
  try {
    const request = requestFromJSON(
      JSON.parse(await fs.readFile(requestPath, 'utf8')),
    )
    result = await runBuild(request, dependencyRoot)
  } catch (value) {
    result = failureResult(value)
  }
  await fs.writeFile(
    resultPath,
    `${JSON.stringify(resultToJSON(result), null, 2)}\n`,
  )
  if (
    (result.diagnostics ?? []).some(
      (diagnostic) => diagnostic.severity === 'error',
    )
  ) {
    process.exitCode = 1
  }
}

if (import.meta.main) {
  void main().catch((error: unknown) => {
    process.exitCode = 1
    console.error(error)
  })
}
