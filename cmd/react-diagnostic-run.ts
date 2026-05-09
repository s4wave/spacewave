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
  passthroughArgs: string[]
}

interface ProcessResult {
  code: number
  signal: NodeJS.Signals | null
  stdout: string
  stderr: string
}

interface ReactDoctorJsonReport {
  schemaVersion: number
  ok: boolean
  directory: string
  mode: string
  diagnostics: unknown[]
  summary: unknown
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

  return { directory, outputPath, offline, compact, passthroughArgs }
}

function hasLongFlag(args: string[], flag: string): boolean {
  return args.some((arg) => arg === flag || arg.startsWith(`${flag}=`))
}

export function buildReactDoctorArgs(
  options: ReactDiagnosticRunCliOptions,
): string[] {
  const args = [options.directory, '--json']
  if (options.offline && !hasLongFlag(options.passthroughArgs, '--offline')) {
    args.push('--offline')
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
