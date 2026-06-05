#!/usr/bin/env bun

import { spawn } from 'child_process'
import { createWriteStream } from 'fs'
import {
  mkdir,
  readdir,
  readFile,
  realpath,
  stat,
  writeFile,
} from 'fs/promises'
import { dirname, join, relative, resolve } from 'path'
import { gzipSync } from 'zlib'
import { rolldown, type Plugin } from 'rolldown'

const goscriptModule =
  'github.com/aperturerobotics/goscript/cmd/goscript@d48c209543378e8b438e00702e32cf8b312fc67c'

interface Options {
  outDir: string
  moduleDir: string
  tsDir: string
  fromTs: string
  buildFlags: string[]
  packageNames: string[]
  mainPackagePath: string
  bldrDistRoot: string
  productionWrapperReport: string
  skipCompile: boolean
  skipBundle: boolean
  protobufTypeScriptBinding: boolean
  top: number
}

interface FileRecord {
  path: string
  rel: string
  bytes: number
}

interface CommandResult {
  command: string
  args: string[]
  logPath: string
}

interface ProductionWrapperReport {
  schemaVersion: number
  outputPath: string
  outputBytes: number
  outputGzipBytes: number
  minify: boolean
  sourcemaps: boolean
  inputCount: number
  inputPaths: string[]
}

function usage(code = 2): never {
  console.error(`usage: bun scripts/goscript-size-report.ts [options]

Options:
  --out <dir>                 output report directory under .tmp/
  --module-dir <dir>          generated Go module to compile
  --from-ts <dir>             reuse an existing GoScript TypeScript tree
  --build-flags <flags>       build flags passed to goscript; repeatable
  --package <pkg>             Go package passed to goscript; repeatable
  --main-package-path <path>  main package import path for wrapper bundling
  --bldr-dist-root <dir>      source root containing web/runtime/goscript
  --production-wrapper-report <file>
                              include production Bldr GoScript wrapper report JSON
  --skip-compile              do not run goscript compile
  --skip-bundle               do not run Rolldown/Oxc bundle probes
  --no-protobuf-ts-binding    emit .pb.gs.ts files instead of binding sibling .pb.ts files
  --top <n>                   number of largest files to include

Default workload is the existing spacewave-core GoScript module:
  .bldr/build/web/js/wasm/spacewave-core
`)
  process.exit(code)
}

function parseArgs(): Options {
  const raw = process.argv.slice(2)
  const opts: Options = {
    outDir: join('.tmp', `goscript-spacewave-core-size-${timestamp()}`),
    moduleDir: join('.bldr', 'build', 'web', 'js', 'wasm', 'spacewave-core'),
    tsDir: '',
    fromTs: '',
    buildFlags: ['-tags=build_type_release,purego,goscript'],
    packageNames: ['.'],
    mainPackagePath: '',
    bldrDistRoot: 'bldr',
    productionWrapperReport: '',
    skipCompile: false,
    skipBundle: false,
    protobufTypeScriptBinding: true,
    top: 30,
  }

  for (let i = 0; i < raw.length; i += 1) {
    const arg = raw[i]
    const next = raw[i + 1]
    switch (arg) {
      case '--out':
        if (!next) usage()
        opts.outDir = next
        i += 1
        break
      case '--module-dir':
        if (!next) usage()
        opts.moduleDir = next
        i += 1
        break
      case '--from-ts':
        if (!next) usage()
        opts.fromTs = next
        opts.skipCompile = true
        i += 1
        break
      case '--build-flags':
        if (!next) usage()
        opts.buildFlags.push(next)
        i += 1
        break
      case '--package':
        if (!next) usage()
        if (opts.packageNames.length === 1 && opts.packageNames[0] === '.') {
          opts.packageNames = []
        }
        opts.packageNames.push(next)
        i += 1
        break
      case '--main-package-path':
        if (!next) usage()
        opts.mainPackagePath = next
        i += 1
        break
      case '--bldr-dist-root':
        if (!next) usage()
        opts.bldrDistRoot = next
        i += 1
        break
      case '--production-wrapper-report':
        if (!next) usage()
        opts.productionWrapperReport = next
        i += 1
        break
      case '--skip-compile':
        opts.skipCompile = true
        break
      case '--skip-bundle':
        opts.skipBundle = true
        break
      case '--no-protobuf-ts-binding':
        opts.protobufTypeScriptBinding = false
        break
      case '--top':
        if (!next) usage()
        opts.top = Number(next)
        if (!Number.isInteger(opts.top) || opts.top <= 0) usage()
        i += 1
        break
      case '-h':
      case '--help':
        usage(0)
        break
      default:
        usage()
    }
  }

  opts.outDir = resolve(opts.outDir)
  opts.moduleDir = resolve(opts.moduleDir)
  opts.bldrDistRoot = resolve(opts.bldrDistRoot)
  if (opts.productionWrapperReport) {
    opts.productionWrapperReport = resolve(opts.productionWrapperReport)
  }
  opts.tsDir = opts.fromTs ? resolve(opts.fromTs) : join(opts.outDir, 'ts')
  return opts
}

