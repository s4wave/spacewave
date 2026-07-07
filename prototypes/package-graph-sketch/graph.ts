import { spawnSync } from 'node:child_process'
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import path from 'node:path'

type Side = 'left' | 'right'
type NodeKind =
  | 'go'
  | 'goscript-generated'
  | 'goscript-generated-overridden'
  | 'gs-override'
  | 'typescript'

type GraphNode = {
  id: string
  label: string
  side: Side
  kind: NodeKind
  packagePath: string
  files?: number
  sourceRoot?: string
  x?: number
  y?: number
}

type GraphEdge = {
  from: string
  to: string
  kind: string
  count: number
}

type Options = {
  repoRoot: string
  out: string
  jsonOut: string
  mode: 'overview' | 'full'
  goPackages: string[]
  goTags: string
  tsRoots: string[]
  generatedRoots: string[]
  overrideRoots: string[]
  includeExternalGo: boolean
  maxNodes: number
  maxEdges: number
}

type GoPackage = {
  ImportPath: string
  Imports?: string[]
  Standard?: boolean
  Error?: { Err?: string }
}

type Tsconfig = {
  compilerOptions?: {
    paths?: Record<string, string[]>
  }
}

type OverviewFamily = {
  family: string
  generated: number
  overriddenGenerated: number
  overrides: number
  overrideOnly: number
}

type OverviewSummary = {
  totalGo: number
  totalGenerated: number
  totalOverriddenGenerated: number
  totalOverrides: number
  totalOverrideOnly: number
  totalTypeScript: number
  typeScriptToGenerated: number
  typeScriptToOverride: number
  typeScriptToGo: number
  families: OverviewFamily[]
  overriddenPackages: {
    packagePath: string
    generatedFiles: number
    overrideFiles: number
    importedByTypeScript: number
  }[]
  overrideOnlyPackages: { packagePath: string; overrideFiles: number }[]
}

const modulePath = 'github.com/s4wave/spacewave'
const defaultGoPackages = [
  './cmd/...',
  './core/...',
  './plugin/...',
  './sdk/...',
  './net/...',
  './db/...',
  './bldr/...',
]
const defaultTsRoots = [
  'app',
  'web',
  'core',
  'sdk',
  'bldr/web',
  'bldr/sdk',
  'bldr/plugin',
  'bldr/example',
  'bldr/entrypoint',
  'bldr/util',
  'net',
  'db',
  'forge/lib/v86/bun',
]

function main() {
  const opts = parseArgs(process.argv.slice(2))
  mkdirSync(path.dirname(opts.out), { recursive: true })
  mkdirSync(path.dirname(opts.jsonOut), { recursive: true })

  const warnings: string[] = []
  const nodes = new Map<string, GraphNode>()
  const edges = new Map<string, GraphEdge>()

  const overridePackages = scanPackageRoots(opts.overrideRoots, warnings)
  const generatedPackages = scanPackageRoots(opts.generatedRoots, warnings)
  const goPackages = loadGoPackages(opts, warnings)

  for (const pkg of goPackages.values()) {
    if (!opts.includeExternalGo && !isRepoGoPackage(pkg.ImportPath)) {
      continue
    }
    addNode(nodes, {
      id: goID(pkg.ImportPath),
      label: displayPackage(pkg.ImportPath),
      side: 'left',
      kind: 'go',
      packagePath: pkg.ImportPath,
    })
  }

  for (const pkg of goPackages.values()) {
    const from = goID(pkg.ImportPath)
    if (!nodes.has(from)) {
      continue
    }
    for (const dep of pkg.Imports ?? []) {
      const to = goID(dep)
      if (nodes.has(to)) {
        addEdge(edges, from, to, 'go-import')
      }
    }
  }

  for (const pkg of generatedPackages.values()) {
    const overridden = overridePackages.has(pkg.packagePath)
    const id = generatedID(pkg.packagePath)
    addNode(nodes, {
      id,
      label: displayPackage(pkg.packagePath),
      side: 'left',
      kind: overridden ? 'goscript-generated-overridden' : 'goscript-generated',
      packagePath: pkg.packagePath,
      files: pkg.files,
      sourceRoot: pkg.sourceRoot,
    })
    const go = goID(pkg.packagePath)
    if (nodes.has(go)) {
      addEdge(
        edges,
        go,
        id,
        overridden ? 'overridden-output' : 'compiled-output',
      )
    }
  }

  for (const pkg of overridePackages.values()) {
    const id = overrideID(pkg.packagePath)
    addNode(nodes, {
      id,
      label: displayPackage(pkg.packagePath),
      side: 'right',
      kind: 'gs-override',
      packagePath: pkg.packagePath,
      files: pkg.files,
      sourceRoot: pkg.sourceRoot,
    })
    const generated = generatedID(pkg.packagePath)
    const go = goID(pkg.packagePath)
    if (nodes.has(generated)) {
      addEdge(edges, generated, id, 'manual-override')
    } else if (nodes.has(go)) {
      addEdge(edges, go, id, 'manual-override')
    }
  }

  scanGeneratedImports(opts.generatedRoots, nodes, edges, warnings)
  scanOverrideImports(opts.overrideRoots, nodes, edges, warnings)
  scanTypeScript(opts, nodes, edges, warnings)

  const fullNodes = [...nodes.values()]
  const fullEdges = [...edges.values()]
  const overview = buildOverviewSummary(fullNodes, fullEdges)
  const svg =
    opts.mode === 'overview'
      ? renderOverviewSVG(overview, {
          totalNodes: fullNodes.length,
          totalEdges: fullEdges.length,
          warnings,
        })
      : renderFullSVG(fullNodes, fullEdges, opts, warnings)

  writeFileSync(opts.out, svg)
  writeFileSync(
    opts.jsonOut,
    JSON.stringify(
      {
        options: opts,
        warnings,
        overview,
        nodes: fullNodes,
        edges: fullEdges,
      },
      null,
      2,
    ),
  )

  console.log(`wrote ${opts.out}`)
  console.log(`wrote ${opts.jsonOut}`)
  console.log(
    `mode=${opts.mode} nodes=${fullNodes.length} edges=${fullEdges.length} out=${opts.out}`,
  )
}

