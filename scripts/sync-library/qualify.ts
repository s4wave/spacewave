import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import {
  appendFile,
  cp,
  mkdir,
  readFile,
  rm,
  writeFile,
} from 'node:fs/promises'
import { dirname, join, relative, resolve } from 'node:path'
import { parseArgs } from 'node:util'
import { gzipSync } from 'node:zlib'
import { parseSync } from 'oxc-parser'

// Qualification installs one tarball and exercises only its public consumer APIs.
const { values: options } = parseArgs({
  args: process.argv.slice(2),
  options: {
    'skip-build': { type: 'boolean' },
    'allow-dirty': { type: 'boolean' },
    'release-tag': { type: 'string' },
  },
})
const root = process.cwd()
const working = join(root, '.tmp/sync-qualification')
const packageDirectory = join(working, 'package')
const consumer = join(working, 'consumer')
const artifacts = resolve(
  process.env.SYNC_ARTIFACT_DIRECTORY ?? '.bldr-dist/sync-library',
)
const node = resolve(process.env.SYNC_NODE_BINARY ?? Bun.which('node') ?? '')
const bun = process.execPath
const checks: { name: string; elapsedMs: number }[] = []

// Bind release metadata to the application version before building anything.
const releaseTag = options['release-tag']
if (releaseTag) {
  assert.match(releaseTag, /^v\d+\.\d+\.\d+(?:-[\dA-Za-z.-]+)?$/)
  assert.equal(
    releaseTag,
    'v' + (await readFile('core/appversion/version.txt', 'utf8')).trim(),
    'The npm release tag must match the application version',
  )
}

// Recreate the isolated consumer while retaining completed release artifacts.
await mkdir(artifacts, { recursive: true })
await rm(working, { recursive: true, force: true })
await mkdir(consumer, { recursive: true })

/** run records a check's output and duration, rejecting unsuccessful commands. */
async function run(
  name: string,
  cmd: string[],
  cwd = root,
  env: Record<string, string> = {},
): Promise<string> {
  // Start the command with a dedicated log and the requested environment.
  const started = performance.now()
  const log = join(working, name + '.log')
  await writeFile(log, '')
  const child = Bun.spawn(cmd, {
    cwd,
    env: { ...process.env, ...env },
    stdout: 'pipe',
    stderr: 'pipe',
  })

  // Drain both streams while retaining their text for failure diagnostics.
  async function consume(stream: ReadableStream<Uint8Array>): Promise<string> {
    const parts: Uint8Array[] = []
    for await (const chunk of stream) {
      parts.push(chunk)
      await appendFile(log, chunk)
    }
    return Buffer.concat(parts).toString('utf8')
  }
  const [stdout, stderr, code] = await Promise.all([
    consume(child.stdout),
    consume(child.stderr),
    child.exited,
  ])
  if (code) {
    throw new Error(`${name} failed (${code}):\n${stdout}\n${stderr}`)
  }

  // Include only successful checks in the qualification report.
  checks.push({ name, elapsedMs: performance.now() - started })
  console.log(`Passed ${name}`)
  return stdout
}

// Build the library using its supported Node runtime and the current source.
const version = (await run('node-version', [node, '--version'])).trim()
assert.match(
  version,
  /^v24\./,
  'Set SYNC_NODE_BINARY to supported Node 24.15 or later within 24.x',
)
assert.ok(Number(version.split('.')[1]) >= 15)
if (!options['skip-build']) {
  await run('source-types', [bun, 'run', 'typecheck'])
  await run('build', ['go', 'run', './scripts/sync-library'])
}

// Require reproducible source metadata unless the qualification run is an
// explicit development run.
const metadata = JSON.parse(
  await readFile('packages/spacewave/dist/build.json', 'utf8'),
)
if (!options['allow-dirty']) {
  assert.equal(
    metadata.dirty,
    false,
    'Release qualification requires a clean source revision',
  )
}

// Stage versioned metadata without changing the source revision being qualified.
await cp(join(root, 'packages/spacewave'), packageDirectory, {
  recursive: true,
})
if (releaseTag) {
  // Stamp the staged library with the application release version.
  const packageVersion = releaseTag.slice(1)
  await run(
    'release-version',
    [
      'npm',
      'version',
      packageVersion,
      '--no-git-tag-version',
      '--allow-same-version',
      '--ignore-scripts',
    ],
    packageDirectory,
  )

  // Make the bundled example install this same published version.
  const examplePath = join(packageDirectory, 'examples/task-board/package.json')
  const example = JSON.parse(await readFile(examplePath, 'utf8'))
  example.dependencies.spacewave = packageVersion
  await writeFile(examplePath, JSON.stringify(example, null, 2) + '\n')
}

