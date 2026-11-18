#!/usr/bin/env bun
// check-tla-configs splits what aptre-tla does in one pass into the two tiers
// CI runs it in. aptre-tla checks each config and then judges every mutant of
// each spec it checked, spec mutants and testdata/ config mutants alike, which
// for Journal.tla takes hours; a job that gates a landing cannot wait for that.
//
// usage: bun scripts/check-tla-configs.ts [-workers N] config.cfg...
//        bun scripts/check-tla-configs.ts -mutants [-workers N] target...
//
// The first form checks the named configs and judges no mutant; every CI run
// uses it. The second judges mutants and checks no config; the nightly mutant
// workflow uses it. Its target is Foo.tla for every mutant of the spec,
// Foo.mutants.json for the spec mutants listed there, or testdata/Foo-x.cfg
// for that one config mutant, so a spec whose mutants take hours can split
// them across jobs.
//
// Prints one line per result and exits 1 when a config fails or a mutant is
// not a kill, and 2 when the arguments are wrong.
import { existsSync } from 'node:fs'
import path from 'node:path'

import {
  checkModel,
  checkMutant,
  configMutant,
  configMutantsOf,
  findConfigMutants,
  findModels,
  type Model,
  mutantsOf,
  sharedModulesDir,
  type ModelResult,
  type Mutant,
  type Progress,
  unjudged,
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
  // Parse the mode, the worker count and the targets.
  let workers = 1
  let mutants = false
  const targets: string[] = []
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '-workers') {
      workers = Number(args[++i])
    } else if (args[i] === '-mutants') {
      mutants = true
    } else {
      targets.push(args[i])
    }
  }
  const exts = mutants ? ['.tla', '.mutants.json', '.cfg'] : ['.cfg']
  if (
    !Number.isInteger(workers) ||
    workers < 1 ||
    targets.length === 0 ||
    targets.some((t) => !exts.some((ext) => t.endsWith(ext)))
  ) {
    console.error(
      'usage: check-tla-configs [-workers N] config.cfg...\n' +
        '       check-tla-configs -mutants [-workers N] spec.tla|spec.mutants.json|testdata/config.cfg...',
    )
    return 2
  }
  const shared = sharedModulesDir()
  const failed = mutants
    ? await judgeMutants(targets, shared, workers)
    : await checkConfigs(targets, shared, workers)
  if (failed < 0) {
    return 2
  }
  console.log(`${targets.length} target(s), ${failed} failed`)
  return failed ? 1 : 0
}

// checkConfigs checks each config against its spec and returns how many
// failed, or -1 when one pairs with no spec.
async function checkConfigs(
  configs: string[],
  shared: string,
  workers: number,
): Promise<number> {
  let failed = 0
  for (const config of configs) {
    const models = Array.from(findModels(config))
    if (models.length !== 1) {
      console.error(`check-tla-configs: ${config} pairs with no spec`)
      return -1
    }
    const name = path.relative('.', models[0].config)
    const r = await checkModel(
      models[0],
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
  return failed
}

// judgeMutants judges the mutants each target names, as aptre-tla does after
// its configs, and returns how many were not a kill, or -1 when a testdata
// config pairs with no spec.
async function judgeMutants(
  targets: string[],
  shared: string,
  workers: number,
): Promise<number> {
  let failed = 0
  const judge = (r: ModelResult & { verdict: string }, name: string) => {
    console.log(line(r.verdict, r, name))
    if (r.verdict !== 'kill') {
      failed++
      report(r)
    }
  }

  // Judge one spec mutant of Foo.mutants.json.
  const specMutant = async (spec: string, mutant: Mutant) => {
    const name = `${path.relative('.', spec)} mutant "${mutant.name}"`
    judge(
      await checkMutant(
        spec,
        mutant,
        shared,
        workers,
        progress(name, performance.now()),
      ),
      name,
    )
  }

  // Judge one testdata config mutant against its spec.
  const configMutantOf = async (model: Model) => {
    const mutant = configMutant(model)
    const name = path.relative('.', model.config)
    judge(
      mutant.expect === undefined
        ? unjudged(model, mutant, 'ERROR', 'no \\* expect: <Rule> line')
        : await checkMutant(
            model.spec,
            mutant,
            shared,
            workers,
            progress(name, performance.now()),
          ),
      name,
    )
  }

  for (const target of targets) {
    const file = path.resolve(target)
    if (file.endsWith('.cfg')) {
      const model = existsSync(file)
        ? findConfigMutants(path.dirname(file)).find((m) => m.config === file)
        : undefined
      if (model === undefined) {
        console.error(`check-tla-configs: ${target} pairs with no spec`)
        return -1
      }
      await configMutantOf(model)
      continue
    }
    if (!existsSync(file)) {
      console.error(`check-tla-configs: ${target} does not exist`)
      return -1
    }
    const spec = file.replace(/\.mutants\.json$/, '.tla')
    for (const mutant of mutantsOf(spec)) {
      await specMutant(spec, mutant)
    }
    if (file.endsWith('.tla')) {
      for (const model of configMutantsOf(spec)) {
        await configMutantOf(model)
      }
    }
  }
  return failed
}

process.exitCode = await main(process.argv.slice(2))