function parseArgs(args: string[]): Options {
  const repoRoot = path.resolve(process.cwd())
  const outDefault = path.join(repoRoot, '.tmp', 'package-graph-sketch.svg')
  const opts: Options = {
    repoRoot,
    out: outDefault,
    jsonOut: outDefault.replace(/\.svg$/u, '.json'),
    mode: 'overview',
    goPackages: [],
    goTags: 'skip_e2e,purego,goscript',
    tsRoots: [],
    generatedRoots: [],
    overrideRoots: [],
    includeExternalGo: true,
    maxNodes: 1800,
    maxEdges: 5000,
  }

  for (let index = 0; index < args.length; index += 1) {
    const raw = args[index]
    const [name, inlineValue] = raw.includes('=')
      ? raw.split(/=(.*)/su, 2)
      : [raw, undefined]
    const value = () => {
      if (inlineValue !== undefined) {
        return inlineValue
      }
      index += 1
      if (index >= args.length) {
        throw new Error(`${name} requires a value`)
      }
      return args[index]
    }

    switch (name) {
      case '--out':
        opts.out = path.resolve(repoRoot, value())
        opts.jsonOut = opts.out.replace(/\.svg$/u, '.json')
        break
      case '--json-out':
        opts.jsonOut = path.resolve(repoRoot, value())
        break
      case '--mode': {
        const mode = value()
        if (mode !== 'overview' && mode !== 'full') {
          throw new Error(`--mode must be overview or full, got ${mode}`)
        }
        opts.mode = mode
        break
      }
      case '--go-package':
        opts.goPackages.push(value())
        break
      case '--go-tags':
        opts.goTags = value()
        break
      case '--ts-root':
        opts.tsRoots.push(value())
        break
      case '--generated-root':
        opts.generatedRoots.push(value())
        break
      case '--override-root':
        opts.overrideRoots.push(value())
        break
      case '--spacewave-go-only':
        opts.includeExternalGo = false
        break
      case '--max-nodes':
        opts.maxNodes = Number.parseInt(value(), 10)
        break
      case '--max-edges':
        opts.maxEdges = Number.parseInt(value(), 10)
        break
      case '--help':
        printHelp()
        process.exit(0)
      default:
        throw new Error(`unknown option ${raw}`)
    }
  }

  if (opts.goPackages.length === 0) {
    opts.goPackages = defaultGoPackages
  }
  if (opts.tsRoots.length === 0) {
    opts.tsRoots = defaultTsRoots
  }
  if (opts.overrideRoots.length === 0) {
    opts.overrideRoots = ['./gs', '../goscript/gs']
  }
  if (opts.generatedRoots.length === 0) {
    opts.generatedRoots = discoverGeneratedRoots(repoRoot)
  }

  opts.tsRoots = opts.tsRoots
    .map((root) => path.resolve(repoRoot, root))
    .filter(existsSync)
  opts.overrideRoots = opts.overrideRoots
    .map((root) => path.resolve(repoRoot, root))
    .filter(existsSync)
  opts.generatedRoots = opts.generatedRoots
    .map((root) => normalizeGoScriptRoot(path.resolve(repoRoot, root)))
    .filter(existsSync)

  return opts
}

function printHelp() {
  console.log(`usage: bun prototypes/package-graph-sketch/graph.ts [options]

  --out <svg>                  output SVG path
  --json-out <json>            output JSON sidecar path
  --mode <overview|full>       overview is default; full renders package nodes
  --go-package <pattern>       repeatable go list pattern
  --go-tags <tags>             go build tags, default skip_e2e,purego,goscript
  --ts-root <dir>              repeatable TypeScript root
  --generated-root <dir>       repeatable @goscript generated root
  --override-root <dir>        repeatable manual gs override root
  --spacewave-go-only          omit stdlib and external Go dependency nodes
  --max-nodes <n>              max SVG nodes, JSON still includes all nodes
  --max-edges <n>              max SVG edges`)
}

function loadGoPackages(opts: Options, warnings: string[]) {
  const args = ['list', '-e', '-deps', '-json']
  if (opts.goTags.trim() !== '') {
    args.push(`-tags=${opts.goTags}`)
  }
  args.push(...opts.goPackages)

  const result = spawnSync('go', args, {
    cwd: opts.repoRoot,
    encoding: 'utf8',
    maxBuffer: 256 * 1024 * 1024,
  })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0) {
    throw new Error(
      `go ${args.join(' ')} failed:\n${result.stderr}\n${result.stdout}`,
    )
  }

  const packages = new Map<string, GoPackage>()
  for (const pkg of parseConcatenatedJSON<GoPackage>(result.stdout)) {
    if (!pkg.ImportPath) {
      continue
    }
    if (pkg.Error?.Err) {
      warnings.push(`go list error for ${pkg.ImportPath}: ${pkg.Error.Err}`)
    }
    packages.set(pkg.ImportPath, pkg)
  }
  return packages
}

