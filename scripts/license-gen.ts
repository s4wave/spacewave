#!/usr/bin/env bun

// Generates Go and JS license data, merges into unified licenses.json.
//
// Usage: bun scripts/license-gen.ts [--go] [--js] [--merge]
// No flags = generate both + merge.

import { spawn } from 'child_process'
import {
  existsSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  rmSync,
} from 'fs'
import { dirname, join, resolve } from 'path'
import { fileURLToPath } from 'url'
import { build } from 'vite'

const __dirname = dirname(fileURLToPath(import.meta.url))
const rootDir = join(__dirname, '..')
const outDir = join(rootDir, 'app', 'licenses')
const tmpDir = join(rootDir, '.tmp', 'license-build')

const args = process.argv.slice(2)
const explicit = args.some((a) => a.startsWith('--'))
const doGo = !explicit || args.includes('--go')
const doJs = !explicit || args.includes('--js')
const doMerge = !explicit || args.includes('--merge')

interface GoLicenseEntry {
  name: string
  version: string
  licenseName: string
  licenseURL: string
  licenseText: string
}

interface JsLicenseEntry {
  name: string
  version: string
  identifier: string
  text: string
}

interface LicenseEntry {
  name: string
  version: string
  spdx: string
  source: 'go' | 'js' | 'both'
  repo?: string
  isDev?: boolean
  // SPDX dedup: if the license body matches the canonical SPDX base,
  // only copyrightNotice is stored. Otherwise, fullText has the complete text.
  copyrightNotice?: string
  fullText?: string
}

interface LicensesJson {
  bases: Record<string, string>
  entries: LicenseEntry[]
}

// Map aperturerobotics fork module paths to upstream repos.
const forkToUpstream: Record<string, string> = {
  'github.com/aperturerobotics/bbolt': 'https://github.com/etcd-io/bbolt',
  'github.com/aperturerobotics/json-iterator-lite':
    'https://github.com/nicholasgasior/gojsoniter',
  'github.com/aperturerobotics/go-multiaddr':
    'https://github.com/multiformats/go-multiaddr',
  'github.com/aperturerobotics/go-multicodec':
    'https://github.com/multiformats/go-multicodec',
  'github.com/aperturerobotics/go-multihash':
    'https://github.com/multiformats/go-multihash',
  'github.com/aperturerobotics/go-multistream':
    'https://github.com/multiformats/go-multistream',
  'github.com/aperturerobotics/go-websocket':
    'https://github.com/nhooyr/websocket',
  'github.com/aperturerobotics/go-varint':
    'https://github.com/multiformats/go-varint',
  'github.com/aperturerobotics/go-libp2p-channel':
    'https://github.com/libp2p/go-libp2p-channel',
}

interface RunResult {
  code: number | null
  stdout: string
  stderr: string
}

function run(
  command: string,
  cmdArgs: string[],
  cwd?: string,
  env?: NodeJS.ProcessEnv,
): Promise<RunResult> {
  return new Promise((res) => {
    const proc = spawn(command, cmdArgs, {
      cwd: cwd || rootDir,
      stdio: ['inherit', 'pipe', 'pipe'],
      env: env ? { ...process.env, ...env } : process.env,
    })
    let stdout = ''
    let stderr = ''
    proc.stdout.on('data', (data: Buffer) => {
      stdout += data.toString()
    })
    proc.stderr.on('data', (data: Buffer) => {
      stderr += data.toString()
    })
    proc.on('close', (code) => {
      res({ code, stdout, stderr })
    })
  })
}

// Builds the go-licenses binary from scripts/licenses/ into .tools/.
// The sub-module pins go-licenses v2 via a `tool` directive so the working
// version is reproducible and isolated from spacewave's own dependency graph.
async function buildGoLicenses(): Promise<string> {
  const binPath = join(rootDir, '.tools', 'go-licenses')
  if (existsSync(binPath)) return binPath
  mkdirSync(dirname(binPath), { recursive: true })
  const result = await run(
    'go',
    ['-C', 'scripts/licenses', 'build', '-o', binPath, 'github.com/google/go-licenses/v2'],
    rootDir,
  )
  if (result.code !== 0) {
    console.error('failed to build go-licenses:', result.stderr)
    process.exit(1)
  }
  return binPath
}

