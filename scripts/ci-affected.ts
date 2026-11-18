#!/usr/bin/env bun

// ci-affected selects the work of each member job and writes it as GitHub
// Actions job outputs. Every job runs in every mode; the selection only narrows
// its steps. A full run selects every step. A delta run compares the checkout
// with a base commit: it tests the changed Go packages and every package whose
// tests depend on one, lints the changed Go packages, and runs the JavaScript
// checks when a JavaScript or TypeScript input changed. A job with no selected
// step passes after setup. The TLA+ spec jobs are the exception: a delta run
// lists only the spec jobs whose models changed, other than those of
// fullRunSpecs, even when a job definition selected the full suite, and the
// workflow skips the spec matrix when it lists none. Every spec job checks its
// configs through check:tla:configs and judges no mutant: the mutants take
// hours, so they leave the runs that gate a landing, and the mutants mode
// lists the nightly jobs that judge them.
//
// Usage: bun scripts/ci-affected.ts full
//        bun scripts/ci-affected.ts delta <base-commit>
//        bun scripts/ci-affected.ts mutants
//
// Outside GitHub Actions the outputs print to stdout.

import { spawnSync } from 'node:child_process'
import { appendFileSync, existsSync, readFileSync } from 'node:fs'
import { basename, dirname, join } from 'node:path'

import { goscriptShards } from './goscript-test.js'

// fullRunPaths select the full suite when changed: they define the jobs, the
// toolchain, or the lint rules, so no package graph bounds their effect.
const fullRunPaths = [
  /^\.github\/workflows\//,
  /^scripts\/ci-affected\.ts$/,
  /^scripts\/goscript-test\.ts$/,
  /^package\.json$/,
  /^bun\.lock$/,
  /^patches\//,
  /^\.golangci\.yml$/,
  /^\.custom-gcl\.yml$/,
  /^lint\//,
]

