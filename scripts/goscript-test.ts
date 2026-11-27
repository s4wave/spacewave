import { spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')

// goscriptShards assigns every package tested under GoScript to the CI job
// that runs it, balanced by run time. It is the only list of these packages:
// ci-affected.ts adds one member job per shard and selects the shards when a
// listed package's tests are affected.
export const goscriptShards: Record<string, readonly string[]> = {
  a: [
    './core/space/world',
    './core/resource/root/controller',
    './core/space/sobject',
    './core/session/controller',
    './core/space/http/export',
  ],
  b: [
    './core/provider/local',
    './core/provider/spacewave',
    './core/plugin/space',
    './core/space/world/optypes',
    'github.com/s4wave/spacewave/db/object/peer',
    './bldr/manifest',
    './net/link/solicit',
    './core/resource/listener',
    './core/space/http/download',
    './core/space/world/blocktype',
  ],
}

// packageTimeout bounds one package's compile and run.
const packageTimeout = '8m'

// PackageResult is the outcome of one package's goscript test run.
interface PackageResult {
  pkg: string
  ok: boolean
  seconds: number
}

// testPackage runs goscript test on one package in a fresh scratch directory
// and removes the scratch directory afterwards. The work directory sits inside
// the scratch beside a link to the checkout's node_modules, which its tsconfig
// and the emitted code resolve by walking up. goscript writes its runtime
// module shims to the work directory's own node_modules, so they never reach
// the checkout.
function testPackage(pkg: string): PackageResult {
  const scratch = mkdtempSync(join(tmpdir(), 'goscript-test-'))
  const workDir = join(scratch, 'work')
  const start = performance.now()
  try {
    symlinkSync(join(root, 'node_modules'), join(scratch, 'node_modules'))
    mkdirSync(workDir)
    const args = [
      'tool',
      'github.com/s4wave/goscript/cmd/goscript',
      'test',
      '--dir',
      '.',
      '--workdir',
      workDir,
      '--timeout',
      packageTimeout,
      '-p',
      '1',
      '--tags',
      'skip_e2e,purego,goscript',
      pkg,
    ]
    const result = spawnSync('go', args, {
      cwd: root,
      env: { ...process.env, GOMEMLIMIT: '12GiB' },
      stdio: 'inherit',
    })
    return {
      pkg,
      ok: result.status === 0,
      seconds: (performance.now() - start) / 1000,
    }
  } finally {
    rmSync(scratch, { recursive: true, force: true })
  }
}

// resultLine formats one package's outcome and seconds.
function resultLine({ pkg, ok, seconds }: PackageResult): string {
  return `goscript-test: ${ok ? 'ok  ' : 'FAIL'} ${pkg} ${seconds.toFixed(1)}s`
}

// The command tests the packages of one shard, each under its own deadline,
// and keeps going after a failure so every package reports. It exits 1 when
// any package failed. Naming packages runs only those. Each must belong to
// the shard.
//
//   bun scripts/goscript-test.ts <shard> [package...]
if (import.meta.main) {
  // Select the shard's packages, rejecting a name the shard does not list.
  const [shard, ...named] = process.argv.slice(2)
  const packages = shard ? goscriptShards[shard] : undefined
  if (!packages) {
    const shards = Object.keys(goscriptShards).join(' | ')
    console.error(
      `usage: bun scripts/goscript-test.ts <${shards}> [package...]`,
    )
    process.exit(2)
  }
  const stray = named.filter((pkg) => !packages.includes(pkg))
  if (stray.length) {
    console.error(`goscript-test: not in shard ${shard}: ${stray.join(' ')}`)
    process.exit(2)
  }

  // Test each package, printing its result as it finishes.
  const results: PackageResult[] = []
  for (const pkg of named.length ? named : packages) {
    console.log(`goscript-test: start ${pkg}`)
    const result = testPackage(pkg)
    console.log(resultLine(result))
    results.push(result)
  }

  // Repeat every result together, then fail if any package failed.
  const total = results.reduce((sum, result) => sum + result.seconds, 0)
  console.log(`goscript-test: shard ${shard}, ${total.toFixed(1)}s`)
  results.forEach((result) => console.log(resultLine(result)))
  if (results.some((result) => !result.ok)) {
    process.exit(1)
  }
}
