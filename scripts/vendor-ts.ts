// vendor-ts fills vendor/ with the TypeScript of the Go modules the
// typecheck imports through @go/* and @goscript/*, without running Go.
//
// The module list is the import closure: the @go/* and @goscript/* imports
// of the files tsconfig.json includes, then those of each module's own
// TypeScript. Versions come from go.mod with its replace directives
// followed. Each module zip comes from the public Go module proxy and is
// checked against go.sum's h1 hash before anything is extracted; a module
// replaced by a local path is copied from that path. Only .ts files,
// .d.ts included, are written, at vendor/<module path>/..., the layout
// `go mod vendor` uses, so a later `go mod vendor` replaces the result.
//
// vendor/ is current when every module of the closure is at the version
// go.mod selects. A vendor/ written by `go mod vendor` (vendor/modules.txt
// present, no vendor/.ts-modules.json) is checked against modules.txt and
// never modified. A vendor/ written here records what it holds in
// vendor/.ts-modules.json; modules that differ are fetched again. Each
// module is staged under vendor/ and renamed into place, and the record is
// written last, so an interrupted run leaves a module either absent from
// the record or whole.
import { createHash } from 'node:crypto'
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { inflateRawSync } from 'node:zlib'

// recordName is the file under vendor/ that lists what this script wrote.
export const recordName = '.ts-modules.json'

// stagingPrefix names the temporary directories under vendor/.
const stagingPrefix = '.ts-staging-'

// proxyURL is the public Go module proxy.
const proxyURL = 'https://proxy.golang.org'

// ModuleSource is where a module's files come from: a module version on the
// proxy, or a local directory from a replace directive.
export type ModuleSource =
  | { kind: 'proxy'; path: string; version: string }
  | { kind: 'local'; dir: string }

// GoMod is what this script needs from go.mod.
export interface GoMod {
  module: string
  // require maps each required module path to its version.
  require: Map<string, string>
  replace: Replace[]
}

// Replace is one replace directive. version, when set, limits it to that
// version of the module.
export interface Replace {
  path: string
  version?: string
  to: ModuleSource
}

// RecordEntry is what vendor/.ts-modules.json holds for one module.
export interface RecordEntry {
  // source is sourceKey of the module's source.
  source: string
  // imports are the modules the module's TypeScript imports.
  imports: string[]
}

// Record is vendor/.ts-modules.json: module path to its entry.
export type Record = { [path: string]: RecordEntry }

// stripComment removes a // comment from one go.mod line.
function stripComment(line: string): string {
  const at = line.indexOf('//')
  return (at === -1 ? line : line.slice(0, at)).trim()
}

// isLocalPath reports whether a replace target is a directory.
function isLocalPath(target: string): boolean {
  return (
    target.startsWith('./') ||
    target.startsWith('../') ||
    target.startsWith('/')
  )
}

// parseReplace parses the body of one replace directive.
function parseReplace(body: string, modDir: string): Replace {
  const [left, right] = body.split('=>').map((part) => part.trim().split(/\s+/))
  if (!left?.[0] || !right?.[0]) {
    throw new Error(`go.mod: cannot parse replace "${body}"`)
  }
  const to: ModuleSource = isLocalPath(right[0])
    ? { kind: 'local', dir: resolve(modDir, right[0]) }
    : { kind: 'proxy', path: right[0], version: right[1] ?? '' }
  if (to.kind === 'proxy' && !to.version) {
    throw new Error(`go.mod: replace "${body}" names no version`)
  }
  return { path: left[0], version: left[1], to }
}