function parseConcatenatedJSON<T>(text: string): T[] {
  const values: T[] = []
  let depth = 0
  let start = -1
  let inString = false
  let escaped = false

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index]
    if (inString) {
      if (escaped) {
        escaped = false
      } else if (char === '\\') {
        escaped = true
      } else if (char === '"') {
        inString = false
      }
      continue
    }
    if (char === '"') {
      inString = true
      continue
    }
    if (char === '{') {
      if (depth === 0) {
        start = index
      }
      depth += 1
      continue
    }
    if (char === '}') {
      depth -= 1
      if (depth === 0 && start >= 0) {
        values.push(JSON.parse(text.slice(start, index + 1)) as T)
        start = -1
      }
    }
  }
  return values
}

function discoverGeneratedRoots(repoRoot: string) {
  const roots: string[] = []
  for (const start of ['.bldr-dist', '.bldr', '.tmp']) {
    const abs = path.join(repoRoot, start)
    if (existsSync(abs)) {
      discoverNamedDir(abs, '@goscript', roots, 7)
    }
  }
  return roots
}

function discoverNamedDir(
  root: string,
  name: string,
  roots: string[],
  depth: number,
) {
  if (depth < 0) {
    return
  }
  let entries
  try {
    entries = readdirSync(root, { withFileTypes: true })
  } catch {
    return
  }
  for (const entry of entries) {
    if (!entry.isDirectory()) {
      continue
    }
    const abs = path.join(root, entry.name)
    if (entry.name === name) {
      roots.push(abs)
      continue
    }
    discoverNamedDir(abs, name, roots, depth - 1)
  }
}

function normalizeGoScriptRoot(root: string) {
  if (path.basename(root) === '@goscript') {
    return root
  }
  const nested = path.join(root, '@goscript')
  return existsSync(nested) ? nested : root
}

function scanPackageRoots(roots: string[], warnings: string[]) {
  const packages = new Map<
    string,
    { packagePath: string; sourceRoot: string; files: number }
  >()
  for (const root of roots) {
    if (!existsSync(root)) {
      warnings.push(`package root missing: ${root}`)
      continue
    }
    for (const file of walkFiles(root, [
      '.ts',
      '.tsx',
      '.js',
      '.jsx',
      '.json',
    ])) {
      if (isTestFile(file)) {
        continue
      }
      const rel = slash(path.relative(root, file))
      if (rel === '' || rel.startsWith('..')) {
        continue
      }
      const dir = slash(path.dirname(rel))
      if (dir === '.') {
        continue
      }
      const existing = packages.get(dir)
      if (existing) {
        existing.files += 1
      } else {
        packages.set(dir, { packagePath: dir, sourceRoot: root, files: 1 })
      }
    }
  }
  return packages
}

function scanGeneratedImports(
  roots: string[],
  nodes: Map<string, GraphNode>,
  edges: Map<string, GraphEdge>,
  warnings: string[],
) {
  for (const root of roots) {
    for (const file of walkFiles(root, ['.ts', '.tsx', '.js', '.jsx'])) {
      if (isTestFile(file)) {
        continue
      }
      const fromPackage = slash(path.dirname(path.relative(root, file)))
      if (fromPackage === '.' || !nodes.has(generatedID(fromPackage))) {
        continue
      }
      for (const specifier of parseImports(file, warnings)) {
        const target = resolveGoScriptSpecifier(
          specifier,
          path.dirname(file),
          root,
        )
        if (target && nodes.has(generatedID(target))) {
          addEdge(
            edges,
            generatedID(fromPackage),
            generatedID(target),
            'goscript-import',
          )
        }
      }
    }
  }
}

function scanOverrideImports(
  roots: string[],
  nodes: Map<string, GraphNode>,
  edges: Map<string, GraphEdge>,
  warnings: string[],
) {
  for (const root of roots) {
    for (const file of walkFiles(root, ['.ts', '.tsx', '.js', '.jsx'])) {
      if (isTestFile(file)) {
        continue
      }
      const fromPackage = slash(path.dirname(path.relative(root, file)))
      if (fromPackage === '.' || !nodes.has(overrideID(fromPackage))) {
        continue
      }
      for (const specifier of parseImports(file, warnings)) {
        const target = resolveGoScriptSpecifier(
          specifier,
          path.dirname(file),
          root,
        )
        if (target && nodes.has(overrideID(target))) {
          addEdge(
            edges,
            overrideID(fromPackage),
            overrideID(target),
            'override-import',
          )
        } else if (target && nodes.has(generatedID(target))) {
          addEdge(
            edges,
            overrideID(fromPackage),
            generatedID(target),
            'override-goscript-import',
          )
        }
      }
    }
  }
}

function scanTypeScript(
  opts: Options,
  nodes: Map<string, GraphNode>,
  edges: Map<string, GraphEdge>,
  warnings: string[],
) {
  const aliases = loadPathAliases(opts.repoRoot)
  const tsRootForFile = new Map<string, string>()
  const sourceFiles: string[] = []

  for (const root of opts.tsRoots) {
    for (const file of walkFiles(root, ['.ts', '.tsx', '.js', '.jsx'])) {
      if (isTestFile(file) || isGeneratedTypeScriptPath(file)) {
        continue
      }
      sourceFiles.push(file)
      tsRootForFile.set(file, root)
      const pkg = tsPackageForFile(root, file)
      addNode(nodes, {
        id: tsID(pkg),
        label: pkg,
        side: 'right',
        kind: 'typescript',
        packagePath: pkg,
      })
    }
  }

  const roots = opts.tsRoots.slice().sort((a, b) => b.length - a.length)
  for (const file of sourceFiles) {
    const fromRoot = tsRootForFile.get(file)
    if (!fromRoot) {
      continue
    }
    const from = tsID(tsPackageForFile(fromRoot, file))
    for (const specifier of parseImports(file, warnings)) {
      if (specifier.startsWith('@goscript/')) {
        const pkg = normalizeGoScriptPackagePath(
          specifier.slice('@goscript/'.length),
        )
        const target = nodes.has(overrideID(pkg))
          ? overrideID(pkg)
          : generatedID(pkg)
        if (nodes.has(target)) {
          addEdge(edges, from, target, 'typescript-goscript-import')
        }
        continue
      }
      if (specifier.startsWith('@go/')) {
        const goPath = specifier.slice('@go/'.length).replace(/\/[^/]*$/u, '')
        if (nodes.has(goID(goPath))) {
          addEdge(edges, from, goID(goPath), 'typescript-go-import')
        }
        continue
      }
      const resolved = resolveTypeScriptImport(
        opts.repoRoot,
        roots,
        aliases,
        file,
        specifier,
      )
      if (!resolved) {
        continue
      }
      const targetRoot = roots.find((root) => isInside(root, resolved))
      if (!targetRoot) {
        continue
      }
      const target = tsID(tsPackageForFile(targetRoot, resolved))
      if (nodes.has(target) && target !== from) {
        addEdge(edges, from, target, 'typescript-import')
      }
    }
  }
}

