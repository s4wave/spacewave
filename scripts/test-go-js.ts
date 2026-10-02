#!/usr/bin/env bun

// test-go-js runs the Go tests that only build for js/wasm in a headless
// browser. It selects every package under the given patterns (default ./...)
// with a test file that builds for GOOS=js but not for the host, so a new
// js-tagged test package joins the run without a list to update. Prototype
// packages are experiments, not product, and stay out of the run. Running a js
// test binary needs wasmbrowsertest installed as go_js_wasm_exec on PATH.

import { mkdtempSync, rmSync } from 'node:fs'
import { availableParallelism, tmpdir } from 'node:os'
import { basename, join } from 'node:path'

const jsEnv = { ...process.env, GOOS: 'js', GOARCH: 'wasm' }

// goList runs go list with a template and returns its output lines.
function goList(
  template: string,
  patterns: string[],
  env: Record<string, string | undefined>,
): string[] {
  const proc = Bun.spawnSync(
    ['go', 'list', '-e', '-f', template, ...patterns],
    { env, stderr: 'inherit' },
  )
  if (proc.exitCode !== 0) {
    process.exit(proc.exitCode ?? 1)
  }
  return proc.stdout
    .toString()
    .split('\n')
    .filter((line) => line.trim() !== '')
}

// listTestFiles maps each package to its test files for the given environment.
function listTestFiles(
  patterns: string[],
  env: Record<string, string | undefined>,
): Map<string, Set<string>> {
  const files = new Map<string, Set<string>>()
  const template =
    '{{.ImportPath}} {{join .TestGoFiles " "}} {{join .XTestGoFiles " "}}'
  for (const line of goList(template, patterns, env)) {
    const [pkg, ...names] = line.trim().split(/\s+/)
    files.set(pkg, new Set(names))
  }
  return files
}

// selectJSPackages returns the packages with a test file only js builds.
function selectJSPackages(patterns: string[]): string[] {
  const hostFiles = listTestFiles(patterns, process.env)
  const jsFiles = listTestFiles(patterns, jsEnv)

  const pkgs: string[] = []
  for (const [pkg, names] of jsFiles) {
    if (pkg.includes('/prototypes/')) {
      continue
    }
    const host = hostFiles.get(pkg)
    if ([...names].some((name) => !host?.has(name))) {
      pkgs.push(pkg)
    }
  }
  return pkgs.sort()
}

// TestBinary is one package's compiled test binary.
interface TestBinary {
  pkg: string
  dir: string
  path: string
}

// buildTestBinaries compiles every package's test binary into binDir. go test
// -c names each binary after its package's last path element, so packages
// sharing that name build in separate invocations. A package whose build fails
// has no binary; go test has already reported the error.
function buildTestBinaries(pkgs: string[], binDir: string): TestBinary[] {
  const dirs = new Map(
    goList('{{.ImportPath}} {{.Dir}}', pkgs, jsEnv).map((line) => {
      const [pkg, dir] = line.split(' ', 2)
      return [pkg, dir] as const
    }),
  )

  // Place each package in the first group without its name.
  const groups: Map<string, string>[] = []
  for (const pkg of pkgs) {
    const name = basename(pkg)
    let group = groups.find((g) => !g.has(name))
    if (!group) {
      group = new Map()
      groups.push(group)
    }
    group.set(name, pkg)
  }

  const bins: TestBinary[] = []
  for (const [i, group] of groups.entries()) {
    const outDir = join(binDir, String(i))
    Bun.spawnSync(['go', 'test', '-c', '-o', outDir + '/', ...group.values()], {
      env: jsEnv,
      stdout: 'inherit',
      stderr: 'inherit',
    })
    for (const [name, pkg] of group) {
      const path = join(outDir, name + '.test')
      if (Bun.file(path).size > 0) {
        bins.push({ pkg, dir: dirs.get(pkg) ?? '.', path })
      }
    }
  }
  return bins
}

// runTestBinary runs one binary in its package directory, as go test does, and
// prints go test's summary line. It returns whether the tests passed.
async function runTestBinary(bin: TestBinary): Promise<boolean> {
  const start = performance.now()
  const proc = Bun.spawn(
    ['go_js_wasm_exec', bin.path, '-test.paniconexit0', '-test.timeout=110s'],
    { cwd: bin.dir, env: jsEnv, stdout: 'pipe', stderr: 'pipe' },
  )
  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ])
  const elapsed = ((performance.now() - start) / 1000).toFixed(3)
  if (code === 0) {
    console.log(`ok  \t${bin.pkg}\t${elapsed}s`)
    return true
  }
  process.stdout.write(stdout + stderr)
  console.log(`FAIL\t${bin.pkg}\t${elapsed}s`)
  return false
}

const args = process.argv.slice(2)
const listOnly = args[0] === '--list'
const patterns = listOnly ? args.slice(1) : args
const pkgs = selectJSPackages(patterns.length > 0 ? patterns : ['./...'])
if (pkgs.length === 0) {
  console.error('test-go-js: no js-only test packages found')
  process.exit(1)
}
if (listOnly) {
  console.log(pkgs.join('\n'))
  process.exit(0)
}

// Build every test binary before any test starts. Each test binary launches
// Chrome, which must report its DevTools endpoint within wasmbrowsertest's fixed
// 20 second limit; compiling or linking other packages beside it starves the
// launch. go test relinks a binary on every run, so the binaries run directly.
const binDir = mkdtempSync(join(tmpdir(), 'test-go-js-'))
let ok: boolean
try {
  const bins = buildTestBinaries(pkgs, binDir)
  ok = bins.length === pkgs.length

  // Run up to one binary per CPU, as go test -p does.
  const queue = [...bins]
  const workers = Array.from(
    { length: Math.min(availableParallelism(), queue.length) },
    async () => {
      for (let bin = queue.shift(); bin; bin = queue.shift()) {
        ok = (await runTestBinary(bin)) && ok
      }
    },
  )
  await Promise.all(workers)
} finally {
  rmSync(binDir, { recursive: true, force: true })
}
if (!ok) {
  console.log('FAIL')
}
process.exit(ok ? 0 : 1)