// parseGoMod reads the module path, requirements and replace directives
// from go.mod text. modDir resolves local replace paths.
export function parseGoMod(text: string, modDir: string): GoMod {
  const mod: GoMod = { module: '', require: new Map(), replace: [] }
  let block = ''
  for (const raw of text.split('\n')) {
    const line = stripComment(raw)
    if (!line) continue
    if (block) {
      if (line === ')') {
        block = ''
        continue
      }
      if (block === 'require') {
        const [path, version] = line.split(/\s+/)
        mod.require.set(path, version)
      } else if (block === 'replace') {
        mod.replace.push(parseReplace(line, modDir))
      }
      continue
    }
    const [verb, ...rest] = line.split(/\s+/)
    if (rest[0] === '(') {
      block = verb
      continue
    }
    if (verb === 'module') mod.module = rest[0]
    else if (verb === 'require') mod.require.set(rest[0], rest[1])
    else if (verb === 'replace')
      mod.replace.push(parseReplace(rest.join(' '), modDir))
  }
  if (!mod.module) throw new Error('go.mod: no module line')
  return mod
}

// parseGoSum maps "path version" to its h1 hash, skipping /go.mod lines.
export function parseGoSum(text: string): Map<string, string> {
  const sums = new Map<string, string>()
  for (const line of text.split('\n')) {
    const [path, version, hash] = line.trim().split(/\s+/)
    if (!hash || version.endsWith('/go.mod')) continue
    sums.set(`${path} ${version}`, hash)
  }
  return sums
}

// sourceOf returns where go.mod says the module's files come from.
export function sourceOf(mod: GoMod, path: string): ModuleSource {
  const version = mod.require.get(path)
  if (!version) {
    throw new Error(
      `${path} is imported by TypeScript but not required in go.mod`,
    )
  }
  const exact = mod.replace.find(
    (rep) => rep.path === path && rep.version === version,
  )
  const any = mod.replace.find((rep) => rep.path === path && !rep.version)
  return (exact ?? any)?.to ?? { kind: 'proxy', path, version }
}

// sourceKey is a source as one comparable string.
export function sourceKey(source: ModuleSource): string {
  return source.kind === 'local'
    ? `local:${source.dir}`
    : `${source.path}@${source.version}`
}

// moduleOf returns the required module that holds an import path: the
// longest module path that is the path or a parent of it.
export function moduleOf(mod: GoMod, importPath: string): string | null {
  let best: string | null = null
  for (const path of mod.require.keys()) {
    if (importPath !== path && !importPath.startsWith(path + '/')) continue
    if (!best || path.length > best.length) best = path
  }
  return best
}

// goscriptModule is the module @goscript/* maps into.
const goscriptModule = 'github.com/s4wave/goscript'