function loadPathAliases(repoRoot: string) {
  const configPath = path.join(repoRoot, 'tsconfig.json')
  if (!existsSync(configPath)) {
    return []
  }
  const config = JSON.parse(readFileSync(configPath, 'utf8')) as Tsconfig
  const paths = config.compilerOptions?.paths ?? {}
  return Object.entries(paths)
    .map(([key, values]) => ({ key, values }))
    .sort((a, b) => b.key.length - a.key.length)
}

function resolveTypeScriptImport(
  repoRoot: string,
  roots: string[],
  aliases: { key: string; values: string[] }[],
  fromFile: string,
  specifier: string,
) {
  if (specifier.startsWith('.')) {
    return resolveTypeScriptPath(
      path.resolve(path.dirname(fromFile), specifier),
    )
  }
  for (const alias of aliases) {
    const starIndex = alias.key.indexOf('*')
    if (starIndex >= 0) {
      const prefix = alias.key.slice(0, starIndex)
      const suffix = alias.key.slice(starIndex + 1)
      if (!specifier.startsWith(prefix) || !specifier.endsWith(suffix)) {
        continue
      }
      const wildcard = specifier.slice(
        prefix.length,
        specifier.length - suffix.length,
      )
      for (const value of alias.values) {
        const target = value.split('*').join(wildcard)
        const resolved = resolveTypeScriptPath(path.resolve(repoRoot, target))
        if (resolved) {
          return resolved
        }
      }
    } else if (specifier === alias.key) {
      for (const value of alias.values) {
        const resolved = resolveTypeScriptPath(path.resolve(repoRoot, value))
        if (resolved) {
          return resolved
        }
      }
    }
  }

  for (const root of roots) {
    const resolved = resolveTypeScriptPath(path.join(root, specifier))
    if (resolved) {
      return resolved
    }
  }
  return null
}

function resolveTypeScriptPath(base: string) {
  const candidates = [
    base,
    `${base}.ts`,
    `${base}.tsx`,
    `${base}.js`,
    `${base}.jsx`,
    path.join(base, 'index.ts'),
    path.join(base, 'index.tsx'),
    path.join(base, 'index.js'),
    path.join(base, 'index.jsx'),
  ]
  for (const candidate of candidates) {
    if (existsSync(candidate) && statSync(candidate).isFile()) {
      return candidate
    }
  }
  return null
}

function resolveGoScriptSpecifier(
  specifier: string,
  fromDir: string,
  root: string,
) {
  if (specifier.startsWith('@goscript/')) {
    return normalizeGoScriptPackagePath(specifier.slice('@goscript/'.length))
  }
  if (!specifier.startsWith('.')) {
    return null
  }
  const resolved = resolveTypeScriptPath(path.resolve(fromDir, specifier))
  if (!resolved || !isInside(root, resolved)) {
    return null
  }
  return slash(path.dirname(path.relative(root, resolved)))
}

function normalizeGoScriptPackagePath(value: string) {
  return value
    .replace(/\/(index|[^/]+)\.(gs\.)?[jt]sx?$/u, '')
    .replace(/\/$/u, '')
}

function parseImports(file: string, warnings: string[]) {
  let text = ''
  try {
    text = readFileSync(file, 'utf8')
  } catch (err) {
    warnings.push(`read failed for ${file}: ${err}`)
    return []
  }
  const specs = new Set<string>()
  const patterns = [
    /\bimport\s+(?:type\s+)?(?:[^'"]*?\s+from\s*)?['"]([^'"]+)['"]/gu,
    /\bexport\s+(?:type\s+)?[^'"]*?\s+from\s*['"]([^'"]+)['"]/gu,
    /\bimport\s*\(\s*['"]([^'"]+)['"]\s*\)/gu,
  ]
  for (const pattern of patterns) {
    for (const match of text.matchAll(pattern)) {
      specs.add(match[1])
    }
  }
  return [...specs]
}

function walkFiles(root: string, extensions: string[]) {
  const files: string[] = []
  const skip = new Set([
    'node_modules',
    'vendor',
    'dist',
    '.git',
    '.bldr',
    '.bldr-dist',
    '.tmp',
    'coverage',
    'vite-check',
  ])
  const visit = (dir: string) => {
    let entries
    try {
      entries = readdirSync(dir, { withFileTypes: true })
    } catch {
      return
    }
    for (const entry of entries) {
      const abs = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        if (!skip.has(entry.name)) {
          visit(abs)
        }
        continue
      }
      if (
        entry.isFile() &&
        extensions.some((ext) => entry.name.endsWith(ext))
      ) {
        files.push(abs)
      }
    }
  }
  visit(root)
  return files
}