// Go license generation: runs go-licenses with a template, deduplicates
// sub-packages to module level, enriches versions from vendor/modules.txt.
async function generateGoLicenses(): Promise<GoLicenseEntry[]> {
  console.log('Generating Go licenses...')
  const tplPath = join(rootDir, 'scripts', 'go-license-template.tpl')
  const binPath = await buildGoLicenses()

  // Run go-licenses once per supported (GOOS, GOARCH) pair and union the
  // results. Without iterating over platforms, packages behind build
  // constraints (e.g. bazil.org/fuse on linux/darwin, syscall/js on
  // js/wasm, windows-only registry packages) are missed depending on the
  // host. The module-level dedup loop below merges duplicates across runs.
  const platforms: Array<{ goos: string; goarch: string }> = [
    { goos: 'linux', goarch: 'amd64' },
    { goos: 'darwin', goarch: 'amd64' },
    { goos: 'windows', goarch: 'amd64' },
    { goos: 'js', goarch: 'wasm' },
  ]

  const raw: GoLicenseEntry[] = []
  for (const { goos, goarch } of platforms) {
    const result = await run(
      binPath,
      ['report', './...', '--template', tplPath, '--ignore', 'github.com/s4wave/spacewave'],
      rootDir,
      { GOOS: goos, GOARCH: goarch },
    )

    if (result.code !== 0 && !result.stdout.trim()) {
      console.error(`go-licenses failed for ${goos}/${goarch}:`, result.stderr)
      process.exit(1)
    }

    let entries: GoLicenseEntry[]
    try {
      entries = JSON.parse(result.stdout)
    } catch {
      console.error(`Failed to parse go-licenses JSON output for ${goos}/${goarch}`)
      console.error('stdout (first 500 chars):', result.stdout.slice(0, 500))
      process.exit(1)
    }

    console.log(`  ${goos}/${goarch}: ${entries.length} packages`)
    raw.push(...entries)
  }

  // Parse vendor/modules.txt for version mapping
  const versions: Record<string, string> = {}
  const modulesTxt = readFileSync(join(rootDir, 'vendor', 'modules.txt'), 'utf-8')
  for (const line of modulesTxt.split('\n')) {
    const m = line.match(/^# (\S+) (\S+)/)
    if (m) versions[m[1]] = m[2]
  }

  function findModule(pkg: string): string {
    const parts = pkg.split('/')
    for (let i = parts.length; i > 0; i--) {
      const candidate = parts.slice(0, i).join('/')
      if (versions[candidate]) return candidate
    }
    return parts.length >= 3 ? parts.slice(0, 3).join('/') : pkg
  }

  // Deduplicate to module level
  const modules: Record<string, GoLicenseEntry> = {}
  for (const entry of raw) {
    const mod = findModule(entry.name)
    if (!modules[mod]) {
      modules[mod] = {
        name: mod,
        version: versions[mod] || entry.version || 'Unknown',
        licenseName: entry.licenseName,
        licenseURL: entry.licenseURL,
        licenseText: entry.licenseText || '',
      }
    } else {
      const existing = modules[mod]
      if (!existing.licenseText && entry.licenseText) {
        existing.licenseText = entry.licenseText
        existing.licenseURL = entry.licenseURL
      }
      if (existing.licenseName === 'Unknown' && entry.licenseName !== 'Unknown') {
        existing.licenseName = entry.licenseName
      }
    }
  }

  const goLicenses = Object.values(modules).sort((a, b) => a.name.localeCompare(b.name))

  mkdirSync(outDir, { recursive: true })
  writeFileSync(join(outDir, 'go-licenses.json'), JSON.stringify(goLicenses, null, 2) + '\n')

  const withText = goLicenses.filter((e) => e.licenseText).length
  console.log(`Go: ${goLicenses.length} modules, ${withText} with license text`)
  return goLicenses
}

// JS license generation: runs a Vite build with build.license enabled.
async function generateJsLicenses(): Promise<JsLicenseEntry[]> {
  console.log('Generating JS licenses...')

  mkdirSync(tmpDir, { recursive: true })

  await build({
    root: rootDir,
    configFile: resolve(rootDir, 'vite.config.ts'),
    build: {
      license: { fileName: 'licenses.json' },
      outDir: tmpDir,
      emptyOutDir: true,
    },
    logLevel: 'warn',
  })

  const licensePath = join(tmpDir, 'licenses.json')
  let jsLicenses: JsLicenseEntry[]
  try {
    jsLicenses = JSON.parse(readFileSync(licensePath, 'utf-8'))
  } catch {
    console.error('Failed to read Vite license output at', licensePath)
    process.exit(1)
  }

  rmSync(tmpDir, { recursive: true, force: true })

  mkdirSync(outDir, { recursive: true })
  writeFileSync(join(outDir, 'js-licenses.json'), JSON.stringify(jsLicenses, null, 2) + '\n')

  const withText = jsLicenses.filter((e) => e.text).length
  console.log(`JS: ${jsLicenses.length} packages, ${withText} with license text`)
  return jsLicenses
}

// Derive upstream repo URL from a Go module path.
function goModuleRepo(name: string): string | undefined {
  if (forkToUpstream[name]) return forkToUpstream[name]
  const parts = name.split('/')
  if (parts[0] === 'github.com' && parts.length >= 3) {
    return `https://github.com/${parts[1]}/${parts[2]}`
  }
  if (parts[0] === 'golang.org' && parts[1] === 'x' && parts.length >= 3) {
    return `https://github.com/golang/${parts[2]}`
  }
  if (parts[0].endsWith('.io') || parts[0].endsWith('.dev')) {
    return `https://${parts.slice(0, Math.min(parts.length, 3)).join('/')}`
  }
  return undefined
}

// Derive npm repo URL from a JS package name.
function jsPackageRepo(name: string): string | undefined {
  if (name.startsWith('@aptre/')) {
    return `https://github.com/aperturerobotics/${name.replace('@aptre/', '')}`
  }
  if (name.startsWith('@radix-ui/')) {
    return 'https://github.com/radix-ui/primitives'
  }
  if (name.startsWith('@lexical/') || name === 'lexical') {
    return 'https://github.com/facebook/lexical'
  }
  return undefined
}

// Normalize whitespace for license body comparison.
// Collapses all whitespace (including line breaks within paragraphs) to single
// spaces, preserving paragraph breaks (double newlines).
function normalize(text: string): string {
  return text
    .replace(/\r\n/g, '\n')
    .replace(/\n\n+/g, '\x00')       // preserve paragraph breaks
    .replace(/\s+/g, ' ')             // collapse all whitespace
    .replace(/\x00/g, '\n\n')         // restore paragraph breaks
    .trim()
}

// Extract copyright notice lines and the license body from a license text.
function splitLicense(text: string): { copyright: string; body: string } {
  const lines = text.trim().split('\n')
  const copyrightLines: string[] = []
  let bodyStart = 0

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim().toLowerCase()
    if (
      line === '' ||
      line === 'mit license' ||
      line === 'the mit license' ||
      line === 'the mit license (mit)' ||
      line === 'isc license' ||
      line === 'bsd 2-clause license' ||
      line === 'bsd 3-clause license' ||
      line.startsWith('copyright')
    ) {
      if (line.startsWith('copyright')) {
        copyrightLines.push(lines[i].trim())
      }
      continue
    }
    bodyStart = i
    break
  }

  return {
    copyright: copyrightLines.join('\n'),
    body: lines.slice(bodyStart).join('\n').trim(),
  }
}