// importPattern matches the specifier of an import, export or dynamic
// import that starts with @go/ or @goscript/.
const importPattern =
  /(?:\bfrom|\bimport)\s*\(?\s*['"](@go(?:script)?\/[^'"]+)['"]/g

// goImports returns the Go import paths a TypeScript source names through
// @go/* and @goscript/*.
export function goImports(source: string): string[] {
  const paths: string[] = []
  for (const match of source.matchAll(importPattern)) {
    const spec = match[1]
    paths.push(
      spec.startsWith('@goscript/')
        ? goscriptModule + '/gs/' + spec.slice('@goscript/'.length)
        : spec.slice('@go/'.length),
    )
  }
  return paths
}

// modulesImported maps the @go/* and @goscript/* imports of sources to the
// required modules that hold them, leaving out the main module, which
// tsconfig maps to the checkout itself.
export function modulesImported(mod: GoMod, sources: string[]): Set<string> {
  const modules = new Set<string>()
  for (const source of sources) {
    for (const path of goImports(source)) {
      if (path === mod.module || path.startsWith(mod.module + '/')) continue
      // A path no required module holds is left for the typecheck to
      // report; the pattern also matches import text inside strings.
      const owner = moduleOf(mod, path)
      if (owner) modules.add(owner)
    }
  }
  return modules
}

// escapePath escapes a module path or version for the proxy: each upper
// case letter becomes ! and its lower case.
export function escapePath(path: string): string {
  return path.replace(/[A-Z]/g, (ch) => '!' + ch.toLowerCase())
}

// ZipFile is one file of a zip archive.
export interface ZipFile {
  name: string
  data: Uint8Array
}

// maxModuleBytes bounds a module's uncompressed size, as Go's module zip
// rules do.
export const maxModuleBytes = 500 << 20

// readZip returns the files of a zip archive, stored or deflated. It refuses
// zip64 archives, duplicate names, and archives whose declared sizes exceed
// maxTotal, before inflating anything; no entry inflates past its declared
// size.
export function readZip(zip: Uint8Array, maxTotal = maxModuleBytes): ZipFile[] {
  const view = new DataView(zip.buffer, zip.byteOffset, zip.byteLength)
  let end = zip.byteLength - 22
  while (end >= 0 && view.getUint32(end, true) !== 0x06054b50) end--
  if (end < 0) throw new Error('zip: no end of central directory')
  const count = view.getUint16(end + 10, true)
  let at = view.getUint32(end + 16, true)
  if (count === 0xffff || at === 0xffffffff) {
    throw new Error('zip: zip64 archives are not supported')
  }

  // Read the central directory and check the declared sizes first.
  const decoder = new TextDecoder()
  const entries: {
    name: string
    method: number
    size: number
    usize: number
    local: number
  }[] = []
  const names = new Set<string>()
  let total = 0
  for (let i = 0; i < count; i++) {
    if (view.getUint32(at, true) !== 0x02014b50) {
      throw new Error('zip: bad central directory entry')
    }
    const method = view.getUint16(at + 10, true)
    const size = view.getUint32(at + 20, true)
    const usize = view.getUint32(at + 24, true)
    const nameLen = view.getUint16(at + 28, true)
    const extraLen = view.getUint16(at + 30, true)
    const commentLen = view.getUint16(at + 32, true)
    const local = view.getUint32(at + 42, true)
    const name = decoder.decode(zip.subarray(at + 46, at + 46 + nameLen))
    at += 46 + nameLen + extraLen + commentLen
    if (size === 0xffffffff || usize === 0xffffffff || local === 0xffffffff) {
      throw new Error('zip: zip64 archives are not supported')
    }
    if (names.has(name)) throw new Error(`zip: duplicate entry ${name}`)
    names.add(name)
    total += usize
    if (total > maxTotal) {
      throw new Error(`zip: uncompressed size exceeds ${maxTotal} bytes`)
    }
    entries.push({ name, method, size, usize, local })
  }

  const files: ZipFile[] = []
  for (const { name, method, size, usize, local } of entries) {
    if (name.endsWith('/')) continue
    if (view.getUint32(local, true) !== 0x04034b50) {
      throw new Error(`zip: bad local header for ${name}`)
    }
    const start =
      local +
      30 +
      view.getUint16(local + 26, true) +
      view.getUint16(local + 28, true)
    const body = zip.subarray(start, start + size)
    let data: Uint8Array
    if (method === 0) data = body
    else if (method === 8) {
      try {
        data = inflateRawSync(body, { maxOutputLength: Math.max(usize, 1) })
      } catch (err) {
        throw new Error(
          `zip: ${name} does not inflate to its declared ${usize} bytes`,
          { cause: err },
        )
      }
    } else throw new Error(`zip: ${name} uses compression method ${method}`)
    if (data.byteLength !== usize) {
      throw new Error(`zip: ${name} is not its declared ${usize} bytes`)
    }
    files.push({ name, data })
  }
  return files
}

// hash1 is the Go dirhash Hash1 of a file list: the SHA-256 of the lines
// "<hex sha256 of file>  <name>\n" in name order, as "h1:" and base64.
export function hash1(files: ZipFile[]): string {
  const lines = [...files]
    .sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0))
    .map((file) => {
      if (file.name.includes('\n')) {
        throw new Error(`dirhash: file name with newline: ${file.name}`)
      }
      const sum = createHash('sha256').update(file.data).digest('hex')
      return `${sum}  ${file.name}\n`
    })
  return 'h1:' + createHash('sha256').update(lines.join('')).digest('base64')
}