function tsPackageForFile(root: string, file: string) {
  const rootLabel =
    slash(path.relative(process.cwd(), root)) || slash(path.basename(root))
  const dir = slash(path.dirname(path.relative(root, file))).replace(
    /^\.\//u,
    '',
  )
  return dir === '.' ? rootLabel : `${rootLabel}/${dir}`
}

function selectRenderedNodes(nodes: GraphNode[], maxNodes: number) {
  const priority: Record<NodeKind, number> = {
    'gs-override': 0,
    'goscript-generated-overridden': 1,
    'goscript-generated': 2,
    go: 3,
    typescript: 4,
  }
  return nodes
    .slice()
    .sort(
      (a, b) =>
        priority[a.kind] - priority[b.kind] ||
        a.packagePath.localeCompare(b.packagePath),
    )
    .slice(0, maxNodes)
}

function layout(nodes: GraphNode[]) {
  const left = nodes
    .filter((node) => node.side === 'left')
    .sort(
      (a, b) =>
        leftBand(a) - leftBand(b) || a.packagePath.localeCompare(b.packagePath),
    )
  const right = nodes
    .filter((node) => node.side === 'right')
    .sort(
      (a, b) =>
        rightBand(a) - rightBand(b) ||
        a.packagePath.localeCompare(b.packagePath),
    )
  const row = 26
  const top = 150
  for (const [index, node] of left.entries()) {
    const depth = Math.min(node.packagePath.split('/').length, 18)
    const nearDivider = node.kind.startsWith('goscript-generated')
    node.x = nearDivider ? 960 - depth * 10 : 820 - depth * 20
    node.y = top + index * row
  }
  for (const [index, node] of right.entries()) {
    const depth = Math.min(node.packagePath.split('/').length, 18)
    node.x = node.kind === 'gs-override' ? 1160 + depth * 10 : 1320 + depth * 20
    node.y = top + index * row
  }
}

function buildOverviewSummary(
  nodes: GraphNode[],
  edges: GraphEdge[],
): OverviewSummary {
  const byID = new Map(nodes.map((node) => [node.id, node]))
  const generated = nodes.filter(
    (node) =>
      node.kind === 'goscript-generated' ||
      node.kind === 'goscript-generated-overridden',
  )
  const overrides = nodes.filter((node) => node.kind === 'gs-override')
  const families = new Map<string, OverviewFamily>()

  const familyFor = (packagePath: string) => {
    const family = packageFamily(packagePath)
    let entry = families.get(family)
    if (!entry) {
      entry = {
        family,
        generated: 0,
        overriddenGenerated: 0,
        overrides: 0,
        overrideOnly: 0,
      }
      families.set(family, entry)
    }
    return entry
  }

  for (const node of generated) {
    const family = familyFor(node.packagePath)
    family.generated += 1
    if (node.kind === 'goscript-generated-overridden') {
      family.overriddenGenerated += 1
    }
  }
  for (const node of overrides) {
    const family = familyFor(node.packagePath)
    family.overrides += 1
    if (!byID.has(generatedID(node.packagePath))) {
      family.overrideOnly += 1
    }
  }

  const typeScriptToGeneratedPackages = new Set<string>()
  const typeScriptToOverridePackages = new Set<string>()
  let typeScriptToGo = 0
  for (const edge of edges) {
    if (edge.kind === 'typescript-goscript-import') {
      const target = byID.get(edge.to)
      if (target?.kind === 'gs-override') {
        typeScriptToOverridePackages.add(target.packagePath)
      } else if (target?.kind?.startsWith('goscript-generated')) {
        typeScriptToGeneratedPackages.add(target.packagePath)
      }
    }
    if (edge.kind === 'typescript-go-import') {
      typeScriptToGo += edge.count
    }
  }

  const overriddenPackages = generated
    .filter((node) => node.kind === 'goscript-generated-overridden')
    .map((node) => {
      const override = byID.get(overrideID(node.packagePath))
      return {
        packagePath: node.packagePath,
        generatedFiles: node.files ?? 0,
        overrideFiles: override?.files ?? 0,
        importedByTypeScript: typeScriptToOverridePackages.has(node.packagePath)
          ? 1
          : 0,
      }
    })
    .sort(
      (a, b) =>
        b.importedByTypeScript - a.importedByTypeScript ||
        b.overrideFiles - a.overrideFiles ||
        a.packagePath.localeCompare(b.packagePath),
    )

  const overrideOnlyPackages = overrides
    .filter((node) => !byID.has(generatedID(node.packagePath)))
    .map((node) => ({
      packagePath: node.packagePath,
      overrideFiles: node.files ?? 0,
    }))
    .sort(
      (a, b) =>
        b.overrideFiles - a.overrideFiles ||
        a.packagePath.localeCompare(b.packagePath),
    )

  return {
    totalGo: nodes.filter((node) => node.kind === 'go').length,
    totalGenerated: generated.filter(
      (node) => node.kind === 'goscript-generated',
    ).length,
    totalOverriddenGenerated: generated.filter(
      (node) => node.kind === 'goscript-generated-overridden',
    ).length,
    totalOverrides: overrides.length,
    totalOverrideOnly: overrideOnlyPackages.length,
    totalTypeScript: nodes.filter((node) => node.kind === 'typescript').length,
    typeScriptToGenerated: typeScriptToGeneratedPackages.size,
    typeScriptToOverride: typeScriptToOverridePackages.size,
    typeScriptToGo,
    families: [...families.values()].sort(
      (a, b) =>
        b.overriddenGenerated - a.overriddenGenerated ||
        b.overrides - a.overrides ||
        b.generated - a.generated ||
        a.family.localeCompare(b.family),
    ),
    overriddenPackages,
    overrideOnlyPackages,
  }
}