// checkerPaths are the full-run paths the spec jobs read: the checker and its
// version, which bun.lock pins. Any other full-run path leaves a delta run's
// spec jobs to the specs that changed.
const checkerPaths = [/^package\.json$/, /^bun\.lock$/, /^patches\//]

// fullRunSpecs are the specs a delta run leaves to the full runs, because their
// models take longer than a delta run may hold its branch's queue.
// The CI hourly workflow's full dispatch checks them on master. A delta run
// that changes one checks only its stackConfigs, in one job.
const fullRunSpecs = new Set(['db/block/gc/Journal.tla'])

// stackConfigs are the Journal configs a delta run checks when the spec
// changes, with their times on a GitHub runner on one core; they sum to about
// 11 minutes. A config that grows past its time moves to the full run.
const stackConfigs = [
  'db/block/gc/Journal-collect-notail.cfg', // 6 s
  'db/block/gc/Journal-rogue-hold.cfg', // 15 s
  'db/block/gc/Journal-stage.cfg', // 38 s
  'db/block/gc/Journal-writers-staged.cfg', // 96 s
  'db/block/gc/Journal-hole.cfg', // 107 s
  'db/block/gc/Journal-adopt-revive-hold.cfg', // 148 s
  'db/block/gc/Journal-lease-notail.cfg', // 179 s
  'db/block/gc/Journal-collect.cfg', // 6 s
  'db/block/gc/Journal-fill.cfg', // 6 s
  'db/block/gc/Journal-revive-hold.cfg', // 1 s
  'db/block/gc/Journal-adopt-cut-onerun.cfg', // 42 s
  'db/block/gc/Journal-fill-adopt.cfg', // 8 s
]

// unrunConfigs are the model configs no CI run checks, each with its reason.
// The host-only ones need more memory than the runner's 16 GB and are checked
// on a build host.
const unrunConfigs = new Map([
  [
    'db/block/gc/Journal-adopt-cut-n.cfg',
    'host-only: 6.0 million states, 44 minutes and a 14.5 GB peak',
  ],
  [
    'db/block/gc/Journal-adopt-cut-nostage.cfg',
    'host-only: unfinished after 53 minutes at 14.7 GB',
  ],
  [
    'db/block/gc/Journal-adopt-revive-hole.cfg',
    'fails CountsCover, which stays open until the adoption window is decided',
  ],
  [
    'db/block/gc/Journal-adopt-revive-hole-accept.cfg',
    'fails CountsCover as Journal-adopt-revive-hole does, at depth 29',
  ],
])

// jsInputs are the files the TypeScript typecheck, oxlint, and JavaScript
// tests read.
const jsInputs =
  /\.(ts|tsx|js|jsx|mjs|cjs|css|html)$|(^|\/)tsconfig[^/]*\.json$|^\.oxlintrc/

// sliceCoverageInputs are read by the wasm E2E slice coverage test.
const sliceCoverageInputs = /^e2e\/(wasm|scenario)\//

// Member is one job of the member matrix. roots are the Go package roots its
// test and lint scripts cover; the scripts read GO_PACKAGES and
// GO_LINT_PACKAGES and default to the roots.
interface Member {
  name: string
  roots: string[]
  build_script: string
  lint_go_script: string
  lint_js_script: string
  test_go_script: string
  test_js_script: string
}

const members: Member[] = [
  {
    name: 'auth',
    roots: ['auth'],
    build_script: '',
    lint_go_script: 'lint:go:auth',
    lint_js_script: 'lint:js:auth',
    test_go_script: 'test:go:auth',
    test_js_script: 'test:js:auth',
  },
  {
    name: 'forge',
    roots: ['forge'],
    build_script: '',
    lint_go_script: 'lint:go:forge',
    lint_js_script: 'lint:js:forge',
    test_go_script: 'test:go:forge',
    test_js_script: 'test:js:forge',
  },
  {
    name: 'net',
    roots: ['net'],
    build_script: 'build:net',
    lint_go_script: 'lint:go:net',
    lint_js_script: 'lint:js:net',
    test_go_script: 'test:go:net',
    test_js_script: 'test:js:net',
  },
  {
    name: 'identity',
    roots: ['identity'],
    build_script: '',
    lint_go_script: 'lint:go:identity',
    lint_js_script: 'lint:js:identity',
    test_go_script: 'test:go:identity',
    test_js_script: 'test:js:identity',
  },
  {
    name: 'e2e',
    roots: ['e2e'],
    build_script: '',
    lint_go_script: 'lint:go:e2e',
    lint_js_script: '',
    test_go_script: 'test:go:e2e:wasm:slice-coverage',
    test_js_script: '',
  },
  {
    name: 'bldr',
    roots: ['bldr'],
    build_script: '',
    lint_go_script: '',
    lint_js_script: '',
    test_go_script: 'test:go:bldr',
    test_js_script: '',
  },
  // Each GoScript shard runs its test:go:goscript:<shard> script.
  ...Object.keys(goscriptShards).map((shard) => ({
    name: `goscript-${shard}`,
    roots: [],
    build_script: '',
    lint_go_script: '',
    lint_js_script: '',
    test_go_script: `test:go:goscript:${shard}`,
    test_js_script: '',
  })),
]

const alphaRoots = ['cmd', 'core', 'plugin', 'sdk']
const dbRoots = ['db', 'runtimeenv']
const startupTraceRoots = ['bldr', 'db']

// MemberEntry is one member matrix entry. go_packages and lint_packages are
// space-separated package directories; empty means the script's default.
interface MemberEntry extends Omit<Member, 'roots'> {
  go_packages: string
  lint_packages: string
}

// AlphaEntry is one alpha stage matrix entry.
interface AlphaEntry {
  name: string
  script: string
  check_script: string
  go_packages: string
}

// DbEntry is one db stage matrix entry. Each flag selects one step; the
// package lists narrow the step and are empty for the script's default.
interface DbEntry {
  name: string
  startup_trace: boolean
  lint_go: boolean
  lint_packages: string
  test_go: boolean
  go_packages: string
  test_go_js: boolean
  go_js_packages: string
  js: boolean
}

// dbChecks and dbTests are the two db stages with no step selected.
const dbChecks: DbEntry = {
  name: 'checks',
  startup_trace: false,
  lint_go: false,
  lint_packages: '',
  test_go: false,
  go_packages: '',
  test_go_js: false,
  go_js_packages: '',
  js: false,
}
const dbTests: DbEntry = { ...dbChecks, name: 'tests' }

// SpecEntry is one TLA+ spec job matrix entry: the configs it checks, with
// workers checker threads, within timeout minutes. In ownSpecJobs a config
// may be a Git pathspec glob that names a family of configs.
interface SpecEntry {
  name: string
  configs: string
  workers: number
  timeout: number
}

// ownSpecJobs are the models that take minutes, each checked in its own job so
// no job nears its timeout, alone or with a family of configs whose times sum
// within it; every other config, including a new one, checks in the models
// job, whose configs take about 13 minutes on one core. A model that needs
// longer than 25 minutes on one core checks on the runner's four cores when
// tla-ts spreads it across them and that fits 25 minutes, and otherwise moves
// to nightlyConfigJobs. Journal.cfg, which composes every rule with concurrent
// writers, reaches 293,186 states in 8 minutes on the runner's four cores and
// in 167 s on four build host cores, so four build host cores run about three
// times as fast as a runner's four. Unless an entry says otherwise, the times
// are on one core of a build host, which runs as fast as a GitHub runner.
const ownSpecJobs: SpecEntry[] = [
  {
    // Sixteen configs, 563 s together.
    name: 'Transfer',
    configs: 'core/provider/transfer/Transfer-*.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    // AuditWalk takes 264 s and its siblings 41 s.
    name: 'AuditWalk',
    configs: 'db/block/gc/AuditWalk*.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    name: 'ReplaySkip-Stale',
    configs: 'core/space/world/ReplaySkip-Stale.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    name: 'Journal-rogue',
    configs: 'db/block/gc/Journal-rogue.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    name: 'Journal',
    configs: 'db/block/gc/Journal.cfg',
    workers: 4,
    timeout: 25,
  },
  {
    name: 'Journal-adopt',
    configs: 'db/block/gc/Journal-adopt.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    // 28 to 43 minutes on a runner's one core; 385 s on four build host
    // cores, about 19 minutes on a runner's four.
    name: 'Journal-adopt-cut',
    configs: 'db/block/gc/Journal-adopt-cut.cfg',
    workers: 4,
    timeout: 45,
  },
  {
    // 14 to 25 minutes on a runner's one core; 224 s on four build host
    // cores, about 11 minutes on a runner's four.
    name: 'Journal-adopt-cut-open',
    configs: 'db/block/gc/Journal-adopt-cut-open.cfg',
    workers: 4,
    timeout: 30,
  },
  {
    name: 'Journal-adopt-revive-reader',
    configs: 'db/block/gc/Journal-adopt-revive-reader.cfg',
    workers: 1,
    timeout: 30,
  },
  {
    name: 'Journal-adopt-revive-replaced',
    configs: 'db/block/gc/Journal-adopt-revive-replaced.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    name: 'Journal-hole',
    configs: 'db/block/gc/Journal-hole.cfg',
    workers: 1,
    timeout: 25,
  },
  {
    name: 'Journal-lease',
    configs: 'db/block/gc/Journal-lease.cfg',
    workers: 1,
    timeout: 45,
  },
  {
    name: 'Journal-notail',
    configs: 'db/block/gc/Journal-notail.cfg',
    workers: 1,
    timeout: 30,
  },
  {
    name: 'Journal-stage',
    configs: 'db/block/gc/Journal-stage.cfg',
    workers: 1,
    timeout: 60,
  },
  {
    name: 'Journal-fill-staged',
    configs: 'db/block/gc/Journal-fill-staged.cfg',
    workers: 1,
    timeout: 60,
  },
  {
    name: 'Journal-fill-writers',
    configs: 'db/block/gc/Journal-fill-writers.cfg',
    workers: 1,
    timeout: 60,
  },
  {
    name: 'Journal-writers-staged',
    configs: 'db/block/gc/Journal-writers-staged.cfg',
    workers: 1,
    timeout: 25,
  },
]

// nightlyConfigJobs are the models that take longer than 25 minutes on a CI
// runner, too long for a run that gates a landing; the nightly TLA+ mutants
// workflow checks each in its own job, and no gating run checks them.
const nightlyConfigJobs: SpecEntry[] = [
  {
    // Unfinished after 19 minutes at 2.6 GB on one core.
    name: 'Journal-adopt-hold',
    configs: 'db/block/gc/Journal-adopt-hold.cfg',
    workers: 1,
    timeout: 120,
  },
  {
    // Unfinished after 19 minutes at 5.4 GB on one core.
    name: 'Journal-adopt-revive',
    configs: 'db/block/gc/Journal-adopt-revive.cfg',
    workers: 1,
    timeout: 120,
  },
  {
    // Unfinished after 19 minutes at 3.1 GB on one core.
    name: 'Journal-adopt-revive-fillnew',
    configs: 'db/block/gc/Journal-adopt-revive-fillnew.cfg',
    workers: 1,
    timeout: 120,
  },
  {
    // Exceeded its 45-minute timeout on a CI runner.
    name: 'Journal-adopt-cut-r2',
    configs: 'db/block/gc/Journal-adopt-cut-r2.cfg',
    workers: 1,
    timeout: 120,
  },
  {
    // Exceeded its 90-minute timeout on a CI runner twice. Given four workers,
    // tla-ts still checks it on one thread, and it was unfinished after 20
    // minutes on a build host.
    name: 'Journal-adopt-revive-fill',
    configs: 'db/block/gc/Journal-adopt-revive-fill.cfg',
    workers: 1,
    timeout: 180,
  },
  {
    // 47 to 81 minutes on a runner's one core; 693 s on four build host
    // cores, about 35 minutes on a runner's four.
    name: 'Journal-hole-fill',
    configs: 'db/block/gc/Journal-hole-fill.cfg',
    workers: 4,
    timeout: 120,
  },
]

// nightlyConfigs are the configs of nightlyConfigJobs.
const nightlyConfigs = new Set(nightlyConfigJobs.map((job) => job.configs))

// MutantEntry is one nightly TLA+ mutant job matrix entry: the mutant sources
// it judges through check:tla:mutants, on one checker thread within timeout
// minutes. A source is a spec, for all of its mutants, its Foo.mutants.json,
// or one testdata config mutant.
interface MutantEntry {
  name: string
  targets: string
  timeout: number
}

// splitMutantSpecs are the specs whose mutants take hours, each source of which
// is judged in its own job: Foo.mutants.json, and each testdata config mutant.
// The mutants of every other spec a CI run checks share one job. One thread
// judges faster than four: Journal's spec mutants take 12 minutes on one and
// 31 on four.
const splitMutantSpecs = ['db/block/gc/Journal.tla']

// slowConfigMutants are the testdata config mutants not yet judged within 100
// minutes on one core of a build host; each keeps a 360-minute job until its
// time is known. Every other config mutant is judged within 39 minutes, and
// its job stops at 60.
const slowConfigMutants = new Set([
  'Journal-adopt-cut-norecount',
  'Journal-adopt-revive-fill-nofillbarrier',
  'Journal-adopt-revive-fill-nofilllanded',
])

// mutantEntries returns the nightly mutant jobs: one of the specs whose
// configs the spec jobs check, less splitMutantSpecs, and one per mutant
// source of each of those.
function mutantEntries(): MutantEntry[] {
  const split = new Set(splitMutantSpecs)
  const tlaFiles = trackedSpecs()
  const specs = new Set<string>()
  for (const config of specConfigs()) {
    const spec = specOf(config, tlaFiles)
    if (spec !== undefined && !split.has(spec)) {
      specs.add(spec)
    }
  }
  const entries: MutantEntry[] = [
    { name: 'specs', targets: [...specs].sort().join(' '), timeout: 120 },
  ]
  for (const spec of splitMutantSpecs) {
    const stem = basename(spec, '.tla')
    const list = spec.replace(/\.tla$/, '.mutants.json')
    if (existsSync(list)) {
      entries.push({ name: stem, targets: list, timeout: 60 })
    }
    const testdata = run('git', [
      'ls-files',
      `${dirname(spec)}/testdata/${stem}.cfg`,
      `${dirname(spec)}/testdata/${stem}-*.cfg`,
    ])
    for (const config of testdata.split('\n').filter((f) => f !== '')) {
      const name = basename(config, '.cfg')
      entries.push({
        name,
        targets: config,
        timeout: slowConfigMutants.has(name) ? 360 : 60,
      })
    }
  }
  return entries
}

// Selection is the work one run performs.
interface Selection {
  mode: 'full' | 'delta'
  members: MemberEntry[]
  alpha: AlphaEntry[]
  db: DbEntry[]
  specs: SpecEntry[]
}

// Graph is the module's package graph from go list.
interface Graph {
  // dirs maps each package directory, relative to the module root, to its
  // import path.
  dirs: Map<string, string>
  // dependents maps each import path, in or outside the module, to the module
  // packages whose tests depend on it.
  dependents: Map<string, Set<string>>
  // module is the main module path.
  module: string
}

// run executes a command and returns its standard output, exiting on failure.
function run(command: string, args: string[]): string {
  const result = spawnSync(command, args, {
    encoding: 'utf8',
    maxBuffer: 256 * 1024 * 1024,
    stdio: ['ignore', 'pipe', 'inherit'],
  })
  if (result.status !== 0) {
    console.error(`ci-affected: ${command} ${args.join(' ')} failed`)
    process.exit(1)
  }
  return result.stdout
}

// loadGraph reads every package and its test dependencies with the build tags
// the member tests use.
function loadGraph(): Graph {
  const module = run('go', ['list', '-m']).trim()
  const template = '{{.ImportPath}}\t{{.ForTest}}\t{{.Dir}}\t{{join .Deps ","}}'
  const out = run('go', [
    'list',
    '-e',
    '-test',
    '-tags',
    'purego,skip_e2e',
    '-f',
    template,
    './...',
  ])

  const root = process.cwd()
  const dirs = new Map<string, string>()
  const dependents = new Map<string, Set<string>>()
  for (const line of out.split('\n')) {
    if (line === '') {
      continue
    }

    // A test variant stands for the package it tests; a test main adds nothing.
    const [importPath, forTest, dir, deps] = line.split('\t')
    if (importPath.endsWith('.test')) {
      continue
    }
    const pkg = forTest || importPath
    if (!forTest) {
      dirs.set(relativeDir(root, dir), pkg)
    }

    // Record the package as a dependent of each package it builds against.
    for (const dep of deps.split(',')) {
      const name = dep.replace(/ \[.*\]$/, '')
      let set = dependents.get(name)
      if (!set) {
        set = new Set()
        dependents.set(name, set)
      }
      set.add(pkg)
    }
  }
  return { dirs, dependents, module }
}

// relativeDir returns dir relative to root, with '.' for the root itself.
function relativeDir(root: string, dir: string): string {
  if (dir === root) {
    return '.'
  }
  return dir.slice(root.length + 1)
}

// packageOf returns the import path of the package that owns a changed file. A
// Go file belongs to the package in its own directory; none owns a Go file that
// build constraints exclude. Other files, such as embeds and testdata, belong to
// the nearest enclosing package below the repository root.
function packageOf(graph: Graph, file: string): string | undefined {
  if (file.endsWith('.go')) {
    return graph.dirs.get(dirname(file))
  }
  for (let dir = dirname(file); dir !== '.'; dir = dirname(dir)) {
    const pkg = graph.dirs.get(dir)
    if (pkg !== undefined) {
      return pkg
    }
  }
  return undefined
}

// changedModules returns the module paths whose required version differs
// between the base and head go.mod. It returns undefined when a directive
// other than require changed, since that can affect every package.
function changedModules(base: string): Set<string> | undefined {
  const before = parseGoMod(run('git', ['show', `${base}:go.mod`]))
  const after = parseGoMod(readFileSync('go.mod', 'utf8'))
  if (before.other !== after.other) {
    return undefined
  }

  const changed = new Set<string>()
  for (const [path, version] of after.require) {
    if (before.require.get(path) !== version) {
      changed.add(path)
    }
  }
  for (const path of before.require.keys()) {
    if (!after.require.has(path)) {
      changed.add(path)
    }
  }
  return changed
}

// parseGoMod splits go.mod into its required module versions and the text of
// every other directive.
function parseGoMod(text: string): {
  require: Map<string, string>
  other: string
} {
  const require = new Map<string, string>()
  const other: string[] = []
  let inRequire = false
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\/\/.*$/, '').trim()
    if (line === '') {
      continue
    }

    // Track require blocks and single-line requires.
    if (line === 'require (') {
      inRequire = true
      continue
    }
    if (inRequire && line === ')') {
      inRequire = false
      continue
    }
    const fields = line.replace(/^require\s+/, '').split(/\s+/)
    if (inRequire || line.startsWith('require ')) {
      require.set(fields[0], fields[1])
      continue
    }
    other.push(line)
  }
  return { require, other: other.join('\n') }
}

