#!/usr/bin/env bun

// ci-affected selects the CI work a change needs and writes it as GitHub
// Actions job outputs. A full run selects every job. A delta run compares the
// checkout with a base commit: it tests the changed Go packages and every
// package whose tests depend on one, lints the changed Go packages, runs the
// JavaScript checks when a JavaScript or TypeScript input changed, and runs
// Lean, Python Resource, and Licenses when their inputs changed. Delta runs
// carry no E2E job.
//
// Usage: bun scripts/ci-affected.ts full
//        bun scripts/ci-affected.ts delta <base-commit>
//
// Outside GitHub Actions the outputs print to stdout.

import { spawnSync } from 'node:child_process'
import { appendFileSync, readFileSync } from 'node:fs'
import { dirname } from 'node:path'

// fullRunPaths select the full suite when changed: they define the jobs, the
// toolchain, or the lint rules, so no package graph bounds their effect.
const fullRunPaths = [
  /^\.github\/workflows\//,
  /^scripts\/ci-affected\.ts$/,
  /^package\.json$/,
  /^bun\.lock$/,
  /^patches\//,
  /^\.golangci\.yml$/,
  /^\.custom-gcl\.yml$/,
  /^lint\//,
]

// jsInputs are the files the TypeScript typecheck, oxlint, and JavaScript
// tests read.
const jsInputs =
  /\.(ts|tsx|js|jsx|mjs|cjs|css|html)$|(^|\/)tsconfig[^/]*\.json$|^\.oxlintrc/

// leanInputs are the proofs; the Go conformance tests join through the package
// graph.
const leanInputs = /^lean\//
const leanPackages = ['core/sobject', 'core/sobject/sync']

// pythonInputs are the Python package sources, its lockfile, and the protobuf
// definitions its bindings are generated from.
const pythonInputs =
  /^(python|spacewave_resource)\/|^pyproject\.toml$|^uv\.lock$|\.proto$|^\.protoc-python-/
const pythonPackages = [
  'bldr/resource',
  'bldr/resource/server',
  'core/resource/session',
]

// licenseInputs are the dependency manifests and the license reporter.
const licenseInputs = /^go\.(mod|sum)$|^scripts\/licenses\/|^app\/licenses\//

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
    name: 'goscript',
    roots: [],
    build_script: '',
    lint_go_script: '',
    lint_js_script: '',
    test_go_script: 'test:go:goscript',
    test_js_script: '',
  },
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

// Selection is the work one run performs.
interface Selection {
  mode: 'full' | 'delta'
  members: MemberEntry[]
  alpha: AlphaEntry[]
  db: DbEntry[]
  lean: boolean
  python: boolean
  licenses: boolean
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

// goscriptPackages returns the import paths the GoScript test script names.
function goscriptPackages(graph: Graph): Set<string> {
  const pkg = JSON.parse(readFileSync('package.json', 'utf8'))
  const script: string = pkg.scripts['test:go:goscript']
  const out = new Set<string>()
  for (const token of script.split(/\s+/)) {
    if (token.startsWith('./')) {
      out.add(graph.module + '/' + token.slice(2))
    } else if (token.startsWith(graph.module + '/')) {
      out.add(token)
    }
  }
  return out
}

// fullSelection selects every job with each script's default packages.
function fullSelection(): Selection {
  return {
    mode: 'full',
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
    lean: true,
    python: true,
    licenses: true,
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
    return fullSelection()
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

  // Build the member entries that have work.
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
    if (member.name === 'goscript') {
      testGo = goscriptSelected ? member.test_go_script : ''
    }
    const entry: MemberEntry = {
      ...member,
      build_script: js ? member.build_script : '',
      lint_go_script: lintPackages.length > 0 ? member.lint_go_script : '',
      lint_js_script: js ? member.lint_js_script : '',
      test_go_script: testGo,
      test_js_script: js ? member.test_js_script : '',
      go_packages: member.name === 'e2e' ? '' : goPackages.join(' '),
      lint_packages: lintPackages.join(' '),
    }
    if (
      entry.build_script ||
      entry.lint_go_script ||
      entry.lint_js_script ||
      entry.test_go_script ||
      entry.test_js_script
    ) {
      memberEntries.push(entry)
    }
  }

  // Build the alpha stages that have work.
  const alphaTested = packageDirs(graph, tested, alphaRoots)
  const alphaChanged = packageDirs(graph, changed, alphaRoots)
  const alpha: AlphaEntry[] = []
  if (alphaTested.length > 0 || js) {
    alpha.push({
      name: 'test-go',
      script: alphaTested.length > 0 ? 'test:go:alpha' : '',
      check_script: js ? 'typecheck' : '',
      go_packages: alphaTested.join(' '),
    })
  }
  if (js) {
    alpha.push({
      name: 'test-js',
      script: 'test:js:alpha',
      check_script: 'lint:js:alpha',
      go_packages: '',
    })
  }
  if (alphaChanged.length > 0) {
    alpha.push({
      name: 'lint-go',
      script: 'lint:go:alpha',
      check_script: '',
      go_packages: alphaChanged.join(' '),
    })
  }

  // Build the db stages that have work. The js-only Go tests cover every
  // product tree, so they take every selected package. The graph is the host
  // build's, so a delta run misses a js-only test whose dependency changed
  // only in js-tagged files; the hourly full run covers it.
  const dbTested = packageDirs(graph, tested, dbRoots)
  const dbChanged = packageDirs(graph, changed, dbRoots)
  const startupTrace = packageDirs(graph, tested, startupTraceRoots).length > 0
  const goJS = packageDirs(graph, tested, [...graph.dirs.keys()])
  const db: DbEntry[] = []
  if (startupTrace || dbChanged.length > 0) {
    db.push({
      ...dbChecks,
      startup_trace: startupTrace,
      lint_go: dbChanged.length > 0,
      lint_packages: dbChanged.join(' '),
    })
  }
  if (dbTested.length > 0 || goJS.length > 0 || js) {
    db.push({
      ...dbTests,
      test_go: dbTested.length > 0,
      go_packages: dbTested.join(' '),
      test_go_js: goJS.length > 0,
      go_js_packages: goJS.join(' '),
      js,
    })
  }

  return {
    mode: 'delta',
    members: memberEntries,
    alpha,
    db,
    lean:
      files.some((file) => leanInputs.test(file)) ||
      leanPackages.some((dir) => tested.has(graph.dirs.get(dir) ?? '')),
    python:
      files.some((file) => pythonInputs.test(file)) ||
      pythonPackages.some((dir) => tested.has(graph.dirs.get(dir) ?? '')),
    licenses: files.some((file) => licenseInputs.test(file)),
  }
}

// writeOutputs publishes the selection as job outputs. Each matrix output is
// a JSON matrix value, or empty when the job has no entries.
function writeOutputs(selection: Selection) {
  const matrix = (include: object[]) =>
    include.length > 0 ? JSON.stringify({ include }) : ''
  const outputs: Record<string, string> = {
    mode: selection.mode,
    members: matrix(selection.members),
    alpha: matrix(selection.alpha),
    db: matrix(selection.db),
    lean: String(selection.lean),
    python: String(selection.python),
    licenses: String(selection.licenses),
  }
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
  writeOutputs(fullSelection())
} else if (mode === 'delta' && base) {
  writeOutputs(deltaSelection(base))
} else {
  console.error('usage: ci-affected.ts full | delta <base-commit>')
  process.exit(2)
}
