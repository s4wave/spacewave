import { closeSync, openSync } from 'node:fs'
import { appendFile, mkdir, rename } from 'node:fs/promises'
import { join, resolve } from 'node:path'

export const goScriptStorageBenchmarkEngines = [
  'chromium',
  'firefox',
  'webkit',
] as const

export type GoScriptStorageBenchmarkEngine =
  (typeof goScriptStorageBenchmarkEngines)[number]

export interface GoScriptStorageBenchmarkArgs {
  runId: string
  outputRoot: string
  chromiumCpuProfile: boolean
  engines: GoScriptStorageBenchmarkEngine[]
}

export interface GoScriptStorageBenchmarkOptions extends GoScriptStorageBenchmarkArgs {
  repoRoot: string
  spacewaveRevision: string
  goScriptRevision: string
}

export interface GoScriptStorageBenchmarkEngineResult {
  engine: GoScriptStorageBenchmarkEngine
  status: 'passed' | 'unsupported' | 'failed'
  exitCode: number
  engineVersion: string
  fixtureSha256: string
  reason: string
  artifactDir: string
  logFile: string
}

export interface GoScriptStorageBenchmarkIndex {
  schemaVersion: 1
  runId: string
  spacewaveRevision: string
  goScriptRevision: string
  fixtureSha256: string
  engines: GoScriptStorageBenchmarkEngineResult[]
}

export interface GoScriptStorageBenchmarkInvocation {
  options: GoScriptStorageBenchmarkOptions
  engine: GoScriptStorageBenchmarkEngine
  artifactDir: string
  logFile: string
}

export type GoScriptStorageBenchmarkEngineRunner = (
  invocation: GoScriptStorageBenchmarkInvocation,
) => Promise<GoScriptStorageBenchmarkEngineResult>

const benchmarkTestName = '^TestGoScriptStorageBenchmark$'
const benchmarkEnabledEnv = 'E2E_GOSCRIPT_STORAGE_BENCH'
const benchmarkRunIdEnv = 'E2E_GOSCRIPT_STORAGE_BENCH_RUN_ID'
const benchmarkOutputRootEnv = 'E2E_GOSCRIPT_STORAGE_BENCH_OUTPUT_ROOT'
const benchmarkSpacewaveRevisionEnv =
  'E2E_GOSCRIPT_STORAGE_BENCH_SPACEWAVE_REVISION'
const benchmarkGoScriptRevisionEnv =
  'E2E_GOSCRIPT_STORAGE_BENCH_GOSCRIPT_REVISION'
const benchmarkCpuProfileEnv = 'E2E_GOSCRIPT_STORAGE_BENCH_CPU_PROFILE'
const benchmarkBrowserEnv = 'E2E_WASM_BROWSER'
const artifactSchemaVersion = 2
const capabilitySchemaVersion = 1

export function parseGoScriptStorageBenchmarkArgs(
  args: string[],
  cwd = process.cwd(),
): GoScriptStorageBenchmarkArgs {
  let runId = ''
  let outputRoot = resolve(cwd, '.tmp/e2e/goscriptbench')
  let chromiumCpuProfile = false
  const engines: GoScriptStorageBenchmarkEngine[] = []

  for (let idx = 0; idx < args.length; idx++) {
    const arg = args[idx]
    switch (arg) {
      case '--run-id': {
        const value = args[++idx]
        if (!value || value.startsWith('--')) {
          throw new Error('--run-id requires a value')
        }
        runId = value
        break
      }
      case '--output-root': {
        const value = args[++idx]
        if (!value || value.startsWith('--')) {
          throw new Error('--output-root requires a value')
        }
        outputRoot = resolve(cwd, value)
        break
      }
      case '--engine': {
        const value = args[++idx]
        if (
          !value ||
          !goScriptStorageBenchmarkEngines.includes(
            value as GoScriptStorageBenchmarkEngine,
          )
        ) {
          throw new Error(
            `--engine must be one of ${goScriptStorageBenchmarkEngines.join(', ')}`,
          )
        }
        const engine = value as GoScriptStorageBenchmarkEngine
        if (engines.includes(engine)) {
          throw new Error(`--engine ${engine} was selected more than once`)
        }
        engines.push(engine)
        break
      }
      case '--chromium-cpu-profile':
        chromiumCpuProfile = true
        break
      default:
        throw new Error(`unknown argument: ${arg}`)
    }
  }

  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(runId)) {
    throw new Error('--run-id must be a safe artifact path component')
  }
  return {
    runId,
    outputRoot,
    chromiumCpuProfile,
    engines:
      engines.length === 0
        ? [...goScriptStorageBenchmarkEngines]
        : goScriptStorageBenchmarkEngines.filter((engine) =>
            engines.includes(engine),
          ),
  }
}