function timestamp(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return [
    d.getFullYear(),
    pad(d.getMonth() + 1),
    pad(d.getDate()),
    '-',
    pad(d.getHours()),
    pad(d.getMinutes()),
    pad(d.getSeconds()),
  ].join('')
}

async function pathExists(path: string): Promise<boolean> {
  try {
    await stat(path)
    return true
  } catch {
    return false
  }
}

async function runCommand(
  command: string,
  args: string[],
  cwd: string,
  logPath: string,
  env: Record<string, string | undefined> = {},
): Promise<CommandResult> {
  await mkdir(dirname(logPath), { recursive: true })
  const log = createWriteStream(logPath, { flags: 'w' })
  log.write(`$ ${command} ${args.map(shellQuote).join(' ')}\n`)

  const proc = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  proc.stdout.pipe(log)
  proc.stderr.pipe(log)

  const exitCode = await new Promise<number>((resolveCode) => {
    proc.on('close', (code, signal) => {
      resolveCode(signal ? 1 : (code ?? 1))
    })
  })
  await new Promise<void>((resolveDone) => log.end(resolveDone))
  if (exitCode !== 0) {
    throw new Error(
      `${command} exited ${exitCode}; inspect ${relative(process.cwd(), logPath)}`,
    )
  }
  return { command, args, logPath }
}