// within reports whether a package directory lies under one of the roots.
function within(dir: string, roots: string[]): boolean {
  return roots.some((root) => dir === root || dir.startsWith(root + '/'))
}

// packageDirs returns the sorted ./ directories of the given import paths that
// lie under roots.
function packageDirs(
  graph: Graph,
  pkgs: Set<string>,
  roots: string[],
): string[] {
  const out: string[] = []
  for (const [dir, pkg] of graph.dirs) {
    if (pkgs.has(pkg) && within(dir, roots)) {
      out.push(dir === '.' ? '.' : './' + dir)
    }
  }
  return out.sort()
}

// goscriptPackages returns the import paths of the packages the GoScript
// shards test.
function goscriptPackages(graph: Graph): Set<string> {
  const out = new Set<string>()
  for (const pkg of Object.values(goscriptShards).flat()) {
    out.add(pkg.startsWith('./') ? graph.module + '/' + pkg.slice(2) : pkg)
  }
  return out
}

// gitFiles returns the tracked files the pathspecs name.
function gitFiles(pathspecs: string[]): string[] {
  return run('git', ['ls-files', '--', ...pathspecs])
    .split('\n')
    .filter((file) => file !== '')
}

// specConfigs returns every model config the spec jobs check: the tracked
// .cfg files outside testdata, less unrunConfigs and nightlyConfigs.
function specConfigs(): string[] {
  return run('git', ['ls-files', '*.cfg', ':!:**/testdata/**'])
    .split('\n')
    .filter(
      (file) =>
        file !== '' && !unrunConfigs.has(file) && !nightlyConfigs.has(file),
    )
}