export async function runGoScriptStorageBenchmarkMatrix(
  options: GoScriptStorageBenchmarkOptions,
  runEngine: GoScriptStorageBenchmarkEngineRunner = runGoScriptStorageBenchmarkEngine,
): Promise<GoScriptStorageBenchmarkIndex> {
  const runDir = join(options.outputRoot, options.runId)
  const logDir = join(runDir, 'logs')
  await mkdir(logDir, { recursive: true })

  const engines: GoScriptStorageBenchmarkEngineResult[] = []
  for (const engine of options.engines) {
    const artifactDir = join(runDir, engine)
    const logFile = join(logDir, `${engine}.log`)
    try {
      engines.push(await runEngine({ options, engine, artifactDir, logFile }))
    } catch (error) {
      await appendFile(
        logFile,
        `${error instanceof Error ? (error.stack ?? error.message) : String(error)}\n`,
      )
      engines.push({
        engine,
        status: 'failed',
        exitCode: 1,
        engineVersion: '',
        fixtureSha256: '',
        reason: '',
        artifactDir: engine,
        logFile: `logs/${engine}.log`,
      })
    }
  }

  const fixtureSha256 =
    engines.find((engine) => engine.status === 'passed')?.fixtureSha256 ?? ''
  const index: GoScriptStorageBenchmarkIndex = {
    schemaVersion: 1,
    runId: options.runId,
    spacewaveRevision: options.spacewaveRevision,
    goScriptRevision: options.goScriptRevision,
    fixtureSha256,
    engines,
  }
  validateGoScriptStorageBenchmarkIndex(index)

  const indexPath = join(runDir, 'index.json')
  const tempPath = join(
    runDir,
    `.index-${process.pid}-${Date.now().toString(36)}.json`,
  )
  await Bun.write(tempPath, `${JSON.stringify(index, null, 2)}\n`)
  await rename(tempPath, indexPath)
  return index
}

export function validateGoScriptStorageBenchmarkIndex(
  index: GoScriptStorageBenchmarkIndex,
): void {
  if (index.schemaVersion !== 1) {
    throw new Error(`unsupported matrix schema version ${index.schemaVersion}`)
  }
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(index.runId)) {
    throw new Error('matrix run ID is invalid')
  }
  if (index.spacewaveRevision === '' || index.goScriptRevision === '') {
    throw new Error('matrix source identity is incomplete')
  }
  if (index.engines.length === 0) {
    throw new Error('matrix must contain at least one engine result')
  }

  let priorEngineIndex = -1
  for (const [idx, result] of index.engines.entries()) {
    const engineIndex = goScriptStorageBenchmarkEngines.indexOf(result.engine)
    if (engineIndex <= priorEngineIndex) {
      throw new Error(
        `matrix engine ${idx} is duplicated or outside canonical order`,
      )
    }
    priorEngineIndex = engineIndex
    if (result.status === 'passed') {
      if (
        result.exitCode !== 0 ||
        result.engineVersion === '' ||
        result.fixtureSha256 === '' ||
        result.reason !== ''
      ) {
        throw new Error(`passed ${result.engine} result is incomplete`)
      }
      if (result.fixtureSha256 !== index.fixtureSha256) {
        throw new Error(`${result.engine} fixture differs from the matrix`)
      }
    } else if (result.status === 'unsupported') {
      if (
        result.exitCode !== 0 ||
        result.engineVersion === '' ||
        result.fixtureSha256 !== '' ||
        result.reason === ''
      ) {
        throw new Error(`unsupported ${result.engine} result is incomplete`)
      }
    } else if (result.exitCode === 0) {
      throw new Error(`failed ${result.engine} result has a zero exit code`)
    }
    if (
      result.artifactDir !== result.engine ||
      result.logFile !== `logs/${result.engine}.log`
    ) {
      throw new Error(`${result.engine} result paths leave the run directory`)
    }
  }
}