function shellQuote(value: string): string {
  if (/^[A-Za-z0-9_./:=,@+-]+$/.test(value)) return value
  return `'${value.replace(/'/g, `'\\''`)}'`
}

async function goListImportPath(opts: Options): Promise<string> {
  const args = ['list']
  args.push(...opts.buildFlags)
  args.push('-f', '{{.ImportPath}}', '.')
  const proc = spawn('go', args, {
    cwd: opts.moduleDir,
    env: { ...process.env, GOOS: '', GOARCH: '' },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  let stdout = ''
  let stderr = ''
  proc.stdout.on('data', (chunk: Buffer) => {
    stdout += chunk.toString()
  })
  proc.stderr.on('data', (chunk: Buffer) => {
    stderr += chunk.toString()
  })
  const exitCode = await new Promise<number>((resolveCode) => {
    proc.on('close', (code, signal) => {
      resolveCode(signal ? 1 : (code ?? 1))
    })
  })
  if (exitCode !== 0) {
    throw new Error(`go list failed: ${stderr.trim() || stdout.trim()}`)
  }
  const importPath = stdout.trim()
  if (!importPath) throw new Error('go list returned an empty import path')
  return importPath
}

async function compileGoScript(opts: Options): Promise<CommandResult | null> {
  if (opts.skipCompile) return null
  await mkdir(opts.tsDir, { recursive: true })
  const args = ['compile', '--dir', opts.moduleDir, '--output', opts.tsDir]
  for (const pkg of opts.packageNames) {
    args.push('--package', pkg)
  }
  for (const flag of opts.buildFlags) {
    args.push('--build-flags', flag)
  }
  args.push('--all-dependencies')
  if (opts.protobufTypeScriptBinding) {
    args.push('--protobuf-ts-binding')
  }

  const goscript = process.env.BLDR_GOSCRIPT
  if (goscript && goscript.trim() !== '') {
    return await runCommand(
      goscript,
      args,
      opts.moduleDir,
      join(opts.outDir, 'goscript-compile.log'),
    )
  }

  return await runCommand(
    'go',
    ['run', goscriptModule, ...args],
    opts.moduleDir,
    join(opts.outDir, 'goscript-compile.log'),
    { GONOSUMDB: 'github.com/aperturerobotics/goscript' },
  )
}

async function walkFiles(root: string): Promise<FileRecord[]> {
  const out: FileRecord[] = []
  async function walk(dir: string) {
    const entries = await readdir(dir, { withFileTypes: true })
    for (const entry of entries) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) {
        await walk(path)
        continue
      }
      if (!entry.isFile()) continue
      const info = await stat(path)
      out.push({
        path,
        rel: relative(root, path).split('\\').join('/'),
        bytes: info.size,
      })
    }
  }
  await walk(root)
  out.sort((a, b) => a.rel.localeCompare(b.rel))
  return out
}

async function gzipFile(path: string): Promise<string> {
  const data = await readFile(path)
  const outPath = `${path}.gz`
  await writeFile(outPath, gzipSync(data, { level: 9 }))
  return outPath
}

async function createTreeArchive(
  tsDir: string,
  outDir: string,
): Promise<string> {
  const archivePath = join(outDir, 'ts-tree.tar.gz')
  await runCommand(
    'tar',
    ['-czf', archivePath, '-C', tsDir, '.'],
    process.cwd(),
    join(outDir, 'tar-ts-tree.log'),
  )
  return archivePath
}

function miB(bytes: number): number {
  return Number((bytes / 1024 / 1024).toFixed(2))
}

function countMatches(text: string, pattern: RegExp): number {
  return text.match(pattern)?.length ?? 0
}

async function analyzeProtoFiles(files: FileRecord[]) {
  const pbFiles = files.filter((file) => file.rel.endsWith('.pb.gs.ts'))
  const methodDefinition = (name: string) =>
    new RegExp(
      `\\b(?:public\\s+|private\\s+|protected\\s+)?${name}\\s*\\([^)]*\\)\\s*(?::[^{}]+)?{`,
      'g',
    )
  const methodPatterns = {
    CloneVT: methodDefinition('CloneVT'),
    EqualVT: methodDefinition('EqualVT'),
    SizeVT: methodDefinition('SizeVT'),
    MarshalVT: methodDefinition('MarshalVT'),
    MarshalToSizedBufferVT: methodDefinition('MarshalToSizedBufferVT'),
    UnmarshalVT: methodDefinition('UnmarshalVT'),
    MarshalJSON: methodDefinition('MarshalJSON'),
    UnmarshalJSON: methodDefinition('UnmarshalJSON'),
    MarshalProtoJSON: methodDefinition('MarshalProtoJSON'),
    UnmarshalProtoJSON: methodDefinition('UnmarshalProtoJSON'),
    MarshalProtoText: methodDefinition('MarshalProtoText'),
    UnmarshalProtoText: methodDefinition('UnmarshalProtoText'),
    String: methodDefinition('String'),
  }
  const methodCounts = Object.fromEntries(
    Object.keys(methodPatterns).map((name) => [name, 0]),
  ) as Record<string, number>
  let classCount = 0

  for (const file of pbFiles) {
    const text = await readFile(file.path, 'utf8')
    classCount += countMatches(text, /\bclass\s+[A-Za-z_$][\w$]*/g)
    for (const [name, pattern] of Object.entries(methodPatterns)) {
      methodCounts[name] += countMatches(text, pattern)
    }
  }

  return {
    files: pbFiles.length,
    bytes: pbFiles.reduce((sum, file) => sum + file.bytes, 0),
    classCount,
    methodCounts,
    largestFiles: [...pbFiles].sort((a, b) => b.bytes - a.bytes).slice(0, 20),
  }
}

async function writeEntrypoint(opts: Options): Promise<string> {
  const workDir = join(opts.outDir, 'work')
  await mkdir(workDir, { recursive: true })
  const runtimePath = join(
    opts.bldrDistRoot,
    'web',
    'runtime',
    'goscript',
    'plugin-goscript.ts',
  )
  const runtimeImport = relativeImport(workDir, runtimePath)
  const mainImport = `@goscript/${opts.mainPackagePath.replace(/^\/+|\/+$/g, '')}/plugin.gs.js`
  const entrypoint = [
    `import runGoScriptPlugin from ${JSON.stringify(runtimeImport)}`,
    `import { main as pluginMain } from ${JSON.stringify(mainImport)}`,
    '',
    'export default async function main(api) {',
    '  await runGoScriptPlugin(api, pluginMain)',
    '}',
    '',
  ].join('\n')
  const entrypointPath = join(workDir, 'plugin-goscript-entrypoint.ts')
  await writeFile(entrypointPath, entrypoint)
  return entrypointPath
}

function relativeImport(fromDir: string, toPath: string): string {
  let rel = relative(fromDir, toPath).split('\\').join('/')
  if (!rel.startsWith('.')) rel = `./${rel}`
  return rel
}

function goscriptResolver(tsDir: string): Plugin {
  return {
    name: 'goscript-size-report-resolver',
    async resolveId(source, importer) {
      if (source.startsWith('@goscript/')) {
        const rel = source.slice('@goscript/'.length)
        return existingTypeScriptSibling(join(tsDir, '@goscript', rel))
      }
      if (
        importer &&
        (source.startsWith('./') || source.startsWith('../')) &&
        source.endsWith('.js')
      ) {
        return existingTypeScriptSibling(join(dirname(importer), source))
      }
      return null
    },
  }
}

async function existingTypeScriptSibling(path: string): Promise<string | null> {
  const candidates = path.endsWith('.js')
    ? [path.slice(0, -'.js'.length) + '.ts', path]
    : [`${path}.ts`, path]
  for (const candidate of candidates) {
    if (await pathExists(candidate)) return candidate
  }
  return null
}

async function bundleOutputs(opts: Options) {
  if (opts.skipBundle) return null
  const entrypointPath = await writeEntrypoint(opts)
  const bundleDir = join(opts.outDir, 'bundle')
  await mkdir(bundleDir, { recursive: true })
  const dcePath = join(bundleDir, 'spacewave-core.rolldown.dce.mjs')
  const minPath = join(bundleDir, 'spacewave-core.rolldown.min.mjs')
  const bundle = await rolldown({
    input: entrypointPath,
    platform: 'browser',
    treeshake: true,
    logLevel: 'warn',
    plugins: [goscriptResolver(opts.tsDir)],
  })
  try {
    await bundle.write({
      file: dcePath,
      format: 'esm',
      minify: false,
      sourcemap: false,
    })
    await bundle.write({
      file: minPath,
      format: 'esm',
      minify: true,
      sourcemap: false,
    })
  } finally {
    await bundle.close()
  }
  const dceGzipPath = await gzipFile(dcePath)
  const minGzipPath = await gzipFile(minPath)
  return {
    dcePath,
    dceGzipPath,
    minPath,
    minGzipPath,
  }
}

async function fileSize(path: string): Promise<number> {
  return (await stat(path)).size
}

async function readProductionWrapperReport(
  reportPath: string,
): Promise<ProductionWrapperReport> {
  const isNonEmptyString = (value: unknown): value is string =>
    typeof value === 'string' && value.trim() !== ''
  const isPositiveSafeInteger = (value: unknown): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value > 0
  const isNonNegativeSafeInteger = (value: unknown): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
  const parsed = JSON.parse(
    await readFile(reportPath, 'utf8'),
  ) as Partial<ProductionWrapperReport>
  if (parsed.schemaVersion !== 1) {
    throw new Error(
      `unsupported production wrapper report schema: ${parsed.schemaVersion}`,
    )
  }
  if (
    !isNonEmptyString(parsed.outputPath) ||
    !isPositiveSafeInteger(parsed.outputBytes) ||
    !isPositiveSafeInteger(parsed.outputGzipBytes) ||
    typeof parsed.minify !== 'boolean' ||
    typeof parsed.sourcemaps !== 'boolean' ||
    !isNonNegativeSafeInteger(parsed.inputCount) ||
    !Array.isArray(parsed.inputPaths) ||
    parsed.inputCount !== parsed.inputPaths.length ||
    !parsed.inputPaths.every(isNonEmptyString)
  ) {
    throw new Error(`invalid production wrapper report: ${reportPath}`)
  }
  return parsed as ProductionWrapperReport
}

async function renderMarkdown(report: any): Promise<string> {
  const lines: string[] = [
    '# GoScript Spacewave Core Size Report',
    '',
    `Generated: ${report.generatedAt}`,
    `TypeScript tree: \`${report.paths.tsDir}\``,
    '',
    '## Size Summary',
    '',
    '| Artifact | Bytes | MiB |',
    '| --- | ---: | ---: |',
  ]
  for (const row of report.sizeRows) {
    lines.push(`| ${row.name} | ${row.bytes} | ${row.mib} |`)
  }
  lines.push(
    '',
    '## Counts',
    '',
    '| Metric | Count |',
    '| --- | ---: |',
    `| TypeScript files | ${report.counts.tsFiles} |`,
    `| All files | ${report.counts.allFiles} |`,
    `| Protobuf GoScript files | ${report.protobuf.files} |`,
    `| Protobuf GoScript classes | ${report.protobuf.classCount} |`,
    '',
    '## Protobuf Method Counts',
    '',
    '| Method | Count |',
    '| --- | ---: |',
  )
  for (const [method, count] of Object.entries(report.protobuf.methodCounts)) {
    lines.push(`| \`${method}\` | ${count} |`)
  }
  if (report.productionWrapperReport) {
    lines.push(
      '',
      '## Production Wrapper Report',
      '',
      '| Metric | Value |',
      '| --- | --- |',
      `| Report path | \`${report.paths.productionWrapperReport}\` |`,
      `| Output path | \`${report.productionWrapperReport.outputPath}\` |`,
      `| Minify | ${report.productionWrapperReport.minify} |`,
      `| Sourcemaps | ${report.productionWrapperReport.sourcemaps} |`,
      `| Dependency inputs | ${report.productionWrapperReport.inputCount} |`,
    )
  }
  lines.push(
    '',
    '## Largest TypeScript Files',
    '',
    '| Bytes | File |',
    '| ---: | --- |',
  )
  for (const file of report.largestFiles) {
    lines.push(`| ${file.bytes} | \`${file.rel}\` |`)
  }
  lines.push(
    '',
    '## Largest Protobuf GoScript Files',
    '',
    '| Bytes | File |',
    '| ---: | --- |',
  )
  for (const file of report.protobuf.largestFiles) {
    lines.push(`| ${file.bytes} | \`${file.rel}\` |`)
  }
  lines.push('')
  return lines.join('\n')
}