// isSafeRelativePath reports whether a path within a module stays below the
// module's directory: not empty, not absolute, no backslash, and no empty,
// . or .. segment.
export function isSafeRelativePath(rest: string): boolean {
  if (rest.includes('\\')) return false
  return rest
    .split('/')
    .every((segment) => segment !== '' && segment !== '.' && segment !== '..')
}

// verifiedFiles checks a module zip against its go.sum hash and returns its
// .ts files by path within the module. A missing or different hash, or any
// entry outside the module's directory, throws naming the module before any
// file is returned.
export function verifiedFiles(
  zip: Uint8Array,
  path: string,
  version: string,
  sums: Map<string, string>,
): Map<string, Uint8Array> {
  const want = sums.get(`${path} ${version}`)
  if (!want) throw new Error(`go.sum has no hash for ${path} ${version}`)
  const files = readZip(zip)
  const got = hash1(files)
  if (got !== want) {
    throw new Error(
      `${path} ${version}: zip hash ${got} does not match go.sum ${want}`,
    )
  }
  const prefix = `${path}@${version}/`
  const ts = new Map<string, Uint8Array>()
  for (const file of files) {
    const rest = file.name.startsWith(prefix)
      ? file.name.slice(prefix.length)
      : null
    if (rest === null || !isSafeRelativePath(rest)) {
      throw new Error(`${path} ${version}: zip entry ${file.name} is unsafe`)
    }
    if (rest.endsWith('.ts')) ts.set(rest, file.data)
  }
  return ts
}

// localFiles returns the .ts files below dir, skipping node_modules and
// vendor directories.
function localFiles(dir: string): Map<string, Uint8Array> {
  const files = new Map<string, Uint8Array>()
  const walk = (at: string) => {
    for (const entry of readdirSync(at, { withFileTypes: true })) {
      const full = join(at, entry.name)
      if (entry.isDirectory()) {
        if (entry.name === 'node_modules' || entry.name === 'vendor') continue
        walk(full)
      } else if (entry.name.endsWith('.ts')) {
        files.set(relative(dir, full), readFileSync(full))
      }
    }
  }
  walk(dir)
  return files
}

// parseModulesTxt maps each module vendor/modules.txt lists to the source
// key of what `go mod vendor` copied.
export function parseModulesTxt(
  text: string,
  modDir: string,
): Map<string, string> {
  const vendored = new Map<string, string>()
  for (const line of text.split('\n')) {
    if (!line.startsWith('# ')) continue
    const [left, right] = line
      .slice(2)
      .split('=>')
      .map((part) => part.trim())
    const [path, version] = left.split(/\s+/)
    if (!right) {
      vendored.set(path, sourceKey({ kind: 'proxy', path, version }))
      continue
    }
    const [to, toVersion] = right.split(/\s+/)
    vendored.set(
      path,
      sourceKey(
        isLocalPath(to)
          ? { kind: 'local', dir: resolve(modDir, to) }
          : { kind: 'proxy', path: to, version: toVersion },
      ),
    )
  }
  return vendored
}

// staleModules returns the modules of wanted whose vendored source differs
// from the one go.mod selects; a module vendored nowhere is stale.
export function staleModules(
  mod: GoMod,
  wanted: Iterable<string>,
  vendored: Map<string, string>,
): string[] {
  const stale: string[] = []
  for (const path of wanted) {
    if (vendored.get(path) !== sourceKey(sourceOf(mod, path))) stale.push(path)
  }
  return stale.sort()
}

// Fetch downloads one URL; tests replace it.
export type Fetch = (url: string) => Promise<Uint8Array>

// fetchBytes downloads one URL from the proxy.
async function fetchBytes(url: string): Promise<Uint8Array> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`GET ${url}: ${res.status} ${res.statusText}`)
  return new Uint8Array(await res.arrayBuffer())
}

