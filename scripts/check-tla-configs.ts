#!/usr/bin/env bun
// check-tla-configs checks the named TLA+ configs and the spec mutants that
// name one of them, and nothing else: no other mutant of their specs and no
// config mutant in testdata/. aptre-tla judges every mutant of each spec it
// checks, which for Journal.tla takes over an hour, so a job held to minutes
// checks its configs through this and leaves the rest to the full run.
//
// usage: bun scripts/check-tla-configs.ts [-workers N] config.cfg...
//
// Prints one line per result and exits 1 when a config fails or a mutant is
// not a kill, and 2 when the arguments are wrong.
import path from 'node:path'

import {
  checkModel,
  checkMutant,
  findModels,
  mutantsOf,
  sharedModulesDir,
  type ModelResult,
  type Progress,
} from '@aptre/common/tla'

// line formats one result as aptre-tla prints it.
function line(verdict: string, r: ModelResult, name: string): string {
  const stats = `${String(r.states).padStart(7)} states ${String(r.depth).padStart(3)} deep ${r.ms.toFixed(0).padStart(6)} ms`
  return `${verdict.padEnd(5)} ${stats}  ${name}${r.rule ? `  ${r.rule}` : ''}`
}

// report prints a failed result's error and trace below its line.
function report(r: ModelResult) {
  for (const l of [...(r.error?.split('\n') ?? []), ...(r.trace ?? [])]) {
    console.log(`      ${l}`)
  }
}

// progress prints a long search's progress, which the checker reports every
// 60 s, so a job log shows where a check had got to when it stopped.
function progress(name: string, start: number) {
  return (p: Progress) => {
    const s = ((performance.now() - start) / 1000).toFixed(0)
    console.log(
      `progress  ${name}  ${s}s  distinct ${p.distinctStates}  generated ${p.generatedStates}  depth ${p.depth}  queue ${p.queuedStates}`,
    )
  }
}

async function main(args: string[]): Promise<number> {
  // Parse the worker count and the configs.
  let workers = 1
  const configs: string[] = []
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '-workers') {
      workers = Number(args[++i])
    } else {
      configs.push(args[i])
    }
  }
  if (!Number.isInteger(workers) || workers < 1 || configs.length === 0) {
    console.error('usage: check-tla-configs [-workers N] config.cfg...')
    return 2
  }

  // Check each config against its spec.
  const shared = sharedModulesDir()
  const named = new Set<string>()
  const specs = new Set<string>()
  let failed = 0
  for (const config of configs) {
    const models = Array.from(findModels(config))
    if (models.length !== 1 || !config.endsWith('.cfg')) {
      console.error(`check-tla-configs: ${config} pairs with no spec`)
      return 2
    }
    const model = models[0]
    named.add(path.resolve(model.config))
    specs.add(model.spec)
    const name = path.relative('.', model.config)
    const r = await checkModel(
      model,
      shared,
      workers,
      progress(name, performance.now()),
    )
    console.log(line(r.ok ? 'pass' : 'FAIL', r, name))
    if (!r.ok) {
      failed++
      report(r)
    }
  }

  // Judge each spec mutant whose config is one of those checked.
  for (const spec of specs) {
    const dir = path.dirname(spec)
    const own = path.basename(spec, '.tla') + '.cfg'
    for (const mutant of mutantsOf(spec)) {
      if (!named.has(path.resolve(dir, mutant.config ?? own))) {
        continue
      }
      const name = `${path.relative('.', spec)} mutant "${mutant.name}"`
      const r = await checkMutant(
        spec,
        mutant,
        shared,
        workers,
        progress(name, performance.now()),
      )
      console.log(line(r.verdict, r, name))
      if (r.verdict !== 'kill') {
        failed++
        report(r)
      }
    }
  }

  console.log(`${configs.length} model(s), ${failed} failed`)
  return failed ? 1 : 0
}

process.exitCode = await main(process.argv.slice(2))