// specEntries returns the spec jobs that check a config selected allows: each
// own job, holding those of its configs it allows, and a models job of the
// other configs it allows.
function specEntries(selected: (config: string) => boolean): SpecEntry[] {
  const all = specConfigs()
  const runnable = new Set(all)
  const jobs = ownSpecJobs.map((job) => ({
    job,
    configs: gitFiles(job.configs.split(' ')).filter((cfg) =>
      runnable.has(cfg),
    ),
  }))
  const own = new Set(jobs.flatMap((j) => j.configs))
  const models = all.filter((cfg) => !own.has(cfg) && selected(cfg))
  const entries: SpecEntry[] = []
  for (const { job, configs } of jobs) {
    const chosen = configs.filter(selected)
    if (chosen.length > 0) {
      entries.push({ ...job, configs: chosen.join(' ') })
    }
  }
  if (models.length > 0) {
    entries.unshift({
      name: 'models',
      configs: models.join(' '),
      workers: 1,
      timeout: 25,
    })
  }
  return entries
}

// deltaSpecs returns the spec jobs a delta run checks: the models whose
// config, spec or spec modules changed, less those of fullRunSpecs, and one
// Journal-stack job of the changed stackConfigs.
function deltaSpecs(files: string[]): SpecEntry[] {
  const changed = changedSpec(files)
  const tlaFiles = trackedSpecs()
  const entries = specEntries((config) => {
    const spec = specOf(config, tlaFiles)
    return changed(config) && (spec === undefined || !fullRunSpecs.has(spec))
  })
  const stack = stackConfigs.filter(changed)
  if (stack.length > 0) {
    entries.push({
      name: 'Journal-stack',
      configs: stack.join(' '),
      workers: 1,
      timeout: 25,
    })
  }
  return entries
}

