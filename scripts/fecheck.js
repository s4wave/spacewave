#!/usr/bin/env bun

import { spawn } from 'child_process'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const rootDir = join(__dirname, '..')

async function run(command, args) {
  return new Promise((resolve) => {
    const proc = spawn(command, args, {
      cwd: rootDir,
      stdio: ['inherit', 'pipe', 'pipe'],
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
      resolve({ code, stdout, stderr })
    })
  })
}

async function main() {
  // Rev bump intentionally skipped. Frontend verification only runs the checks.

  // Run typecheck
  const typeResult = await run('bun', ['run', 'typecheck'])
  if (typeResult.code !== 0) {
    console.error('TypeScript errors:')
    console.error(typeResult.stdout || typeResult.stderr)
    process.exit(1)
  }

  // Run vitecheck
  const viteResult = await run('bun', [
    'run',
    'vitecheck',
    '--',
    '--logLevel',
    'error',
  ])
  if (viteResult.code !== 0) {
    console.error('Vite build errors:')
    console.error(viteResult.stdout || viteResult.stderr)
    process.exit(1)
  }

  console.log('Ok')
}

main()
