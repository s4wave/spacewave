#!/usr/bin/env bun

// testcheck.js - Run all tests with abbreviated output unless something fails.
// Use this instead of `bun run test` to save LLM context.

import { spawn } from 'child_process'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const rootDir = join(__dirname, '..')

async function run(command, args, name) {
  return new Promise((resolve) => {
    const proc = spawn(command, args, {
      cwd: rootDir,
      stdio: ['inherit', 'pipe', 'pipe'],
      env: { ...process.env, FORCE_COLOR: '0' },
    })

    let stdout = ''
    let stderr = ''

    proc.stdout.on('data', (data) => {
      stdout += data.toString()
    })

    proc.stderr.on('data', (data) => {
      stderr += data.toString()
    })

    proc.on('close', (code) => {
      resolve({ code, stdout, stderr, name })
    })
  })
}

// Extract summary line from vitest output
function extractVitestSummary(output) {
  const lines = output.split('\n')
  for (const line of lines) {
    // Match lines like "Test Files  18 passed (18)" or "Tests  244 passed | 4 skipped (248)"
    if (
      line.includes('Test Files') &&
      (line.includes('passed') || line.includes('failed'))
    ) {
      // Strip ANSI codes
      return line.replace(/\x1b\[[0-9;]*m/g, '').trim()
    }
  }
  return null
}

// Extract test count from vitest output
function extractTestCount(output) {
  const lines = output.split('\n')
  for (const line of lines) {
    if (line.includes('Tests') && !line.includes('Test Files')) {
      return line.replace(/\x1b\[[0-9;]*m/g, '').trim()
    }
  }
  return null
}

// Check if Go tests passed
function extractGoSummary(output) {
  const lines = output.split('\n')
  let passed = 0
  let failed = 0
  for (const line of lines) {
    if (line.includes('--- PASS:')) passed++
    if (line.includes('--- FAIL:')) failed++
    if (line.includes('PASS') && !line.includes('---')) {
      // Final PASS line
      if (failed === 0) return `Go tests passed`
    }
    if (line.includes('FAIL') && !line.includes('---')) {
      return null // Will show full output
    }
  }
  if (passed > 0 && failed === 0) return `Go tests passed (${passed} tests)`
  return null
}

async function main() {
  const results = []
  let hasFailure = false

  // Run JS unit tests
  process.stdout.write('Running JS unit tests... ')
  const jsResult = await run('npx', ['vitest', 'run'], 'test:js')
  if (jsResult.code !== 0) {
    console.log('FAILED')
    hasFailure = true
  } else {
    const summary = extractVitestSummary(jsResult.stdout) || 'passed'
    const tests = extractTestCount(jsResult.stdout)
    console.log(tests ? `${summary}, ${tests}` : summary)
  }
  results.push(jsResult)

  // Run typecheck
  process.stdout.write('Running typecheck... ')
  const typeResult = await run(
    'npm',
    ['run', 'typecheck', '--silent'],
    'typecheck',
  )
  if (typeResult.code !== 0) {
    console.log('FAILED')
    hasFailure = true
  } else {
    console.log('passed')
  }
  results.push(typeResult)

  // Run browser E2E tests
  process.stdout.write('Running browser E2E tests... ')
  const browserResult = await run(
    'npx',
    ['vitest', '--config=vitest.browser.config.ts', '--run'],
    'test:browser',
  )
  if (browserResult.code !== 0) {
    console.log('FAILED')
    hasFailure = true
  } else {
    const summary = extractVitestSummary(browserResult.stdout) || 'passed'
    const tests = extractTestCount(browserResult.stdout)
    console.log(tests ? `${summary}, ${tests}` : summary)
  }
  results.push(browserResult)

  // Run Go tests (skip slow e2e tests that do full plugin compilation)
  process.stdout.write('Running Go tests... ')
  const goResult = await run(
    'go',
    [
      'test',
      '-v',
      '-count=1',
      '-skip',
      'TestSpacewaveCoreE2E|TestBrowserE2EWithBldr',
      './...',
    ],
    'test:go',
  )
  if (goResult.code !== 0) {
    console.log('FAILED')
    hasFailure = true
  } else {
    const summary =
      extractGoSummary(goResult.stdout + goResult.stderr) || 'passed'
    console.log(summary)
  }
  results.push(goResult)

  // If any failed, print full output
  if (hasFailure) {
    console.log('\n--- FAILURES ---\n')
    for (const result of results) {
      if (result.code !== 0) {
        console.log(`=== ${result.name} ===`)
        if (result.stdout) console.log(result.stdout)
        if (result.stderr) console.log(result.stderr)
        console.log('')
      }
    }
    process.exit(1)
  }

  console.log('\nAll tests passed.')
}

main()