// specOf returns the spec the checker pairs config with: the .tla in its
// directory with the longest stem that is the config's name or a dash
// separated prefix of it, or undefined when none is, which the checker skips.
function specOf(config: string, tlaFiles: Set<string>): string | undefined {
  const dir = dirname(config)
  const name = basename(config, '.cfg')
  let best: string | undefined
  for (const file of tlaFiles) {
    if (dirname(file) !== dir) {
      continue
    }
    const stem = basename(file, '.tla')
    if (
      (name === stem || name.startsWith(stem + '-')) &&
      (!best || stem.length > basename(best, '.tla').length)
    ) {
      best = file
    }
  }
  return best
}

// specModules returns spec and every module of its directory it EXTENDS or
// INSTANCEs, directly or through another. The checker reads the remaining
// modules from its own package, which bun.lock pins.
function specModules(spec: string): Set<string> {
  const dir = dirname(spec)
  const seen = new Set<string>()
  const queue = [spec]
  for (let file = queue.pop(); file !== undefined; file = queue.pop()) {
    if (seen.has(file) || !existsSync(file)) {
      continue
    }
    seen.add(file)
    const text = readFileSync(file, 'utf8')
    for (const [, list] of text.matchAll(/\bEXTENDS\s+(\w+(?:\s*,\s*\w+)*)/g)) {
      list
        .split(',')
        .forEach((name) => queue.push(join(dir, name.trim() + '.tla')))
    }
    for (const [, name] of text.matchAll(/\bINSTANCE\s+(\w+)/g)) {
      queue.push(join(dir, name + '.tla'))
    }
  }
  return seen
}

