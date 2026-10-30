import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { delimiter, dirname, isAbsolute, join, resolve, sep } from 'node:path'

// GoModulePin is one module's selected version or local replacement directory.
interface GoModulePin {
  version?: string
  dir?: string
}

const pinsByMod = new Map<string, Map<string, GoModulePin>>()

// goModCacheDir is the module cache Go uses when GOMODCACHE is unset.
function goModCacheDir(): string {
  if (process.env.GOMODCACHE) return process.env.GOMODCACHE
  const gopath = (process.env.GOPATH || join(homedir(), 'go')).split(
    delimiter,
  )[0]
  return join(gopath, 'pkg', 'mod')
}

// isGoModuleCachePath reports whether path lies inside the Go module cache.
export function isGoModuleCachePath(path: string): boolean {
  return resolve(path).startsWith(resolve(goModCacheDir()) + sep)
}

// encodeModulePath escapes uppercase letters the way the module cache does.
function encodeModulePath(path: string): string {
  return path.replace(/[A-Z]/g, (letter) => '!' + letter.toLowerCase())
}

// findGoMod returns the go.mod above dir, or null when none exists.
function findGoMod(dir: string): string | null {
  let cur = resolve(dir)
  for (;;) {
    const candidate = join(cur, 'go.mod')
    try {
      readFileSync(candidate)
      return candidate
    } catch {
      const parent = dirname(cur)
      if (parent === cur) return null
      cur = parent
    }
  }
}

// parseGoMod reads require and replace pins from one go.mod.
function parseGoMod(modPath: string): Map<string, GoModulePin> {
  // Reuse the selected pins when this module file was read already.
  const cached = pinsByMod.get(modPath)
  if (cached) return cached

  // Read the module file once and index each selected pin.
  const text = readFileSync(modPath, 'utf8')
  const root = dirname(modPath)
  const pins = new Map<string, GoModulePin>()
  let section: '' | 'require' | 'replace' = ''
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\/\/.*$/, '').trim()
    if (!line) continue
    if (line === 'require (') {
      section = 'require'
      continue
    }
    if (line === 'replace (') {
      section = 'replace'
      continue
    }
    if (line === ')') {
      section = ''
      continue
    }

    // Record a required version, including a single-line require.
    if (line.startsWith('require ') || section === 'require') {
      const body = line.startsWith('require ')
        ? line.slice('require '.length)
        : line
      const [modulePath, version] = body.split(/\s+/)
      if (modulePath && version) pins.set(modulePath, { version })
      continue
    }

    // A replace selects a directory or another module version.
    if (line.startsWith('replace ') || section === 'replace') {
      const body = line.startsWith('replace ')
        ? line.slice('replace '.length)
        : line
      const parts = body.split('=>')
      if (parts.length !== 2) continue
      const modulePath = parts[0].trim().split(/\s+/)[0]
      const right = parts[1].trim().split(/\s+/)
      if (!modulePath || right.length === 0) continue
      if (right.length === 1) {
        const dir = isAbsolute(right[0]) ? right[0] : resolve(root, right[0])
        pins.set(modulePath, { dir })
      } else {
        pins.set(modulePath, { version: right[right.length - 1] })
      }
    }
  }
  pinsByMod.set(modPath, pins)
  return pins
}

// resolveGoModuleCachePath returns the replacement or module-cache source of
// an @go import path, without the @go/ prefix, when vendor has no copy.
export function resolveGoModuleCachePath(
  roots: string[],
  importPath: string,
): string | null {
  // Merge pins from each module that contains one of the roots.
  const pins = new Map<string, GoModulePin>()
  for (const root of roots) {
    if (!root) continue
    const modPath = findGoMod(root)
    if (!modPath) continue
    for (const [modulePath, pin] of parseGoMod(modPath)) {
      if (!pins.has(modulePath)) pins.set(modulePath, pin)
    }
  }

  // Select the longest module path that prefixes the import.
  let best = ''
  let pin: GoModulePin | undefined
  for (const [modulePath, candidate] of pins) {
    const matches =
      importPath === modulePath || importPath.startsWith(modulePath + '/')
    if (matches && modulePath.length > best.length) {
      best = modulePath
      pin = candidate
    }
  }
  if (!pin) return null

  // Join the remainder onto the replacement directory or the module cache.
  const rest = importPath.slice(best.length).replace(/^\//, '')
  if (pin.dir) return join(pin.dir, rest)
  if (!pin.version) return null
  return join(
    goModCacheDir(),
    encodeModulePath(best) + '@' + encodeModulePath(pin.version),
    rest,
  )
}