// Pack once so every consumer check and publication use identical bytes.
const packed = JSON.parse(
  await run(
    'pack',
    [
      'npm',
      'pack',
      '--ignore-scripts',
      '--json',
      '--pack-destination',
      artifacts,
    ],
    packageDirectory,
  ),
)[0] as {
  filename: string
  version: string
  size: number
  unpackedSize: number
}
if (releaseTag) assert.equal(packed.version, releaseTag.slice(1))

// Record the archive digest before installing it into the consumer.
const tarball = join(artifacts, packed.filename)
const sha256 = createHash('sha256')
  .update(await readFile(tarball))
  .digest('hex')
await writeFile(tarball + '.sha256', `${sha256}  ${packed.filename}\n`)

// Install public dependencies without workspace aliases or lifecycle scripts.
await writeFile(
  join(consumer, 'package.json'),
  JSON.stringify(
    {
      private: true,
      type: 'module',
      dependencies: {
        spacewave: `file:${tarball}`,
        '@standard-schema/spec': '1.1.0',
        react: '19.2.8',
        'react-dom': '19.2.8',
        '@types/react': '19.2.18',
        '@types/react-dom': '19.2.4',
        '@types/node': '24.13.3',
        typescript: '7.0.2',
        esbuild: '0.27.0',
        playwright: '1.62.1',
        zod: '4.5.4',
      },
    },
    null,
    2,
  ),
)
await run('install', [bun, 'install', '--ignore-scripts'], consumer)

// Verify package provenance and run standalone consumers without executable lookup.
const distribution = join(consumer, 'node_modules/spacewave/dist')
assert.deepEqual(
  JSON.parse(await readFile(join(distribution, 'build.json'), 'utf8')),
  metadata,
)
const noGo = { PATH: '' }
await run(
  'ssr-import',
  [
    node,
    '--input-type=module',
    '-e',
    "await import('spacewave'); await import('spacewave/server'); await import('spacewave/react')",
  ],
  consumer,
  noGo,
)

/** bundle compiles a test fixture while keeping its consumer package imports external. */
async function bundle(
  source: string,
  target: string,
  browser = false,
  external: string[] = ['spacewave', 'playwright'],
): Promise<void> {
  await run('bundle-' + target.replaceAll('.', '-'), [
    bun,
    'build',
    source,
    '--target=' + (browser ? 'browser' : 'node'),
    '--outfile=' + join(consumer, target),
    ...external.flatMap((name) => ['--external', name]),
  ])
}

// Exercise public APIs through independent server and client processes.
await bundle('scripts/sync-library/consumer.test.ts', 'consumer.test.mjs')
await bundle('scripts/sync-library/live.test.ts', 'live.test.mjs')
await run(
  'public-process',
  [node, '--test', 'consumer.test.mjs', 'live.test.mjs'],
  consumer,
  noGo,
)

// Check transaction guarantees against the engine shipped in the tarball.
const internal = join(working, 'internal')
await cp(distribution, internal, { recursive: true })
await run('bundle-internal', [
  bun,
  'build',
  'scripts/sync-library/application.test.ts',
  '--target=node',
  '--outfile=' + join(internal, 'application.test.mjs'),
])
await run(
  'internal-transactions',
  [node, '--test', 'application.test.mjs'],
  internal,
  noGo,
)

// Type-check consumers against the package's declarations and optional React API.
await cp(
  'scripts/sync-library/types.test.tsx',
  join(consumer, 'types.test.tsx'),
)
await writeFile(
  join(consumer, 'tsconfig.json'),
  JSON.stringify(
    {
      compilerOptions: {
        strict: true,
        noEmit: true,
        target: 'ES2024',
        module: 'NodeNext',
        moduleResolution: 'NodeNext',
        jsx: 'react-jsx',
        lib: ['ES2024', 'DOM', 'ESNext.Disposable'],
        types: ['node', 'react'],
        skipLibCheck: false,
      },
      include: ['types.test.tsx'],
    },
    null,
    2,
  ),
)
await run('consumer-types', [bun, 'x', 'tsrs', '-p', 'tsconfig.json'], consumer)

// Exercise browser subscriptions with the system utilities their launchers require.
await bundle('e2e/sync-library/browser.ts', 'browser.mjs', true, ['spacewave'])
await cp('e2e/sync-library/react.tsx', join(consumer, 'react-fixture.tsx'))
await cp('e2e/sync-library/schema.ts', join(consumer, 'schema.ts'))
await run(
  'bundle-react',
  [
    bun,
    'build',
    'react-fixture.tsx',
    '--target=browser',
    '--outfile=react-fixture.mjs',
  ],
  consumer,
)
await bundle('e2e/sync-library/journey.test.ts', 'journey.test.mjs')
await run('browser-journey', [node, '--test', 'journey.test.mjs'], consumer)