// Extract SPDX base texts from the actual license data by finding the most
// common body for each SPDX identifier.
function extractBases(
  entries: Array<{ spdx: string; text: string }>,
): Record<string, string> {
  const bodyGroups: Record<string, Map<string, number>> = {}

  for (const entry of entries) {
    if (!entry.text || entry.spdx === 'Unknown') continue
    const { body } = splitLicense(entry.text)
    const normalized = normalize(body)
    if (!normalized) continue

    const spdx = entry.spdx
    if (!bodyGroups[spdx]) bodyGroups[spdx] = new Map()
    const counts = bodyGroups[spdx]
    counts.set(normalized, (counts.get(normalized) || 0) + 1)
  }

  const bases: Record<string, string> = {}
  for (const [spdx, counts] of Object.entries(bodyGroups)) {
    // Pick the most common body as the canonical base
    let best = ''
    let bestCount = 0
    for (const [body, count] of counts) {
      if (count > bestCount) {
        best = body
        bestCount = count
      }
    }
    if (bestCount >= 2) {
      bases[spdx] = best
    }
  }

  return bases
}

// Merge Go and JS licenses into unified schema with SPDX base text dedup.
function mergeLicenses(goData: GoLicenseEntry[], jsData: JsLicenseEntry[]): LicensesJson {
  console.log('Merging licenses...')

  // Read package.json devDependencies for isDev marking.
  const pkgJson = JSON.parse(readFileSync(join(rootDir, 'package.json'), 'utf-8'))
  const devDeps = new Set(Object.keys(pkgJson.devDependencies || {}))

  // First pass: collect all entries with their raw text
  const rawEntries: Array<{
    name: string
    version: string
    spdx: string
    text: string
    source: 'go' | 'js' | 'both'
    repo?: string
    isDev?: boolean
  }> = []

  const seen = new Map<string, number>()

  for (const entry of goData) {
    seen.set(entry.name, rawEntries.length)
    rawEntries.push({
      name: entry.name,
      version: entry.version,
      spdx: entry.licenseName,
      text: entry.licenseText,
      source: 'go',
      repo: goModuleRepo(entry.name),
    })
  }

  for (const entry of jsData) {
    const idx = seen.get(entry.name)
    if (idx !== undefined) {
      rawEntries[idx].source = 'both'
      if (!rawEntries[idx].text && entry.text) rawEntries[idx].text = entry.text
    } else {
      rawEntries.push({
        name: entry.name,
        version: entry.version,
        spdx: entry.identifier,
        text: entry.text || '',
        source: 'js',
        repo: jsPackageRepo(entry.name),
        isDev: devDeps.has(entry.name) || undefined,
      })
    }
  }

  // Extract SPDX base texts from the collected data
  const bases = extractBases(rawEntries)

  // Second pass: dedup against bases
  let deduped = 0
  const entries: LicenseEntry[] = rawEntries
    .sort((a, b) => a.name.localeCompare(b.name))
    .map((raw) => {
      const entry: LicenseEntry = {
        name: raw.name,
        version: raw.version,
        spdx: raw.spdx,
        source: raw.source,
        repo: raw.repo,
        isDev: raw.isDev,
      }

      if (!raw.text) return entry

      const base = bases[raw.spdx]
      if (base) {
        const { copyright, body } = splitLicense(raw.text)
        if (normalize(body) === base) {
          // Body matches base, store only the copyright notice
          entry.copyrightNotice = copyright || undefined
          deduped++
          return entry
        }
      }

      // No base match, store full text
      entry.fullText = raw.text
      return entry
    })

  const result: LicensesJson = { bases, entries }

  writeFileSync(join(outDir, 'licenses.json'), JSON.stringify(result, null, 2) + '\n')

  const spdxSet = new Set(entries.map((e) => e.spdx))
  console.log(
    `Merged: ${entries.length} entries (${goData.length} Go + ${jsData.length} JS), ${spdxSet.size} license types`,
  )
  console.log(
    `SPDX dedup: ${deduped} entries use base text (${Object.keys(bases).length} bases), ${entries.length - deduped} have full text`,
  )
  return result
}

