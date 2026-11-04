#!/usr/bin/env bun

// check-tla runs the shared tla-ts model and chart checks over every model,
// split into groups so the large counting searches run as separate CI jobs.
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import * as path from 'node:path'
import { checkModel, findModels, type Model } from '@aptre/common/tla'

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
      const name = path.relative('.', model.config)
      console.log(`start ${name}`)
      const start = performance.now()
      count++

      // Retain each failure while checking the remaining configurations.
      const result = await checkModel(model)
      if (result.ok) {
        console.log(`pass ${(performance.now() - start).toFixed(0)} ms ${name}`)
        continue
      }
      failed++
      for (const line of [result.rule, result.error, ...(result.trace ?? [])])
        if (line) console.error(line)
      console.error(`FAIL ${name}`)
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