// Copy the published example and confirm that it selects this package version.
const example = join(working, 'example')
await cp(
  join(consumer, 'node_modules/spacewave/examples/task-board'),
  example,
  { recursive: true },
)
const exampleManifest = JSON.parse(
  await readFile(join(example, 'package.json'), 'utf8'),
)
assert.equal(exampleManifest.dependencies.spacewave, packed.version)

// Run the example against the exact archive before it reaches the registry.
exampleManifest.dependencies.spacewave = `file:${tarball}`
await writeFile(
  join(example, 'package.json'),
  JSON.stringify(exampleManifest, null, 2),
)
await run('example-install', [bun, 'install', '--ignore-scripts'], example)
await run('example-types', [bun, 'x', 'tsrs', '-p', 'tsconfig.json'], example)
await bundle('e2e/sync-library/example.test.ts', 'example.test.mjs')
await run('example-browser', [node, '--test', 'example.test.mjs'], consumer, {
  SYNC_EXAMPLE_DIRECTORY: example,
})

/** closure measures an entry point and follows its static imports within the package. */
async function closure(entry: string): Promise<{
  files: string[]
  bytes: number
  gzipBytes: number
  external: string[]
}> {
  // Count each module once, retaining external imports for boundary checks.
  const files = new Set<string>()
  const external = new Set<string>()
  let bytes = 0
  let gzipBytes = 0

  /** visit accumulates one module and recursively follows its relative imports. */
  async function visit(path: string): Promise<void> {
    // Skip modules already reached through another import.
    if (files.has(path)) return
    files.add(path)

    // Account for this module's source and compressed size.
    const text = await readFile(path, 'utf8')
    bytes += Buffer.byteLength(text)
    gzipBytes += gzipSync(text).length

    // Traverse package imports and retain external dependency names.
    const parsed = parseSync(path, text)
    assert.equal(parsed.errors.length, 0)
    for (const imported of parsed.module.staticImports) {
      const name = imported.moduleRequest.value
      if (name.startsWith('.')) {
        await visit(resolve(dirname(path), name))
      } else {
        external.add(name)
      }
    }
  }

  // Return package-relative paths so reports remain portable across runners.
  await visit(join(distribution, entry + '.mjs'))
  return {
    files: [...files].map((file) => relative(distribution, file)),
    bytes,
    gzipBytes,
    external: [...external].sort(),
  }
}

// Measure each public entry point and the worker it loads.
const bundles = Object.fromEntries(
  await Promise.all(
    ['index', 'react', 'server', 'engine-worker'].map(async (name) => [
      name,
      await closure(name),
    ]),
  ),
)

// Reject application-only Go packages in the embedded sync engine.
const buildReport = JSON.parse(
  await readFile('.tmp/sync-library/build-report.json', 'utf8'),
) as { inputs: string[] }
const compiledPackages = [
  ...new Set(
    buildReport.inputs.flatMap((path) => {
      const source = path.split('/@goscript/github.com/s4wave/spacewave/')[1]
      return source ? [dirname(source)] : []
    }),
  ),
].sort()
const forbidden = [
  'app/',
  'core/app/',
  'core/session/',
  'net/transport/',
  'net/webrtc/',
  'net/circuit/',
]
assert.ok(
  compiledPackages.every((path) =>
    forbidden.every((prefix) => !(path + '/').startsWith(prefix)),
  ),
)

// Keep Node and React dependencies confined to their declared entry points.
assert.deepEqual(bundles.index.external, [])
assert.ok(
  bundles.react.external.every(
    (name: string) => name === 'react' || name === 'react/jsx-runtime',
  ),
)
assert.ok(
  bundles.server.external.every((name: string) => name.startsWith('node:')),
)
assert.ok(
  bundles['engine-worker'].external.every((name: string) =>
    name.startsWith('node:'),
  ),
)

// Measure one bounded workload through the installed public package.
await bundle('scripts/sync-library/workload.ts', 'workload.mjs')
const workloadLog = await run(
  'workload',
  [node, 'workload.mjs'],
  consumer,
  noGo,
)
const workload = JSON.parse(
  workloadLog
    .trim()
    .split('\n')
    .findLast((line) => line.startsWith('{'))!,
)

// Retain build identity, package boundaries, timings, and workload evidence.
const report = {
  artifact: packed.filename,
  version: packed.version,
  sha256,
  ...metadata,
  node: version,
  platform: process.platform,
  arch: process.arch,
  packedBytes: packed.size,
  unpackedBytes: packed.unpackedSize,
  bundles,
  compiledPackages,
  workload,
  checks,
}
await writeFile(
  join(artifacts, 'qualification.json'),
  JSON.stringify(report, null, 2) + '\n',
)
console.log(JSON.stringify(report, null, 2))
