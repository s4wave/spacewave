#!/usr/bin/env bun

import { spawn } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const REACT_DIAGNOSTIC_RUN_SCHEMA_VERSION = 1
export const REACT_DIAGNOSTIC_RUN_KIND = 'spacewave.react-diagnostic-run'

const reactDoctorVersion = '0.1.4'
const reactDoctorBinPath = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '..',
  'node_modules',
  'react-doctor',
  'bin',
  'react-doctor.js',
)

const reactDoctorValueFlags = new Set([
  '--project',
  '--fail-on',
  '--explain',
  '--why',
])

const reactDoctorOptionalValueFlags = new Set(['--diff'])

export interface ReactDiagnosticRunCliOptions {
  directory: string
  outputPath: string | null
  offline: boolean
  compact: boolean
  fullAudit: boolean
  passthroughArgs: string[]
}

interface ProcessResult {
  code: number
  signal: NodeJS.Signals | null
  stdout: string
  stderr: string
}

interface ReactDoctorJsonDiagnostic {
  filePath?: string
  plugin?: string
  rule?: string
  severity?: 'error' | 'warning'
  message?: string
  help?: string
  line?: number
  column?: number
  category?: string
}

interface ReactDoctorJsonSummary {
  errorCount?: number
  warningCount?: number
  affectedFileCount?: number
  totalDiagnosticCount?: number
  score?: number | null
  scoreLabel?: string | null
}

interface ReactDoctorJsonReport {
  schemaVersion: number
  ok: boolean
  directory: string
  mode: string
  diagnostics?: ReactDoctorJsonDiagnostic[]
  summary?: ReactDoctorJsonSummary
  error: unknown
  [key: string]: unknown
}

export interface ReactDiagnosticRunReport {
  schemaVersion: typeof REACT_DIAGNOSTIC_RUN_SCHEMA_VERSION
  kind: typeof REACT_DIAGNOSTIC_RUN_KIND
  generatedAt: string
  ok: boolean
  directory: string
  tool: {
    name: 'react-doctor'
    version: string
    offline: boolean
    exitCode: number
    signal: NodeJS.Signals | null
    args: string[]
  }
  reactDoctor: ReactDoctorJsonReport | null
  error: {
    message: string
    stderr: string
  } | null
}

export function parseReactDiagnosticRunArgs(
  args: string[],
): ReactDiagnosticRunCliOptions {
  const passthroughArgs: string[] = []
  let directory = '.'
  let didSetDirectory = false
  let outputPath: string | null = null
  let offline = true
  let compact = false
  let fullAudit = false

  for (let i = 0; i < args.length; i++) {
    const arg = args[i]
    if (arg === '--') {
      passthroughArgs.push(...args.slice(i + 1))
      break
    }
    if (arg === '--output' || arg === '-o') {
      const next = args[i + 1]
      if (!next) throw new Error(`${arg} requires a path`)
      outputPath = next
      i++
      continue
    }
    if (arg === '--compact') {
      compact = true
      continue
    }
    if (arg === '--full-audit') {
      fullAudit = true
      continue
    }
    if (arg === '--offline') {
      offline = true
      continue
    }
    if (arg === '--online') {
      offline = false
      continue
    }
    if (arg.startsWith('-')) {
      passthroughArgs.push(arg)
      if (
        (reactDoctorValueFlags.has(arg) ||
          reactDoctorOptionalValueFlags.has(arg)) &&
        args[i + 1] &&
        !args[i + 1].startsWith('-')
      ) {
        passthroughArgs.push(args[i + 1])
        i++
      }
      continue
    }
    if (!didSetDirectory) {
      directory = arg
      didSetDirectory = true
      continue
    }
    passthroughArgs.push(arg)
  }

  return { directory, outputPath, offline, compact, fullAudit, passthroughArgs }
}

function hasLongFlag(args: string[], flag: string): boolean {
  return args.some((arg) => arg === flag || arg.startsWith(`${flag}=`))
}

function hasDeadCodeFlag(args: string[]): boolean {
  return args.some((arg) => arg === '--dead-code' || arg === '--no-dead-code')
}