// moduleFiles returns the verified .ts files of a module from its source.
async function moduleFiles(
  path: string,
  source: ModuleSource,
  sums: Map<string, string>,
  get: Fetch,
): Promise<{ files: Map<string, Uint8Array>; bytes: number }> {
  if (source.kind === 'local')
    return { files: localFiles(source.dir), bytes: 0 }
  const url = `${proxyURL}/${escapePath(source.path)}/@v/${escapePath(source.version)}.zip`
  let zip: Uint8Array
  try {
    zip = await get(url)
  } catch (err) {
    const reason = err instanceof Error ? err.message : String(err)
    throw new Error(
      `vendor/ lacks ${path} ${source.version} and cannot fetch it (${reason}); run bun run typecheck once online, or bun run prepare where Go runs`,
      { cause: err },
    )
  }
  return {
    files: verifiedFiles(zip, source.path, source.version, sums),
    bytes: zip.byteLength,
  }
}

// install stages files under vendor/ and renames them into place as
// vendor/<path>, replacing what was there.
function install(
  vendorDir: string,
  path: string,
  files: Map<string, Uint8Array>,
) {
  const staging = join(
    vendorDir,
    stagingPrefix +
      createHash('sha256').update(path).digest('hex').slice(0, 12),
  )
  rmSync(staging, { recursive: true, force: true })
  for (const [name, data] of files) {
    const out = join(staging, name)
    mkdirSync(dirname(out), { recursive: true })
    writeFileSync(out, data)
  }
  mkdirSync(staging, { recursive: true })
  const target = join(vendorDir, path)
  const old = staging + '-old'
  rmSync(old, { recursive: true, force: true })
  if (existsSync(target)) renameSync(target, old)
  mkdirSync(dirname(target), { recursive: true })
  renameSync(staging, target)
  rmSync(old, { recursive: true, force: true })
}

// readRecord reads vendor/.ts-modules.json, or null when absent.
function readRecord(vendorDir: string): Record | null {
  const file = join(vendorDir, recordName)
  if (!existsSync(file)) return null
  return JSON.parse(readFileSync(file, 'utf8')) as Record
}

// writeRecord replaces vendor/.ts-modules.json in one rename.
function writeRecord(vendorDir: string, record: Record) {
  const file = join(vendorDir, recordName)
  writeFileSync(file + '.tmp', JSON.stringify(record, null, 2) + '\n')
  renameSync(file + '.tmp', file)
}

// SyncResult reports one sync.
export interface SyncResult {
  fetched: string[]
  bytes: number
  // warning names a stale vendor/ that `go mod vendor` produced. The sync
  // leaves such a tree untouched, since Go owns it, and the type check runs
  // against it as it stands.
  warning?: string
}