async function main() {
  mkdirSync(outDir, { recursive: true })

  let goData: GoLicenseEntry[] | undefined
  let jsData: JsLicenseEntry[] | undefined
  const tasks: Promise<void>[] = []
  if (doGo) tasks.push(generateGoLicenses().then((d) => { goData = d }))
  if (doJs) tasks.push(generateJsLicenses().then((d) => { jsData = d }))
  await Promise.all(tasks)

  if (doMerge) {
    if (!goData) {
      const p = join(outDir, 'go-licenses.json')
      goData = existsSync(p) ? JSON.parse(readFileSync(p, 'utf-8')) : []
    }
    if (!jsData) {
      const p = join(outDir, 'js-licenses.json')
      jsData = existsSync(p) ? JSON.parse(readFileSync(p, 'utf-8')) : []
    }
    const result = mergeLicenses(goData!, jsData!)
    const bytes = Buffer.byteLength(JSON.stringify(result))
    const fullBytes = Buffer.byteLength(JSON.stringify(
      result.entries.map((e) => ({ ...e, text: e.fullText || '' })),
    ))
    console.log(`Size: ${(bytes / 1024).toFixed(0)} KB (vs ${(fullBytes / 1024).toFixed(0)} KB without dedup)`)
  }

  console.log('Done. Output: app/licenses/')
}

main()
