import { readFile } from 'fs/promises'
import { resolve } from 'path'

const root = resolve(import.meta.dirname, '..')

// protoClosure returns the root protos and every proto of this module they
// import, directly or through another, as paths relative to the repository
// root in sorted order. Imports from other modules are left to the packages
// that ship their bindings.
async function protoClosure(roots: string[]): Promise<string[]> {
  // The module line of go.mod is the prefix of this module's proto imports.
  const goMod = await readFile(resolve(root, 'go.mod'), 'utf-8')
  const modulePath = goMod.match(/^module\s+(\S+)/m)?.[1]
  if (!modulePath) {
    throw new Error('go.mod has no module line')
  }
  const prefix = modulePath.replaceAll('.', '\\.')
  const importRe = new RegExp(
    `^import\\s+(?:public\\s+|weak\\s+)?"${prefix}/([^"]+)"`,
    'gm',
  )

  // Walk the imports from each root, visiting each proto once.
  const seen = new Set<string>()
  const pending = [...roots]
  for (let path = pending.pop(); path !== undefined; path = pending.pop()) {
    if (seen.has(path)) continue
    seen.add(path)
    const source = await readFile(resolve(root, path), 'utf-8')
    for (const match of source.matchAll(importRe)) {
      pending.push(match[1])
    }
  }
  return [...seen].sort()
}

// The command runs aptre generate on the import closure of its --root protos,
// so a generation script selects only its roots and an imported proto cannot be
// left out. Every other argument passes through to aptre generate:
//
//   bun scripts/gen-closure.ts --root=<a.proto>... [generate flags]
if (import.meta.main) {
  // Split the root protos from the flags meant for aptre generate.
  const args = process.argv.slice(2)
  const roots = args
    .filter((arg) => arg.startsWith('--root='))
    .map((arg) => arg.slice('--root='.length))
  if (!roots.length) {
    throw new Error(
      'usage: bun scripts/gen-closure.ts --root=<a.proto>... [generate flags]',
    )
  }
  const flags = args.filter((arg) => !arg.startsWith('--root='))

  // Run aptre generate on the closure, exiting with its status.
  const closure = await protoClosure(roots)
  const targets = closure.map((path) => `--targets=${path}`)
  const generate = [process.execPath, 'run', 'go:aptre', '--', 'generate']
  const proc = Bun.spawn([...generate, ...targets, ...flags], {
    cwd: root,
    stdio: ['inherit', 'inherit', 'inherit'],
  })
  process.exit(await proc.exited)
}
