// check-dist-closure checks, without a Go build, that the TypeScript sources
// each dist.go embeds are closed: every local import of an embedded source
// resolves to a source that a dist.go also embeds. It mirrors
// TestDistSourcesAreClosed in web/dist_test.go, which checks the compiled
// embed.FS in CI; this copy runs in the commit hook, so a new source that an
// embedded file imports cannot be committed without its go:embed line.
//
// Run it from the repository root: bun .githooks/check-dist-closure.ts

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { posix } from 'node:path'

// distRoots maps each embed.FS's import prefix to the directory of the
// dist.go that declares it. The root FS serves every other @s4wave/ path.
const distRoots: { prefix: string; dir: string }[] = [
  { prefix: 'bldr/', dir: 'bldr' },
  { prefix: 'web/', dir: 'web' },
  { prefix: '', dir: '.' },
]

const importPattern =
  /(?:\bfrom\s*|\bimport\s*(?:\(\s*)?|@import\s*)["']([^"']+)["']/gm

// isSource reports whether name is a TypeScript or CSS source.
function isSource(name: string): boolean {
  return /\.(ts|tsx|css)$/.test(name)
}

// isTestSource reports whether name is a test source.
function isTestSource(name: string): boolean {
  return /\.test\.tsx?$/.test(name)
}

// walkFiles lists the files under dir, relative to base, skipping names
// that start with . or _ unless all is set, as go:embed does for a directory.
function walkFiles(base: string, dir: string, all: boolean, out: Set<string>) {
  for (const entry of readdirSync(posix.join(base, dir), {
    withFileTypes: true,
  })) {
    if (!all && /^[._]/.test(entry.name)) {
      continue
    }
    const rel = posix.join(dir, entry.name)
    if (entry.isDirectory()) {
      walkFiles(base, rel, all, out)
    } else {
      out.add(rel)
    }
  }
}

// embeddedFiles expands the go:embed patterns of dir/dist.go into the files
// they embed, relative to dir.
function embeddedFiles(dir: string): Set<string> {
  const source = readFileSync(posix.join(dir, 'dist.go'), 'utf8')
  const files = new Set<string>()
  for (const line of source.split('\n')) {
    if (!line.startsWith('//go:embed ')) {
      continue
    }
    for (let pattern of line.slice('//go:embed '.length).trim().split(/\s+/)) {
      const all = pattern.startsWith('all:')
      if (all) {
        pattern = pattern.slice('all:'.length)
      }
      const matches = [
        ...new Bun.Glob(pattern).scanSync({
          cwd: dir,
          onlyFiles: false,
          dot: true,
        }),
      ]
      if (matches.length === 0) {
        throw new Error(`${dir}/dist.go: pattern ${pattern} matches no file`)
      }
      for (const match of matches) {
        if (statSync(posix.join(dir, match)).isDirectory()) {
          walkFiles(dir, match, all, files)
        } else {
          files.add(match)
        }
      }
    }
  }
  return files
}

// owner returns the embed root and the path within it of a repository path.
function owner(target: string): { dir: string; name: string } {
  for (const root of distRoots) {
    if (root.prefix === '' || target.startsWith(root.prefix)) {
      return { dir: root.dir, name: target.slice(root.prefix.length) }
    }
  }
  throw new Error('unreachable')
}

// importTarget resolves an import of source to a repository path, or null
// for a package outside this repository.
function importTarget(source: string, spec: string): string | null {
  if (spec.startsWith('@s4wave/')) {
    return spec.slice('@s4wave/'.length)
  }
  if (spec.startsWith('.')) {
    return posix.normalize(posix.join(posix.dirname(source), spec))
  }
  return null
}

// candidates lists the files an import of target may resolve to.
function candidates(target: string): string[] {
  const ext = posix.extname(target)
  if (ext === '.js') {
    const base = target.slice(0, -3)
    return [base + '.ts', base + '.tsx']
  }
  if (ext === '') {
    return [
      target + '.ts',
      target + '.tsx',
      posix.join(target, 'index.ts'),
      posix.join(target, 'index.tsx'),
    ]
  }
  return [target]
}

const embedded = new Map(
  distRoots.map((root) => [root.dir, embeddedFiles(root.dir)]),
)
const failures: string[] = []
for (const root of distRoots) {
  if (root.dir === 'bldr') {
    continue
  }
  for (const name of embedded.get(root.dir)!) {
    if (!isSource(name)) {
      continue
    }
    const source = posix.join(root.prefix, name)
    if (isTestSource(name)) {
      failures.push(`${root.dir}/dist.go embeds test source ${source}`)
      continue
    }
    const text = readFileSync(posix.join(root.dir, name), 'utf8')
    for (const match of text.matchAll(importPattern)) {
      const target = importTarget(source, match[1])
      if (target === null) {
        continue
      }
      const found = candidates(target).some((candidate) => {
        const { dir, name } = owner(candidate)
        return embedded.get(dir)!.has(name)
      })
      if (!found) {
        failures.push(
          `${source} imports ${match[1]}, which is not embedded; add it to the go:embed list in the dist.go that owns its directory`,
        )
      }
    }
  }
}
if (failures.length > 0) {
  console.error(failures.join('\n'))
  process.exit(1)
}
