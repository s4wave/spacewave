#!/usr/bin/env bun

import { spawn } from 'child_process'

function run(command, args) {
  return new Promise((resolve) => {
    const proc = spawn(command, args, {
      stdio: 'inherit',
      env: process.env,
    })
    proc.on('close', (code, signal) => {
      if (signal) {
        resolve(1)
        return
      }
      resolve(code ?? 1)
    })
  })
}

async function main() {
  const args = process.argv.slice(2)
  const changed = process.env.CI_BASE
    ? ['--changed', process.env.CI_BASE, '--passWithNoTests']
    : []
  const vitestArgs = ['vitest', 'run', ...changed, ...args]
  const vitestCode = await run('bunx', vitestArgs)
  if (vitestCode !== 0) {
    process.exit(vitestCode)
  }
  if (args.length > 0) {
    return
  }
  const typecheckCode = await run('bun', ['run', 'typecheck'])
  if (typecheckCode !== 0) {
    process.exit(typecheckCode)
  }
}

await main()
