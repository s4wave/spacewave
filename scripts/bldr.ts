import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { join } from 'node:path'

// Run the bldr tool. Uses the prebuilt .tools/bldr-bin written by scripts/prepare.ts
// when present, falling back to `go run` so a fresh checkout works before prepare.
const args = process.argv.slice(2)
const bin = join(process.cwd(), '.tools', 'bldr-bin')

if (existsSync(bin)) {
  const result = spawnSync(bin, ['--bldr-src-path=../..', ...args], {
    cwd: process.cwd(),
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    throw new Error(`bldr-bin ${args.join(' ')} exited ${result.status}`)
  }
} else {
  const result = spawnSync('bun', ['run', 'go:run', '--', 'github.com/s4wave/spacewave/bldr/cmd/bldr', '--bldr-src-path=../..', ...args], {
    cwd: process.cwd(),
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    throw new Error(`go run bldr ${args.join(' ')} exited ${result.status}`)
  }
}