async function main(): Promise<void> {
  const opts = parseArgs()
  await mkdir(opts.outDir, { recursive: true })
  if (!(await pathExists(opts.tsDir)) && opts.skipCompile) {
    throw new Error(`TypeScript tree not found: ${opts.tsDir}`)
  }
  if (!(await pathExists(opts.moduleDir)) && !opts.mainPackagePath) {
    throw new Error(`module directory not found: ${opts.moduleDir}`)
  }
  if (!opts.mainPackagePath) {
    opts.mainPackagePath = await goListImportPath(opts)
  }

  const compile = await compileGoScript(opts)
  const files = await walkFiles(opts.tsDir)
  const tsFiles = files.filter((file) => file.rel.endsWith('.ts'))
  const rawTreeBytes = tsFiles.reduce((sum, file) => sum + file.bytes, 0)
  const protobuf = await analyzeProtoFiles(tsFiles)
  const archivePath = await createTreeArchive(opts.tsDir, opts.outDir)
  const bundle = await bundleOutputs(opts)
  const productionWrapperReport = opts.productionWrapperReport
    ? await readProductionWrapperReport(opts.productionWrapperReport)
    : null

  const sizeRows = [
    {
      name: 'Raw generated .ts tree',
      bytes: rawTreeBytes,
      mib: miB(rawTreeBytes),
    },
    {
      name: 'Raw generated .ts tree gzip tar',
      bytes: await fileSize(archivePath),
      mib: miB(await fileSize(archivePath)),
    },
    {
      name: 'Protobuf .pb.gs.ts subset',
      bytes: protobuf.bytes,
      mib: miB(protobuf.bytes),
    },
  ]
  if (bundle) {
    const dceBytes = await fileSize(bundle.dcePath)
    const dceGzipBytes = await fileSize(bundle.dceGzipPath)
    const minBytes = await fileSize(bundle.minPath)
    const minGzipBytes = await fileSize(bundle.minGzipPath)
    sizeRows.push(
      {
        name: 'Rolldown/Oxc tree-shaken bundle',
        bytes: dceBytes,
        mib: miB(dceBytes),
      },
      {
        name: 'Rolldown/Oxc tree-shaken bundle gzip',
        bytes: dceGzipBytes,
        mib: miB(dceGzipBytes),
      },
      {
        name: 'Rolldown/Oxc tree-shaken minified bundle',
        bytes: minBytes,
        mib: miB(minBytes),
      },
      {
        name: 'Rolldown/Oxc tree-shaken minified bundle gzip',
        bytes: minGzipBytes,
        mib: miB(minGzipBytes),
      },
    )
  }
  if (productionWrapperReport) {
    sizeRows.push(
      {
        name: 'Production Bldr GoScript wrapper bundle',
        bytes: productionWrapperReport.outputBytes,
        mib: miB(productionWrapperReport.outputBytes),
      },
      {
        name: 'Production Bldr GoScript wrapper bundle gzip',
        bytes: productionWrapperReport.outputGzipBytes,
        mib: miB(productionWrapperReport.outputGzipBytes),
      },
    )
  }

  const report = {
    generatedAt: new Date().toISOString(),
    paths: {
      outDir: await realpath(opts.outDir),
      moduleDir: await realpath(opts.moduleDir),
      tsDir: await realpath(opts.tsDir),
      treeArchive: archivePath,
      bundle,
      productionWrapperReport: opts.productionWrapperReport || null,
    },
    compile,
    buildFlags: opts.buildFlags,
    packageNames: opts.packageNames,
    mainPackagePath: opts.mainPackagePath,
    protobufTypeScriptBinding: opts.protobufTypeScriptBinding,
    productionWrapperReport,
    counts: {
      allFiles: files.length,
      tsFiles: tsFiles.length,
    },
    sizeRows,
    protobuf,
    largestFiles: [...tsFiles]
      .sort((a, b) => b.bytes - a.bytes)
      .slice(0, opts.top),
  }
  const jsonPath = join(opts.outDir, 'size-report.json')
  const mdPath = join(opts.outDir, 'size-report.md')
  await writeFile(jsonPath, `${JSON.stringify(report, null, 2)}\n`)
  await writeFile(mdPath, await renderMarkdown(report))
  await writeFile(
    join(dirname(opts.outDir), 'goscript-spacewave-core-size-latest.txt'),
    opts.outDir,
  )

  console.log(`wrote ${relative(process.cwd(), jsonPath)}`)
  console.log(`wrote ${relative(process.cwd(), mdPath)}`)
  for (const row of sizeRows) {
    console.log(`${row.name}: ${row.bytes} bytes (${row.mib} MiB)`)
  }
}

await main()