export function buildReactDoctorArgs(
  options: ReactDiagnosticRunCliOptions,
): string[] {
  const args = [options.directory, '--json']
  if (options.offline && !hasLongFlag(options.passthroughArgs, '--offline')) {
    args.push('--offline')
  }
  if (options.fullAudit && !hasLongFlag(options.passthroughArgs, '--full')) {
    args.push('--full')
  }
  if (options.fullAudit && !hasDeadCodeFlag(options.passthroughArgs)) {
    args.push('--dead-code')
  }
  if (!options.fullAudit && !hasDeadCodeFlag(options.passthroughArgs)) {
    args.push('--no-dead-code')
  }
  if (!hasLongFlag(options.passthroughArgs, '--fail-on')) {
    args.push('--fail-on', 'none')
  }
  args.push(...options.passthroughArgs)
  return args
}

function spawnReactDoctor(args: string[]): Promise<ProcessResult> {
  return new Promise((resolveProcess) => {
    const proc = spawn('node', [reactDoctorBinPath, ...args], {
      cwd: process.cwd(),
      env: { ...process.env, FORCE_COLOR: '0' },
      stdio: ['inherit', 'pipe', 'pipe'],
    })

    let stdout = ''
    let stderr = ''
    proc.stdout.on('data', (data: Buffer) => {
      stdout += data.toString()
    })
    proc.stderr.on('data', (data: Buffer) => {
      stderr += data.toString()
    })
    proc.on('close', (code, signal) => {
      resolveProcess({ code: code ?? 1, signal, stdout, stderr })
    })
  })
}

function parseReactDoctorReport(stdout: string): ReactDoctorJsonReport {
  const report = JSON.parse(stdout) as unknown
  if (typeof report !== 'object' || report === null) {
    throw new Error('React Doctor did not emit a JSON object')
  }
  return report as ReactDoctorJsonReport
}

export function buildReactDiagnosticRunReport(input: {
  directory: string
  offline: boolean
  args: string[]
  processResult: ProcessResult
  reactDoctor: ReactDoctorJsonReport | null
  parseError: Error | null
  generatedAt?: string
}): ReactDiagnosticRunReport {
  const errorMessage =
    input.parseError?.message ??
    (input.processResult.signal
      ? `React Doctor exited with signal ${input.processResult.signal}`
      : input.processResult.code === 0
        ? null
        : `React Doctor exited with code ${input.processResult.code}`)
  const reactDoctorOk = input.reactDoctor?.ok ?? false
  const ok = input.processResult.code === 0 && !input.parseError && reactDoctorOk

  return {
    schemaVersion: REACT_DIAGNOSTIC_RUN_SCHEMA_VERSION,
    kind: REACT_DIAGNOSTIC_RUN_KIND,
    generatedAt: input.generatedAt ?? new Date().toISOString(),
    ok,
    directory: resolve(input.directory),
    tool: {
      name: 'react-doctor',
      version: reactDoctorVersion,
      offline: input.offline,
      exitCode: input.processResult.code,
      signal: input.processResult.signal,
      args: input.args,
    },
    reactDoctor: input.reactDoctor,
    error: errorMessage
      ? {
          message: errorMessage,
          stderr: input.processResult.stderr,
        }
      : null,
  }
}

