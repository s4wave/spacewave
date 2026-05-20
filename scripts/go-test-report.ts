#!/usr/bin/env bun

import { spawn } from 'child_process'
import { appendFileSync } from 'fs'

interface GoTestEvent {
  Action?: string
  Package?: string
  Test?: string
  Elapsed?: number
  Output?: string
}

interface TestRecord {
  pkg: string
  test: string
  status: string
  elapsed: number
}

interface PackageRecord {
  pkg: string
  status: string
  elapsed: number
}

function usage(): never {
  console.error(
    'usage: bun scripts/go-test-report.ts [--title <title>] [--] <go test -json command>',
  )
  process.exit(2)
}

function parseArgs(): { title: string; command: string; args: string[] } {
  const rawArgs = process.argv.slice(2)
  let title = process.env.GO_TEST_REPORT_TITLE || 'Go test duration report'
  const commandArgs: string[] = []

  for (let i = 0; i < rawArgs.length; i += 1) {
    const arg = rawArgs[i]
    if (arg === '--') {
      commandArgs.push(...rawArgs.slice(i + 1))
      break
    }
    if (arg === '--title') {
      const next = rawArgs[i + 1]
      if (!next) usage()
      title = next
      i += 1
      continue
    }
    commandArgs.push(...rawArgs.slice(i))
    break
  }

  const [command, ...args] = commandArgs
  if (!command) usage()
  return { title, command, args }
}

function escapeMarkdown(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/\|/g, '\\|')
}

function formatElapsed(seconds: number): string {
  if (seconds < 1) {
    return `${Math.round(seconds * 1000)}ms`
  }
  return `${seconds.toFixed(3)}s`
}

function renderReport(
  title: string,
  tests: TestRecord[],
  packages: PackageRecord[],
): string {
  const sortedTests = [...tests].sort((a, b) => b.elapsed - a.elapsed)
  const sortedPackages = [...packages].sort((a, b) => b.elapsed - a.elapsed)
  const lines: string[] = [`### ${escapeMarkdown(title)}`, '']

  if (sortedPackages.length !== 0) {
    lines.push('| Package | Status | Duration |')
    lines.push('| --- | --- | ---: |')
    for (const record of sortedPackages) {
      lines.push(
        `| \`${escapeMarkdown(record.pkg)}\` | ${escapeMarkdown(record.status.toUpperCase())} | ${formatElapsed(record.elapsed)} |`,
      )
    }
    lines.push('')
  }

  if (sortedTests.length === 0) {
    lines.push('No per-test duration events were emitted.')
    lines.push('')
    return lines.join('\n')
  }

  lines.push('| Test | Status | Duration |')
  lines.push('| --- | --- | ---: |')
  for (const record of sortedTests) {
    lines.push(
      `| \`${escapeMarkdown(record.test)}\` | ${escapeMarkdown(record.status.toUpperCase())} | ${formatElapsed(record.elapsed)} |`,
    )
  }
  lines.push('')
  return lines.join('\n')
}

async function main(): Promise<number> {
  const { title, command, args } = parseArgs()
  const tests = new Map<string, TestRecord>()
  const packages = new Map<string, PackageRecord>()
  let stdoutBuffer = ''

  const proc = spawn(command, args, {
    stdio: ['inherit', 'pipe', 'inherit'],
    env: { ...process.env, FORCE_COLOR: '0' },
  })

  function handleLine(line: string) {
    if (line.trim() === '') return

    let event: GoTestEvent
    try {
      event = JSON.parse(line) as GoTestEvent
    } catch {
      process.stdout.write(`${line}\n`)
      return
    }

    if (event.Output) {
      process.stdout.write(event.Output)
    }

    const action = event.Action || ''
    if (!['pass', 'fail', 'skip'].includes(action)) return

    const elapsed = event.Elapsed
    if (typeof elapsed !== 'number') return

    const pkg = event.Package || ''
    if (event.Test) {
      tests.set(`${pkg}\t${event.Test}`, {
        pkg,
        test: event.Test,
        status: action,
        elapsed,
      })
      return
    }

    if (pkg) {
      packages.set(pkg, {
        pkg,
        status: action,
        elapsed,
      })
    }
  }

  proc.stdout.on('data', (chunk: Buffer) => {
    stdoutBuffer += chunk.toString()
    for (;;) {
      const newline = stdoutBuffer.indexOf('\n')
      if (newline === -1) break
      const line = stdoutBuffer.slice(0, newline)
      stdoutBuffer = stdoutBuffer.slice(newline + 1)
      handleLine(line)
    }
  })

  const exitCode = await new Promise<number>((resolve) => {
    proc.on('close', (code, signal) => {
      if (stdoutBuffer !== '') {
        handleLine(stdoutBuffer)
        stdoutBuffer = ''
      }
      resolve(signal ? 1 : (code ?? 1))
    })
  })

  const report = renderReport(
    title,
    [...tests.values()],
    [...packages.values()],
  )
  process.stdout.write(`\n${report}`)
  const summaryPath = process.env.GITHUB_STEP_SUMMARY
  if (summaryPath) {
    appendFileSync(summaryPath, `\n${report}`)
  }

  return exitCode
}

process.exit(await main())