// syncVendor brings vendor/ up to the import closure of sources. It
// throws, naming what to run, when a module cannot be fetched or verified.
// A vendor/ that `go mod vendor` produced is never written; when it is stale
// the result carries a warning instead.
export async function syncVendor(opts: {
  root: string
  mod: GoMod
  sums: Map<string, string>
  sources: string[]
  get?: Fetch
}): Promise<SyncResult> {
  const { root, mod, sums } = opts
  const get = opts.get ?? fetchBytes
  const vendorDir = join(root, 'vendor')
  const wanted = modulesImported(mod, opts.sources)

  const modulesTxt = join(vendorDir, 'modules.txt')
  let record = readRecord(vendorDir)
  if (!record && existsSync(modulesTxt)) {
    const vendored = parseModulesTxt(readFileSync(modulesTxt, 'utf8'), root)
    // go mod vendor copies every package the Go build imports, so the
    // closure is the TypeScript imports of the vendored modules too.
    const queue = [...wanted]
    for (let path = queue.pop(); path; path = queue.pop()) {
      const dir = join(vendorDir, path)
      if (!existsSync(dir)) continue
      for (const next of modulesImported(
        mod,
        [...localFiles(dir).values()].map(String),
      )) {
        if (!wanted.has(next)) {
          wanted.add(next)
          queue.push(next)
        }
      }
    }
    const stale = staleModules(mod, wanted, vendored)
    if (!stale.length) return { fetched: [], bytes: 0 }
    return {
      fetched: [],
      bytes: 0,
      warning: `${vendorDir} came from go mod vendor and is stale for ${stale.join(', ')}; run bun run prepare where Go runs, or remove vendor/ to fetch TypeScript only`,
    }
  }

  record ??= {}
  for (const name of readdirSyncSafe(vendorDir)) {
    if (name.startsWith(stagingPrefix)) {
      rmSync(join(vendorDir, name), { recursive: true, force: true })
    }
  }

  const result: SyncResult = { fetched: [], bytes: 0 }
  const next: Record = {}
  const queue = [...wanted]
  const seen = new Set(queue)
  for (let path = queue.shift(); path; path = queue.shift()) {
    const source = sourceOf(mod, path)
    const key = sourceKey(source)
    let entry = record[path]
    // A recorded module whose directory is gone, deleted or caught between
    // install's renames, is fetched again.
    if (
      !entry ||
      entry.source !== key ||
      source.kind === 'local' ||
      !existsSync(join(vendorDir, path))
    ) {
      const { files, bytes } = await moduleFiles(path, source, sums, get)
      mkdirSync(vendorDir, { recursive: true })
      install(vendorDir, path, files)
      entry = {
        source: key,
        imports: [
          ...modulesImported(
            mod,
            [...files.values()].map((data) => new TextDecoder().decode(data)),
          ),
        ]
          .filter((dep) => dep !== path)
          .sort(),
      }
      record[path] = entry
      writeRecord(vendorDir, record)
      result.fetched.push(path)
      result.bytes += bytes
    }
    next[path] = entry
    for (const dep of entry.imports) {
      if (!seen.has(dep)) {
        seen.add(dep)
        queue.push(dep)
      }
    }
  }
  if (
    result.fetched.length ||
    Object.keys(record).length !== Object.keys(next).length
  ) {
    mkdirSync(vendorDir, { recursive: true })
    writeRecord(vendorDir, next)
  }
  return result
}

// readdirSyncSafe lists a directory, or nothing when it is absent.
function readdirSyncSafe(dir: string): string[] {
  return existsSync(dir) ? readdirSync(dir) : []
}

// includedSources reads the files tsconfig.json includes, less its
// excludes.
function includedSources(root: string): string[] {
  const config = JSON.parse(
    readFileSync(join(root, 'tsconfig.json'), 'utf8'),
  ) as {
    include: string[]
    exclude?: string[]
  }
  const excluded = (config.exclude ?? []).map(
    (pattern) =>
      new Bun.Glob(pattern.includes('*') ? pattern : pattern + '/**'),
  )
  const files = new Set<string>()
  for (const pattern of config.include) {
    for (const file of new Bun.Glob(pattern).scanSync({ cwd: root })) {
      if (!/\.(ts|tsx|mts|cts)$/.test(file)) continue
      if (file.includes('node_modules/')) continue
      if (excluded.some((glob) => glob.match(file))) continue
      files.add(file)
    }
  }
  return [...files].map((file) => readFileSync(join(root, file), 'utf8'))
}

async function main() {
  const root = resolve(import.meta.dir, '..')
  const started = performance.now()
  try {
    const mod = parseGoMod(readFileSync(join(root, 'go.mod'), 'utf8'), root)
    const sums = parseGoSum(readFileSync(join(root, 'go.sum'), 'utf8'))
    const result = await syncVendor({
      root,
      mod,
      sums,
      sources: includedSources(root),
    })
    if (result.warning) console.warn(`vendor-ts: warning: ${result.warning}`)
    if (result.fetched.length) {
      const seconds = ((performance.now() - started) / 1000).toFixed(1)
      console.log(
        `vendor-ts: fetched ${result.fetched.length} modules, ${result.bytes} bytes, in ${seconds} s`,
      )
    }
  } catch (err) {
    console.error(
      `vendor-ts: ${err instanceof Error ? err.message : String(err)}`,
    )
    process.exit(1)
  }
}

if (import.meta.main) await main()