function pluralize(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`
}

function count(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  return value
}

function formatScore(summary: ReactDoctorJsonSummary | undefined): string {
  if (typeof summary?.score !== 'number') return 'score unavailable'
  return `${summary.score}/100 ${summary.scoreLabel ?? ''}`.trim()
}

function getDiagnosticSeverity(
  diagnostic: ReactDoctorJsonDiagnostic,
): 'error' | 'warning' {
  if (diagnostic.severity === 'error') return 'error'
  return 'warning'
}

function formatDiagnosticRule(diagnostic: ReactDoctorJsonDiagnostic): string {
  return `${diagnostic.plugin ?? 'react-doctor'}/${diagnostic.rule ?? 'unknown'}`
}

function formatDiagnosticSite(diagnostic: ReactDoctorJsonDiagnostic): string {
  const filePath = diagnostic.filePath ?? '(unknown file)'
  if (!diagnostic.line || diagnostic.line <= 0) return filePath
  return `${filePath}:${diagnostic.line}`
}

function formatTopDiagnosticGroups(
  diagnostics: ReactDoctorJsonDiagnostic[],
): string[] {
  const groups = new Map<string, ReactDoctorJsonDiagnostic[]>()
  for (const diagnostic of diagnostics) {
    const key = formatDiagnosticRule(diagnostic)
    const group = groups.get(key)
    if (group) group.push(diagnostic)
    else groups.set(key, [diagnostic])
  }

  return [...groups.entries()]
    .sort(([, diagnosticsA], [, diagnosticsB]) => {
      const severityA = getDiagnosticSeverity(diagnosticsA[0])
      const severityB = getDiagnosticSeverity(diagnosticsB[0])
      const severityDelta =
        severityA === severityB
          ? 0
          : severityA === 'error'
            ? -1
            : 1
      if (severityDelta !== 0) return severityDelta
      return diagnosticsB.length - diagnosticsA.length
    })
    .slice(0, 5)
    .flatMap(([rule, ruleDiagnostics]) => {
      const first = ruleDiagnostics[0]
      const severity = getDiagnosticSeverity(first)
      return [
        `- ${severity} ${rule} (${pluralize(ruleDiagnostics.length, 'site')}, ${first.category ?? 'Uncategorized'})`,
        `  ${first.message ?? 'No diagnostic message.'}`,
        `  ${formatDiagnosticSite(first)}`,
      ]
    })
}

export function formatReactDiagnosticHumanSummary(
  report: ReactDiagnosticRunReport,
  outputPath: string,
): string {
  const summary = report.reactDoctor?.summary
  const diagnostics = Array.isArray(report.reactDoctor?.diagnostics)
    ? report.reactDoctor.diagnostics
    : []
  const totalDiagnostics =
    count(summary?.totalDiagnosticCount) || diagnostics.length
  const statusLabel = report.error
    ? 'failed'
    : totalDiagnostics > 0
      ? 'completed with diagnostics'
      : 'completed with no diagnostics'
  const lines = [
    `React Doctor ${statusLabel}`,
    `Report: ${resolve(outputPath)}`,
    `Mode: ${report.reactDoctor?.mode ?? 'unknown'} (${report.tool.offline ? 'offline' : 'online'})`,
  ]

  if (report.reactDoctor) {
    const issueSummary = [
      pluralize(count(summary?.errorCount), 'error'),
      pluralize(count(summary?.warningCount), 'warning'),
      pluralize(count(summary?.affectedFileCount), 'affected file'),
      formatScore(summary),
    ].join(', ')
    lines.push(`Summary: ${issueSummary}`)

    if (diagnostics.length > 0) {
      lines.push('', 'Top diagnostics:')
      lines.push(...formatTopDiagnosticGroups(diagnostics))
    }
  }

  if (report.error) {
    lines.push('', `Error: ${report.error.message}`)
    if (report.error.stderr.trim()) {
      lines.push(report.error.stderr.trim())
    }
  }

  lines.push('', 'Machine contract: schema-versioned JSON report.')
  return `${lines.join('\n')}\n`
}

async function writeReport(
  report: ReactDiagnosticRunReport,
  outputPath: string | null,
  compact: boolean,
): Promise<void> {
  const serialized = compact
    ? JSON.stringify(report)
    : JSON.stringify(report, null, 2)
  if (!outputPath) {
    process.stdout.write(`${serialized}\n`)
    return
  }
  await mkdir(dirname(resolve(outputPath)), { recursive: true })
  await writeFile(outputPath, `${serialized}\n`)
  process.stderr.write(formatReactDiagnosticHumanSummary(report, outputPath))
}

export async function runReactDiagnosticRun(args: string[]): Promise<number> {
  const options = parseReactDiagnosticRunArgs(args)
  const reactDoctorArgs = buildReactDoctorArgs(options)
  const processResult = await spawnReactDoctor(reactDoctorArgs)
  let reactDoctor: ReactDoctorJsonReport | null = null
  let parseError: Error | null = null
  try {
    reactDoctor = parseReactDoctorReport(processResult.stdout)
  } catch (error) {
    parseError = error instanceof Error ? error : new Error(String(error))
  }
  const report = buildReactDiagnosticRunReport({
    directory: options.directory,
    offline: options.offline,
    args: reactDoctorArgs,
    processResult,
    reactDoctor,
    parseError,
  })
  await writeReport(report, options.outputPath, options.compact)
  return report.ok ? 0 : 1
}

const entrypointPath = process.argv[1] ? resolve(process.argv[1]) : ''
if (entrypointPath === fileURLToPath(import.meta.url)) {
  runReactDiagnosticRun(process.argv.slice(2)).then(
    (code) => {
      process.exitCode = code
    },
    (error: unknown) => {
      process.stderr.write(
        `${error instanceof Error ? error.message : String(error)}\n`,
      )
      process.exitCode = 1
    },
  )
}
