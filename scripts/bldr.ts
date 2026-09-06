import { spawnSync } from 'node:child_process'

// Go's build cache tracks source and dependency changes, including GoScript.
// A prebuilt binary can silently compile releases with an obsolete runtime.
const args = process.argv.slice(2)
const result = spawnSync(
  'bun',
  [
    'run',
    'go:run',
    '--',
    'github.com/s4wave/spacewave/bldr/cmd/bldr',
    '--bldr-src-path=../..',
    ...args,
  ],
  { cwd: process.cwd(), stdio: 'inherit' },
)
if (result.status !== 0) {
  throw new Error(`go run bldr ${args.join(' ')} exited ${result.status}`)
}
