#!/usr/bin/env bun

// check-tla keeps the shared model and chart checks, using the pinned TLC
// checker for the large counting and ledger state spaces instead of tla-ts.
import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { once } from 'node:events'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { Readable, Transform, Writable } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { checkModel, findModels, type Model, viewOf } from '@aptre/common/tla'

// tlcDigest pins the official v1.7.4 release, which contains TLC 2.19.
const tlcDigest =
  '936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88'
const tlcURL =
  'https://github.com/tlaplus/tlaplus/releases/download/v1.7.4/tla2tools.jar'
const outputRoot = path.resolve('.tmp/tla-ci')

// intervalGroups isolate the three canonical searches without duplicating
// discovery. Any other configuration stays in the remaining-model group.
const intervalGroups = new Map([
  ['base', 'db/block/gc/IntervalCounts.cfg'],
  ['discard', 'db/block/gc/IntervalCounts-discard.cfg'],
  ['overflow', 'db/block/gc/IntervalCounts-overflow.cfg'],
])

// groupOf assigns every discovered configuration to exactly one CI job.
function groupOf(model: Model): string {
  for (const [group, config] of intervalGroups)
    if (path.resolve(model.config) === path.resolve(config)) return group
  return 'other'
}

// digest identifies the exact checker, model or configuration bytes.
function digest(data: Uint8Array): string {
  return createHash('sha256').update(data).digest('hex')
}

// checkerJar downloads the pinned checker once and verifies even cached bytes.
async function checkerJar(): Promise<string> {
  // Keep tool downloads apart from each model's disposable state directory.
  fs.mkdirSync(outputRoot, { recursive: true })
  const jar = path.join(outputRoot, 'tla2tools-v1.7.4.jar')
  if (!fs.existsSync(jar)) {
    const response = await fetch(tlcURL)
    if (!response.ok) throw new Error(`TLC download: HTTP ${response.status}`)
    const bytes = new Uint8Array(await response.arrayBuffer())
    if (digest(bytes) !== tlcDigest)
      throw new Error('TLC download hash mismatch')
    fs.writeFileSync(jar, bytes)
  }

  // An altered cache must never select another checker silently.
  if (digest(fs.readFileSync(jar)) !== tlcDigest)
    throw new Error(`TLC cache hash mismatch: ${jar}`)
  return jar
}

// forward streams checker output to its retained log and the live job output.
function forward(
  source: Readable,
  destination: Writable,
  log: number,
): Promise<void> {
  const tee = new Transform({
    transform(chunk, _encoding, callback) {
      try {
        fs.writeSync(log, chunk)
        callback(null, chunk)
      } catch (error) {
        callback(error instanceof Error ? error : new Error(String(error)))
      }
    },
  })
  return pipeline(source, tee, destination, { end: false })
}

// checkTLC uses the complete configuration, including deadlock and liveness
// checking. It sets no safety-only, deadlock-suppression or state-bound flag.
async function checkTLC(model: Model): Promise<void> {
  // Isolate each search and retain the exact inputs' identities with its log.
  const jar = await checkerJar()
  const run = fs.mkdtempSync(path.join(outputRoot, 'model-'))
  const states = path.join(run, 'states')
  const log = fs.openSync(path.join(run, 'tlc.log'), 'wx')
  const config = path.resolve(model.config)
  const spec = path.resolve(model.spec)
  console.log(
    `TLC jar=${tlcDigest} spec=${digest(fs.readFileSync(spec))} config=${digest(fs.readFileSync(config))} output=${run}`,
  )

  // One bounded JVM runs at a time. Ephemeral CI searches restart from their
  // inputs, so checkpoints are disabled rather than kept as resume state.
  const child = spawn(
    'java',
    [
      '-XX:ActiveProcessorCount=1',
      '-Xmx2g',
      '-jar',
      jar,
      '-workers',
      '1',
      '-checkpoint',
      '0',
      '-fp',
      '56',
      '-seed',
      '20261103',
      '-metadir',
      states,
      '-config',
      config,
      spec,
    ],
    { cwd: path.dirname(spec), stdio: ['ignore', 'pipe', 'pipe'] },
  )
  const exited = once(child, 'close')
  const output = [
    forward(child.stdout, process.stdout, log),
    forward(child.stderr, process.stderr, log),
  ]
  try {
    const [[code, signal]] = await Promise.all([exited, Promise.all(output)])
    if (code !== 0) throw new Error(`TLC exited ${code}, signal ${signal}`)
    fs.rmSync(states, { recursive: true, force: true })
  } finally {
    // A failed log sink must not leave its checker consuming the build host.
    if (child.exitCode === null && child.signalCode === null) child.kill()
    await Promise.allSettled([exited, ...output])
    fs.closeSync(log)
  }
}

// main preserves the shared runner's discovery, chart commands and diagnostics.
async function main(args: string[]): Promise<number> {
  // Chart generation and help remain the shared CLI's own behavior.
  if (['chart', '-h', '--help'].includes(args[0] ?? '')) {
    const child = spawn('aptre-tla', args, { stdio: 'inherit' })
    const [code] = await once(child, 'close')
    return code ?? 1
  }

  // CI expands only groups present in the same shared inventory it checks.
  if (args[0] === '--groups-json') {
    const groups = new Set<string>()
    for (const target of args.length > 1 ? args.slice(1) : ['.'])
      for (const model of findModels(target)) groups.add(groupOf(model))
    if (groups.size === 0) {
      console.error('FAIL no model configurations selected')
      return 1
    }
    console.log(JSON.stringify([...groups].sort()))
    return 0
  }

  // Grouping filters the shared inventory; ungrouped calls still check it all.
  let group: string | undefined
  if (args[0] === '--group') {
    group = args[1]
    if (group !== 'other' && !intervalGroups.has(group ?? '')) {
      console.error('FAIL --group requires base, discard, overflow or other')
      return 1
    }
    args = args.slice(2)
  }

  // Check every discovered configuration, never a hand-maintained allowlist.
  let count = 0
  let failed = 0
  for (const target of args.length ? args : ['.']) {
    for (const model of findModels(target)) {
      if (group !== undefined && groupOf(model) !== group) continue

      // Choose the checker without bypassing a model's chart validation.
      const name = path.relative('.', model.config)
      const native =
        [
          'db/block/gc/IntervalCounts.tla',
          'core/sobject/world/engine/DeadlistLedger.tla',
        ].some((spec) => path.resolve(model.spec) === path.resolve(spec)) &&
        viewOf(model.spec) === undefined
      console.log(`start ${native ? 'TLC' : 'tla-ts'} ${name}`)
      const start = performance.now()
      count++

      // Retain each failure while checking the remaining configurations.
      try {
        if (native) {
          await checkTLC(model)
        } else {
          const result = await checkModel(model)
          if (!result.ok) {
            for (const line of [
              result.rule,
              result.error,
              ...(result.trace ?? []),
            ])
              if (line) console.error(line)
            throw new Error('model or chart check failed')
          }
        }
        console.log(`pass ${(performance.now() - start).toFixed(0)} ms ${name}`)
      } catch (error) {
        failed++
        console.error(`FAIL ${name}: ${error}`)
      }
    }
  }

  // A misspelled selection must not turn an intended proof into a green no-op.
  if (count === 0) {
    console.error('FAIL no model configurations selected')
    return 1
  }

  // Preserve the aggregate failure result after every selected model finishes.
  console.log(`${count} model(s), ${failed} failed`)
  return failed ? 1 : 0
}

process.exitCode = await main(process.argv.slice(2))