async function runGoScriptStorageBenchmarkEngine(
  invocation: GoScriptStorageBenchmarkInvocation,
): Promise<GoScriptStorageBenchmarkEngineResult> {
  const { options, engine, artifactDir, logFile } = invocation
  const env = { ...process.env }
  env[benchmarkEnabledEnv] = 'true'
  env[benchmarkRunIdEnv] = options.runId
  env[benchmarkOutputRootEnv] = options.outputRoot
  env[benchmarkSpacewaveRevisionEnv] = options.spacewaveRevision
  env[benchmarkGoScriptRevisionEnv] = options.goScriptRevision
  env[benchmarkBrowserEnv] = engine
  if (options.chromiumCpuProfile && engine === 'chromium') {
    env[benchmarkCpuProfileEnv] = 'true'
  } else {
    delete env[benchmarkCpuProfileEnv]
  }

  const logDescriptor = openSync(logFile, 'w')
  let exitCode = 1
  try {
    const process = Bun.spawn(
      [
        'go',
        'test',
        '-count=1',
        '-v',
        '-timeout=45m',
        '-run',
        benchmarkTestName,
        './e2e/goscriptbench',
      ],
      {
        cwd: options.repoRoot,
        env,
        stdout: logDescriptor,
        stderr: logDescriptor,
      },
    )
    exitCode = await process.exited
  } finally {
    closeSync(logDescriptor)
  }

  const paths = {
    artifactDir: engine,
    logFile: `logs/${engine}.log`,
  }
  if (exitCode !== 0) {
    return {
      engine,
      status: 'failed',
      exitCode,
      engineVersion: '',
      fixtureSha256: '',
      reason: '',
      ...paths,
    }
  }

  const capability = await readPublishedCapability(artifactDir, engine, options)
  if (capability !== undefined) {
    return {
      engine,
      status: 'unsupported',
      exitCode,
      engineVersion: capability.engineVersion,
      fixtureSha256: '',
      reason: capability.reason,
      ...paths,
    }
  }

  const published = await readPublishedEngine(artifactDir, engine, options)
  return {
    engine,
    status: 'passed',
    exitCode,
    engineVersion: published.engineVersion,
    fixtureSha256: published.fixtureSha256,
    reason: '',
    ...paths,
  }
}

async function readPublishedCapability(
  artifactDir: string,
  engine: GoScriptStorageBenchmarkEngine,
  options: GoScriptStorageBenchmarkOptions,
): Promise<{ engineVersion: string; reason: string } | undefined> {
  const file = Bun.file(join(artifactDir, 'capability.json'))
  if (!(await file.exists())) {
    return undefined
  }
  const capability = requireRecord(JSON.parse(await file.text()), 'capability')
  if (
    requireNumber(capability.schemaVersion, 'capability.schemaVersion') !==
      capabilitySchemaVersion ||
    requireString(capability.runId, 'capability.runId') !== options.runId ||
    requireString(capability.engine, 'capability.engine') !== engine ||
    requireString(capability.capability, 'capability.capability') !== 'opfs' ||
    requireString(capability.status, 'capability.status') !== 'unsupported'
  ) {
    throw new Error(`${engine} capability record is invalid`)
  }
  return {
    engineVersion: requireString(
      capability.engineVersion,
      'capability.engineVersion',
    ),
    reason: requireString(capability.reason, 'capability.reason'),
  }
}