// trackedSpecs returns the tracked .tla files outside testdata.
function trackedSpecs(): Set<string> {
  return new Set(
    run('git', ['ls-files', '*.tla', ':!:**/testdata/**'])
      .split('\n')
      .filter((file) => file !== ''),
  )
}

// changedSpec reports whether a model config is affected by files: the
// config, its spec or a module the spec reads changed.
function changedSpec(files: string[]): (config: string) => boolean {
  const changed = new Set(files)
  const tlaFiles = trackedSpecs()
  return (config) => {
    if (changed.has(config)) {
      return true
    }
    const spec = specOf(config, tlaFiles)
    return (
      spec !== undefined &&
      [...specModules(spec)].some((file) => changed.has(file))
    )
  }
}

// fullSelection selects every job with each script's default packages.
function fullSelection(): Selection {
  return {
    mode: 'full',
    specs: specEntries(() => true),
    members: members.map(({ roots: _, ...member }) => ({
      ...member,
      go_packages: '',
      lint_packages: '',
    })),
    alpha: [
      {
        name: 'test-go',
        script: 'test:go:alpha',
        check_script: 'typecheck',
        go_packages: '',
      },
      {
        name: 'test-js',
        script: 'test:js:alpha',
        check_script: 'lint:js:alpha',
        go_packages: '',
      },
      {
        name: 'lint-go',
        script: 'lint:go:alpha',
        check_script: '',
        go_packages: '',
      },
    ],
    db: [
      { ...dbChecks, startup_trace: true, lint_go: true },
      { ...dbTests, test_go: true, test_go_js: true, js: true },
    ],
  }
}

