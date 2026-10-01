#!/usr/bin/env bun

// test-go-js runs the Go tests that only build for js/wasm in a headless
// browser. It selects every package under the given patterns (default ./db/...)
// with a test file that builds for GOOS=js but not for the host, so a new
// js-tagged test package joins the run without a list to update. Running a js
// test binary needs wasmbrowsertest installed as go_js_wasm_exec on PATH.

const jsEnv = { ...process.env, GOOS: 'js', GOARCH: 'wasm' }

// listTestFiles maps each package to its test files for the given environment.
function listTestFiles(
  patterns: string[],
  env: Record<string, string | undefined>,
): Map<string, Set<string>> {
  const proc = Bun.spawnSync(
    [
      'go',
      'list',
      '-e',
      '-f',
      '{{.ImportPath}} {{join .TestGoFiles " "}} {{join .XTestGoFiles " "}}',
      ...patterns,
    ],
    { env, stderr: 'inherit' },
  )
  if (proc.exitCode !== 0) {
    process.exit(proc.exitCode ?? 1)
  }

  const files = new Map<string, Set<string>>()
  for (const line of proc.stdout.toString().split('\n')) {
    const [pkg, ...names] = line.trim().split(/\s+/)
    if (pkg) {
      files.set(pkg, new Set(names))
    }
  }
  return files
}

// selectJSPackages returns the packages with a test file only js builds.
function selectJSPackages(patterns: string[]): string[] {
  const hostFiles = listTestFiles(patterns, process.env)
  const jsFiles = listTestFiles(patterns, jsEnv)

  const pkgs: string[] = []
  for (const [pkg, names] of jsFiles) {
    const host = hostFiles.get(pkg)
    if ([...names].some((name) => !host?.has(name))) {
      pkgs.push(pkg)
    }
  }
  return pkgs.sort()
}

const args = process.argv.slice(2)
const listOnly = args[0] === '--list'
const patterns = listOnly ? args.slice(1) : args
const pkgs = selectJSPackages(patterns.length > 0 ? patterns : ['./db/...'])
if (pkgs.length === 0) {
  console.error('test-go-js: no js-only test packages found')
  process.exit(1)
}
if (listOnly) {
  console.log(pkgs.join('\n'))
  process.exit(0)
}

const proc = Bun.spawnSync(
  ['go', 'test', '-count=1', '-timeout=110s', ...pkgs],
  { env: jsEnv, stdout: 'inherit', stderr: 'inherit' },
)
process.exit(proc.exitCode ?? 1)