async function readPublishedEngine(
  artifactDir: string,
  engine: GoScriptStorageBenchmarkEngine,
  options: GoScriptStorageBenchmarkOptions,
): Promise<{ engineVersion: string; fixtureSha256: string }> {
  const result = requireRecord(
    JSON.parse(await Bun.file(join(artifactDir, 'result.json')).text()),
    'result',
  )
  if (
    requireNumber(result.schemaVersion, 'result.schemaVersion') !==
    artifactSchemaVersion
  ) {
    throw new Error(`${engine} result schema version is unsupported`)
  }
  const metadata = requireRecord(result.metadata, 'result.metadata')
  if (
    requireString(metadata.runId, 'metadata.runId') !== options.runId ||
    requireString(metadata.engine, 'metadata.engine') !== engine ||
    requireString(metadata.spacewaveRevision, 'metadata.spacewaveRevision') !==
      options.spacewaveRevision ||
    requireString(metadata.goScriptRevision, 'metadata.goScriptRevision') !==
      options.goScriptRevision
  ) {
    throw new Error(`${engine} result identity differs from its process`)
  }
  const engineVersion = requireString(
    metadata.engineVersion,
    'metadata.engineVersion',
  )
  const fixture = requireRecord(metadata.fixture, 'metadata.fixture')
  const fixtureSha256 = requireString(fixture.sha256, 'fixture.sha256')
  if (
    requireNumber(fixture.width, 'fixture.width') !== 1024 ||
    requireNumber(fixture.height, 'fixture.height') !== 1024
  ) {
    throw new Error(`${engine} fixture dimensions differ from 1024 by 1024`)
  }

  const sampling = requireRecord(result.sampling, 'result.sampling')
  if (
    requireNumber(sampling.warmupSamples, 'sampling.warmupSamples') !== 1 ||
    requireNumber(sampling.retainedSamples, 'sampling.retainedSamples') !==
      10 ||
    requireNumber(sampling.diagnosticSamples, 'sampling.diagnosticSamples') !==
      1
  ) {
    throw new Error(
      `${engine} sampling policy differs from the fixed population`,
    )
  }
  const warmup = requireRecord(result.warmup, 'result.warmup')
  if (requireBoolean(warmup.traced, 'warmup.traced')) {
    throw new Error(`${engine} warm-up is traced`)
  }
  const samples = requireArray(result.samples, 'result.samples')
  if (samples.length !== 10) {
    throw new Error(
      `${engine} retained row count is ${samples.length}, expected 10`,
    )
  }
  for (const [idx, sampleValue] of samples.entries()) {
    const sample = requireRecord(sampleValue, `samples[${idx}]`)
    if (requireBoolean(sample.traced, `samples[${idx}].traced`)) {
      throw new Error(`${engine} retained sample ${idx + 1} is traced`)
    }
  }

  const diagnostic = requireRecord(
    JSON.parse(await Bun.file(join(artifactDir, 'diagnostic.json')).text()),
    'diagnostic',
  )
  const diagnosticSample = requireRecord(diagnostic.sample, 'diagnostic.sample')
  if (!requireBoolean(diagnosticSample.traced, 'diagnostic.sample.traced')) {
    throw new Error(`${engine} diagnostic sample is untraced`)
  }
  if (
    requireString(
      diagnostic.runtimeTraceFile,
      'diagnostic.runtimeTraceFile',
    ) !== 'runtime.trace'
  ) {
    throw new Error(`${engine} diagnostic trace path is invalid`)
  }
  if (!(await Bun.file(join(artifactDir, 'runtime.trace')).exists())) {
    throw new Error(`${engine} runtime trace is missing`)
  }
  if (
    options.chromiumCpuProfile &&
    engine === 'chromium' &&
    !(await Bun.file(join(artifactDir, 'browser.cpuprofile')).exists())
  ) {
    throw new Error('Chromium CPU profile is missing')
  }
  return { engineVersion, fixtureSha256 }
}

function requireRecord(value: unknown, field: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`${field} must be an object`)
  }
  return value as Record<string, unknown>
}

function requireArray(value: unknown, field: string): unknown[] {
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be an array`)
  }
  return value
}

function requireString(value: unknown, field: string): string {
  if (typeof value !== 'string' || value === '') {
    throw new Error(`${field} must be a non-empty string`)
  }
  return value
}

function requireNumber(value: unknown, field: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`${field} must be a finite number`)
  }
  return value
}

function requireBoolean(value: unknown, field: string): boolean {
  if (typeof value !== 'boolean') {
    throw new Error(`${field} must be a boolean`)
  }
  return value
}

async function commandOutput(command: string[], cwd: string): Promise<string> {
  const process = Bun.spawn(command, {
    cwd,
    stdout: 'pipe',
    stderr: 'pipe',
  })
  const [exitCode, stdout, stderr] = await Promise.all([
    process.exited,
    new Response(process.stdout).text(),
    new Response(process.stderr).text(),
  ])
  if (exitCode !== 0) {
    throw new Error(`${command.join(' ')} failed: ${stderr.trim()}`)
  }
  const output = stdout.trim()
  if (output === '') {
    throw new Error(`${command.join(' ')} returned empty output`)
  }
  return output
}

async function main(): Promise<number> {
  const repoRoot = resolve(import.meta.dir, '..')
  const args = parseGoScriptStorageBenchmarkArgs(
    process.argv.slice(2),
    repoRoot,
  )
  const [spacewaveRevision, goScriptRevision] = await Promise.all([
    commandOutput(['git', 'rev-parse', 'HEAD'], repoRoot),
    commandOutput(
      ['go', 'list', '-m', '-f', '{{.Version}}', 'github.com/s4wave/goscript'],
      repoRoot,
    ),
  ])
  const index = await runGoScriptStorageBenchmarkMatrix({
    ...args,
    repoRoot,
    spacewaveRevision,
    goScriptRevision,
  })
  return index.engines.some((engine) => engine.status === 'failed') ? 1 : 0
}

if (import.meta.main) {
  try {
    process.exit(await main())
  } catch (error) {
    console.error(
      error instanceof Error ? (error.stack ?? error.message) : error,
    )
    process.exit(1)
  }
}