function renderOverviewSVG(
  summary: OverviewSummary,
  stats: { totalNodes: number; totalEdges: number; warnings: string[] },
) {
  const width = 1600
  const divider = 800
  const familyRows = summary.families.slice(0, 28)
  const overrideRows = summary.overriddenPackages.slice(0, 32)
  const overrideOnlyRows = summary.overrideOnlyPackages.slice(0, 10)
  const height = Math.max(
    1120,
    430 + Math.max(familyRows.length * 28, overrideRows.length * 24),
  )
  const familyMax = Math.max(
    1,
    ...familyRows.map(
      (family) =>
        family.generated + family.overriddenGenerated + family.overrideOnly,
    ),
  )
  const overrideMax = Math.max(
    1,
    ...familyRows.map((family) => family.overrides),
  )
  const warningText =
    stats.warnings.length === 0
      ? ''
      : `<text x="48" y="150" class="small warning">${escapeXML(stats.warnings.slice(0, 2).join(' | '))}</text>`

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
<style>
  .title { font: 700 28px ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #111827; }
  .intent { font: 16px ui-sans-serif, system-ui, sans-serif; fill: #273449; }
  .section { font: 700 15px ui-sans-serif, system-ui, sans-serif; fill: #111827; }
  .small { font: 12px ui-sans-serif, system-ui, sans-serif; fill: #4b5563; }
  .mono { font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; fill: #111827; }
  .number { font: 700 26px ui-sans-serif, system-ui, sans-serif; fill: #111827; }
  .divider { stroke: #111827; stroke-width: 2; stroke-dasharray: 8 8; }
  .panel { fill: #ffffff; stroke: #d7dde7; stroke-width: 1; rx: 8; }
  .warning { fill: #9a3412; }
</style>
<rect width="100%" height="100%" fill="#f8fafc"/>
<text x="48" y="52" class="title">GoScript Boundary Overview</text>
<text x="48" y="86" class="intent">What am I trying to see? Which browser packages are still compiled from Go, which are hand-written TypeScript substitutes in gs/, and where the manual override hotspots are.</text>
<text x="48" y="118" class="small">Full graph data: ${stats.totalNodes} packages, ${stats.totalEdges} edges. This overview suppresses most edges so the boundary is legible.</text>
${warningText}
<line x1="${divider}" y1="170" x2="${divider}" y2="${height - 48}" class="divider"/>
<text x="630" y="194" class="section">Go and generated GoScript output</text>
<text x="832" y="194" class="section">Manual gs overrides and TypeScript consumers</text>
${metricCard(48, 220, 'Compiled output', summary.totalGenerated, '#16a34a', 'Generated packages with no matching gs/ override')}
${metricCard(288, 220, 'Overridden output', summary.totalOverriddenGenerated, '#f97316', 'Generated packages that have a manual substitute')}
${metricCard(832, 220, 'Manual overrides', summary.totalOverrides, '#dc2626', `${summary.totalOverrideOnly} override-only packages`)}
${metricCard(1072, 220, 'TypeScript packages', summary.totalTypeScript, '#7c3aed', `${summary.typeScriptToGenerated + summary.typeScriptToOverride} packages import @goscript`)}
${renderFamilyBars(familyRows, 48, 360, familyMax, overrideMax)}
${renderOverrideRows(overrideRows, 832, 360)}
${renderOverrideOnlyRows(overrideOnlyRows, 832, 360 + overrideRows.length * 24 + 54)}
<g transform="translate(48 ${height - 78})">
  ${legendItem(0, '#16a34a', 'compiled from Go')}
  ${legendItem(180, '#f97316', 'generated package with gs override')}
  ${legendItem(490, '#dc2626', 'manual gs package')}
  ${legendItem(680, '#7c3aed', 'TypeScript consumer')}
</g>
</svg>
`
}

function renderFamilyBars(
  rows: OverviewFamily[],
  x: number,
  y: number,
  familyMax: number,
  overrideMax: number,
) {
  const barWidth = 420
  const lines = [
    `<text x="${x}" y="${y - 30}" class="section">Where the generated graph is substituted</text>`,
    `<text x="${x}" y="${y - 10}" class="small">Green is ordinary generated output. Orange is generated output with a matching manual gs/ package. Red is the manual package count.</text>`,
  ]
  for (const [index, family] of rows.entries()) {
    const rowY = y + index * 28
    const compiled = family.generated
    const overridden = family.overriddenGenerated
    const compiledWidth = (compiled / familyMax) * barWidth
    const overriddenWidth = (overridden / familyMax) * barWidth
    const overrideWidth = (family.overrides / overrideMax) * 150
    lines.push(
      `<text x="${x}" y="${rowY + 11}" class="mono">${escapeXML(shorten(family.family, 34))}</text>`,
      `<rect x="${x + 250}" y="${rowY}" width="${compiledWidth.toFixed(1)}" height="12" rx="3" fill="#16a34a" fill-opacity="0.72"/>`,
      `<rect x="${x + 250 + compiledWidth}" y="${rowY}" width="${overriddenWidth.toFixed(1)}" height="12" rx="3" fill="#f97316" fill-opacity="0.86"/>`,
      `<rect x="${x + 250}" y="${rowY + 15}" width="${overrideWidth.toFixed(1)}" height="7" rx="3" fill="#dc2626" fill-opacity="0.78"/>`,
      `<text x="${x + 685}" y="${rowY + 11}" class="small">${compiled}/${overridden}/${family.overrides}</text>`,
    )
  }
  return lines.join('\n')
}

function renderOverrideRows(
  rows: OverviewSummary['overriddenPackages'],
  x: number,
  y: number,
) {
  const lines = [
    `<text x="${x}" y="${y - 30}" class="section">Override packages to inspect first</text>`,
    `<text x="${x}" y="${y - 10}" class="small">These packages exist in generated output and also have a matching manual gs/ implementation.</text>`,
  ]
  for (const [index, row] of rows.entries()) {
    const rowY = y + index * 24
    const marker = row.importedByTypeScript > 0 ? '#7c3aed' : '#dc2626'
    lines.push(
      `<circle cx="${x + 6}" cy="${rowY + 5}" r="5" fill="${marker}" fill-opacity="0.9"/>`,
      `<text x="${x + 20}" y="${rowY + 9}" class="mono">${escapeXML(shorten(displayPackage(row.packagePath), 58))}</text>`,
      `<text x="${x + 520}" y="${rowY + 9}" class="small">gen ${row.generatedFiles} / gs ${row.overrideFiles}</text>`,
    )
  }
  return lines.join('\n')
}

function renderOverrideOnlyRows(
  rows: OverviewSummary['overrideOnlyPackages'],
  x: number,
  y: number,
) {
  if (rows.length === 0) {
    return ''
  }
  const lines = [
    `<text x="${x}" y="${y}" class="section">Override-only packages</text>`,
    `<text x="${x}" y="${y + 20}" class="small">Manual gs/ packages without a matching generated package in this run.</text>`,
  ]
  for (const [index, row] of rows.entries()) {
    const rowY = y + 48 + index * 22
    lines.push(
      `<text x="${x}" y="${rowY}" class="mono">${escapeXML(shorten(displayPackage(row.packagePath), 62))}</text>`,
      `<text x="${x + 520}" y="${rowY}" class="small">gs ${row.overrideFiles}</text>`,
    )
  }
  return lines.join('\n')
}

function metricCard(
  x: number,
  y: number,
  title: string,
  value: number,
  color: string,
  note: string,
) {
  return `<g>
  <rect x="${x}" y="${y}" width="216" height="96" class="panel"/>
  <rect x="${x + 16}" y="${y + 18}" width="14" height="14" rx="4" fill="${color}"/>
  <text x="${x + 40}" y="${y + 30}" class="small">${escapeXML(title)}</text>
  <text x="${x + 16}" y="${y + 66}" class="number">${value}</text>
  <text x="${x + 16}" y="${y + 84}" class="small">${escapeXML(note)}</text>
</g>`
}

function renderFullSVG(
  fullNodes: GraphNode[],
  fullEdges: GraphEdge[],
  opts: Options,
  warnings: string[],
) {
  const renderedNodes = selectRenderedNodes(fullNodes, opts.maxNodes)
  const renderedNodeIDs = new Set(renderedNodes.map((node) => node.id))
  const renderedEdges = fullEdges
    .filter(
      (edge) => renderedNodeIDs.has(edge.from) && renderedNodeIDs.has(edge.to),
    )
    .slice(0, opts.maxEdges)

  layout(renderedNodes)
  return renderDenseSVG(renderedNodes, renderedEdges, {
    totalNodes: fullNodes.length,
    totalEdges: fullEdges.length,
    renderedNodes: renderedNodes.length,
    renderedEdges: renderedEdges.length,
    warnings,
  })
}

function renderDenseSVG(
  nodes: GraphNode[],
  edges: GraphEdge[],
  stats: {
    totalNodes: number
    totalEdges: number
    renderedNodes: number
    renderedEdges: number
    warnings: string[]
  },
) {
  const width = 2200
  const height = Math.max(
    800,
    220 + Math.max(...nodes.map((node) => node.y ?? 0), 0),
  )
  const divider = width / 2
  const nodeMap = new Map(nodes.map((node) => [node.id, node]))
  const edgeSVG = edges
    .map((edge) => {
      const from = nodeMap.get(edge.from)
      const to = nodeMap.get(edge.to)
      if (
        !from ||
        !to ||
        from.x === undefined ||
        from.y === undefined ||
        to.x === undefined ||
        to.y === undefined
      ) {
        return ''
      }
      const color = edgeColor(edge.kind)
      const width = Math.min(3, 0.6 + Math.log2(edge.count + 1) * 0.35)
      const mid = from.side === to.side ? (from.x + to.x) / 2 : divider
      return `<path d="M ${from.x} ${from.y} C ${mid} ${from.y}, ${mid} ${to.y}, ${to.x} ${to.y}" fill="none" stroke="${color}" stroke-width="${width.toFixed(2)}" stroke-opacity="0.22"/>`
    })
    .join('\n')
  const nodeSVG = nodes.map(renderNode).join('\n')
  const warningText =
    stats.warnings.length === 0
      ? ''
      : `<text x="40" y="112" class="small warning">${escapeXML(stats.warnings.slice(0, 2).join(' | '))}</text>`

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
<style>
  .title { font: 700 24px ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #17202a; }
  .label { font: 11px ui-monospace, SFMono-Regular, Menlo, monospace; fill: #17202a; dominant-baseline: middle; }
  .small { font: 12px ui-sans-serif, system-ui, sans-serif; fill: #4b5563; }
  .warning { fill: #9a3412; }
  .divider { stroke: #111827; stroke-width: 2; stroke-dasharray: 8 8; }
  .divider-label { font: 700 13px ui-sans-serif, system-ui, sans-serif; fill: #111827; text-anchor: middle; }
</style>
<rect width="100%" height="100%" fill="#f8fafc"/>
<text x="40" y="44" class="title">Spacewave GoScript Package Boundary Sketch</text>
<text x="40" y="72" class="small">Rendered ${stats.renderedNodes}/${stats.totalNodes} nodes and ${stats.renderedEdges}/${stats.totalEdges} edges. Left: Go and generated GoScript output. Right: manual gs overrides and TypeScript.</text>
${warningText}
<line x1="${divider}" y1="90" x2="${divider}" y2="${height - 40}" class="divider"/>
<text x="${divider}" y="122" class="divider-label">Go / generated GoScript  |  manual gs / TypeScript</text>
<g>${edgeSVG}</g>
<g>${nodeSVG}</g>
<g transform="translate(40 ${height - 42})">
  ${legendItem(0, '#2563eb', 'Go package')}
  ${legendItem(160, '#16a34a', 'compiled GoScript output')}
  ${legendItem(390, '#f97316', 'overridden GoScript output')}
  ${legendItem(650, '#dc2626', 'manual gs override')}
  ${legendItem(870, '#7c3aed', 'TypeScript package')}
</g>
</svg>
`
}

function renderNode(node: GraphNode) {
  const x = node.x ?? 0
  const y = node.y ?? 0
  const width = Math.min(360, Math.max(120, node.label.length * 7 + 18))
  const alignRight = node.side === 'left'
  const rectX = alignRight ? x - width : x
  const textX = alignRight ? x - 9 : x + 9
  const anchor = alignRight ? 'end' : 'start'
  return `<g>
  <rect x="${rectX}" y="${y - 9}" width="${width}" height="18" rx="4" fill="${nodeColor(node.kind)}" fill-opacity="0.88"/>
  <title>${escapeXML(`${node.kind}: ${node.packagePath}${node.files ? ` (${node.files} files)` : ''}`)}</title>
  <text x="${textX}" y="${y}" class="label" text-anchor="${anchor}">${escapeXML(node.label)}</text>
</g>`
}

function legendItem(x: number, color: string, label: string) {
  return `<g transform="translate(${x} 0)"><rect width="12" height="12" y="-10" rx="3" fill="${color}" fill-opacity="0.88"/><text x="18" y="0" class="small">${escapeXML(label)}</text></g>`
}

function addNode(nodes: Map<string, GraphNode>, node: GraphNode) {
  if (!nodes.has(node.id)) {
    nodes.set(node.id, node)
  }
}

function addEdge(
  edges: Map<string, GraphEdge>,
  from: string,
  to: string,
  kind: string,
) {
  if (from === to) {
    return
  }
  const key = `${from}\0${to}\0${kind}`
  const existing = edges.get(key)
  if (existing) {
    existing.count += 1
  } else {
    edges.set(key, { from, to, kind, count: 1 })
  }
}

function goID(importPath: string) {
  return `go:${importPath}`
}

function generatedID(packagePath: string) {
  return `generated:${packagePath}`
}

function overrideID(packagePath: string) {
  return `override:${packagePath}`
}

function tsID(packagePath: string) {
  return `ts:${packagePath}`
}

function isRepoGoPackage(importPath: string) {
  return importPath === modulePath || importPath.startsWith(`${modulePath}/`)
}

function isGeneratedTypeScriptPath(file: string) {
  return (
    file.includes('/@goscript/') ||
    file.endsWith('.pb.ts') ||
    file.endsWith('.pb.gs.ts')
  )
}

function isTestFile(file: string) {
  return (
    /\.(test|spec)\.[cm]?[jt]sx?$/u.test(file) ||
    /_test\.[cm]?[jt]sx?$/u.test(file)
  )
}

function leftBand(node: GraphNode) {
  if (node.kind === 'goscript-generated-overridden') {
    return 0
  }
  if (node.kind === 'goscript-generated') {
    return 1
  }
  if (isRepoGoPackage(node.packagePath)) {
    return 2
  }
  return 3
}

function rightBand(node: GraphNode) {
  if (node.kind === 'gs-override') {
    return 0
  }
  return 1
}

function nodeColor(kind: NodeKind) {
  switch (kind) {
    case 'go':
      return '#2563eb'
    case 'goscript-generated':
      return '#16a34a'
    case 'goscript-generated-overridden':
      return '#f97316'
    case 'gs-override':
      return '#dc2626'
    case 'typescript':
      return '#7c3aed'
  }
}

function edgeColor(kind: string) {
  if (kind.includes('override')) {
    return '#dc2626'
  }
  if (kind.includes('goscript') || kind.includes('compiled')) {
    return '#16a34a'
  }
  if (kind.includes('typescript')) {
    return '#7c3aed'
  }
  return '#64748b'
}

function displayPackage(importPath: string) {
  return importPath
    .replace(/^github\.com\/s4wave\/spacewave\/?/u, 'spacewave/')
    .replace(/^github\.com\/aperturerobotics\//u, 'aptre/')
}

function packageFamily(packagePath: string) {
  const parts = packagePath.split('/').filter(Boolean)
  if (
    parts[0] === 'github.com' &&
    parts[1] === 's4wave' &&
    parts[2] === 'spacewave'
  ) {
    return `spacewave/${parts[3] ?? 'root'}`
  }
  if (parts[0] === 'github.com' && parts[1] === 'aperturerobotics') {
    return `aptre/${parts[2] ?? 'root'}`
  }
  if (parts[0] === 'github.com') {
    return parts.slice(0, 3).join('/')
  }
  if (parts[0] === 'golang.org') {
    return parts.slice(0, 3).join('/')
  }
  if (parts[0] === 'internal' || parts.includes('internal')) {
    const internalIndex = parts.indexOf('internal')
    return parts.slice(0, internalIndex + 2).join('/')
  }
  return parts.slice(0, Math.min(2, parts.length)).join('/') || packagePath
}

function shorten(value: string, max: number) {
  if (value.length <= max) {
    return value
  }
  return `${value.slice(0, max - 3)}...`
}

function isInside(root: string, file: string) {
  const rel = path.relative(root, file)
  return rel !== '' && !rel.startsWith('..') && !path.isAbsolute(rel)
}

function slash(value: string) {
  return value.split(path.sep).join('/')
}

function escapeXML(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}

main()