// deltaSelection selects the work the changes since base need.
function deltaSelection(base: string): Selection {
  const files = run('git', ['diff', '--name-only', base, 'HEAD'])
    .split('\n')
    .filter((file) => file !== '')
  console.error(`ci-affected: ${files.length} files changed since ${base}`)

  // Configuration and toolchain changes reach every job.
  const fullFile = files.find((file) =>
    fullRunPaths.some((re) => re.test(file)),
  )
  if (fullFile) {
    console.error(`ci-affected: ${fullFile} selects the full suite`)
    const full = fullSelection()
    if (files.some((file) => checkerPaths.some((re) => re.test(file)))) {
      return full
    }
    return { ...full, specs: deltaSpecs(files) }
  }

  // Map changed files to changed packages, and changed module versions to the
  // packages that import those modules.
  const graph = loadGraph()
  const changed = new Set<string>()
  for (const file of files) {
    const pkg = packageOf(graph, file)
    if (pkg !== undefined) {
      changed.add(pkg)
    }
  }
  const tested = new Set(changed)
  let modulesChanged = false
  if (files.includes('go.mod')) {
    const modules = changedModules(base)
    if (!modules) {
      console.error('ci-affected: a go.mod directive selects the full suite')
      return fullSelection()
    }
    modulesChanged = modules.size > 0
    for (const [dep, users] of graph.dependents) {
      if ([...modules].some((m) => dep === m || dep.startsWith(m + '/'))) {
        users.forEach((pkg) => tested.add(pkg))
      }
    }
  }

  // Add every package whose tests build against a changed package.
  for (const pkg of changed) {
    graph.dependents.get(pkg)?.forEach((dep) => tested.add(dep))
  }
  console.error(
    `ci-affected: ${changed.size} packages changed, ${tested.size} selected`,
  )

  // Build every member entry, clearing the steps with no work.
  const js = files.some((file) => jsInputs.test(file))
  const goscript = goscriptPackages(graph)
  const goscriptSelected =
    modulesChanged || [...goscript].some((pkg) => tested.has(pkg))
  const sliceCoverage = files.some((file) => sliceCoverageInputs.test(file))
  const memberEntries: MemberEntry[] = []
  for (const { roots, ...member } of members) {
    const goPackages = packageDirs(graph, tested, roots)
    const lintPackages = packageDirs(graph, changed, roots)
    let testGo = goPackages.length > 0 ? member.test_go_script : ''
    if (member.name === 'e2e') {
      testGo = sliceCoverage ? member.test_go_script : ''
    }
    if (member.name.startsWith('goscript-')) {
      testGo = goscriptSelected ? member.test_go_script : ''
    }
    memberEntries.push({
      ...member,
      build_script: js ? member.build_script : '',
      lint_go_script: lintPackages.length > 0 ? member.lint_go_script : '',
      lint_js_script: js ? member.lint_js_script : '',
      test_go_script: testGo,
      test_js_script: js ? member.test_js_script : '',
      go_packages: member.name === 'e2e' ? '' : goPackages.join(' '),
      lint_packages: lintPackages.join(' '),
    })
  }

  // Build every alpha stage, clearing the steps with no work.
  const alphaTested = packageDirs(graph, tested, alphaRoots)
  const alphaChanged = packageDirs(graph, changed, alphaRoots)
  const alpha: AlphaEntry[] = [
    {
      name: 'test-go',
      script: alphaTested.length > 0 ? 'test:go:alpha' : '',
      check_script: js ? 'typecheck' : '',
      go_packages: alphaTested.join(' '),
    },
    {
      name: 'test-js',
      script: js ? 'test:js:alpha' : '',
      check_script: js ? 'lint:js:alpha' : '',
      go_packages: '',
    },
    {
      name: 'lint-go',
      script: alphaChanged.length > 0 ? 'lint:go:alpha' : '',
      check_script: '',
      go_packages: alphaChanged.join(' '),
    },
  ]

  // Build both db stages, clearing the steps with no work. The js-only Go tests cover every
  // product tree, so they take every selected package. The graph is the host
  // build's, so a delta run misses a js-only test whose dependency changed
  // only in js-tagged files; the hourly full run covers it.
  const dbTested = packageDirs(graph, tested, dbRoots)
  const dbChanged = packageDirs(graph, changed, dbRoots)
  const startupTrace = packageDirs(graph, tested, startupTraceRoots).length > 0
  const goJS = packageDirs(graph, tested, [...graph.dirs.keys()])
  const db: DbEntry[] = [
    {
      ...dbChecks,
      startup_trace: startupTrace,
      lint_go: dbChanged.length > 0,
      lint_packages: dbChanged.join(' '),
    },
    {
      ...dbTests,
      test_go: dbTested.length > 0,
      go_packages: dbTested.join(' '),
      test_go_js: goJS.length > 0,
      go_js_packages: goJS.join(' '),
      js,
    },
  ]

  const specs = deltaSpecs(files)
  console.error(`ci-affected: ${specs.length} spec jobs selected`)

  return {
    mode: 'delta',
    specs,
    members: memberEntries,
    alpha,
    db,
  }
}

// matrix returns a JSON matrix value of include.
const matrix = (include: object[]) => JSON.stringify({ include })

// writeSelection publishes the selection as job outputs.
function writeSelection(selection: Selection) {
  writeOutputs({
    mode: selection.mode,
    members: matrix(selection.members),
    alpha: matrix(selection.alpha),
    db: matrix(selection.db),
    specs: matrix(selection.specs),
  })
}

// writeOutputs publishes outputs as job outputs.
function writeOutputs(outputs: Record<string, string>) {
  const text = Object.entries(outputs)
    .map(([key, value]) => `${key}=${value}\n`)
    .join('')

  const path = process.env.GITHUB_OUTPUT
  if (path) {
    appendFileSync(path, text)
  }
  process.stdout.write(text)
}

const [mode, base] = process.argv.slice(2)
if (mode === 'full') {
  writeSelection(fullSelection())
} else if (mode === 'delta' && base) {
  writeSelection(deltaSelection(base))
} else if (mode === 'mutants') {
  writeOutputs({
    mutants: matrix(mutantEntries()),
    configs: matrix(nightlyConfigJobs),
  })
} else {
  console.error('usage: ci-affected.ts full | delta <base-commit> | mutants')
  process.exit(2)
}
