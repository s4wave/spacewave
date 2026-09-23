import { readFile, writeFile, readdir } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { isBuiltin } from 'node:module'
import { parseSync } from 'oxc-parser'

const output = resolve(process.argv[2] ?? 'packages/spacewave/dist')
const working = resolve('.tmp/sync-library')
async function run(
  command: string[],
  env?: Record<string, string>,
): Promise<string> {
  const child = Bun.spawn(command, {
    stdout: 'pipe',
    stderr: 'pipe',
    env: { ...process.env, ...env },
  })
  const [code, stdout, stderr] = await Promise.all([
    child.exited,
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
  ])
  if (code) throw new Error(`${command[0]} failed: ${stderr}`)
  return stdout.trim()
}

// Reuse the repository's classifier for the exact compiled Go entry point.
const reporter = join(working, 'license-report')
await run(['go', '-C', 'scripts/licenses', 'build', '-o', reporter, '.'])
const goLicenses = JSON.parse(
  await run([reporter, './core/sync/node'], {
    GOOS: 'js',
    GOARCH: 'wasm',
    CGO_ENABLED: '0',
    GOFLAGS: '-mod=readonly -tags=goscript,skip_e2e,purego',
  }),
) as { name: string; version: string; licenseText: string }[]
const notices = new Map<string, string>()
for (const entry of goLicenses)
  notices.set(`${entry.name} ${entry.version}`, entry.licenseText)
const goroot = await run(['go', 'env', 'GOROOT'])
notices.set(
  'Go standard library',
  await readFile(join(goroot, 'LICENSE'), 'utf8').catch(() =>
    readFile(join(dirname(goroot), 'LICENSE'), 'utf8'),
  ),
)
const goscript = await run([
  'go',
  'list',
  '-m',
  '-mod=mod',
  '-f',
  '{{.Dir}}',
  'github.com/s4wave/goscript',
])
notices.set(
  'GoScript runtime',
  await readFile(join(goscript, 'LICENSE'), 'utf8'),
)

// Source import roots plus their package dependencies retain complete npm notices.
const npmRoots = new Set<string>()
for (const file of ['build-report.json', 'client-build-report.json']) {
  const report = JSON.parse(await readFile(join(working, file), 'utf8')) as {
    inputs: string[]
  }
  for (const input of report.inputs) {
    if (!/\.[cm]?[jt]sx?$/.test(input)) continue
    const parsed = parseSync(input, await readFile(input, 'utf8'))
    for (const entry of parsed.module.staticImports) {
      const specifier = entry.moduleRequest.value
      if (
        isBuiltin(specifier) ||
        /^(\.|\/|#|node:|@go\/|@goscript\/|@s4wave\/|@aptre\/bldr)/.test(
          specifier,
        ) ||
        specifier === 'react' ||
        specifier.startsWith('react/')
      )
        continue
      npmRoots.add(
        specifier
          .split('/')
          .slice(0, specifier.startsWith('@') ? 2 : 1)
          .join('/'),
      )
    }
  }
}
const visited = new Set<string>()
async function collect(name: string, from: string): Promise<void> {
  let entry: string
  try {
    entry = Bun.resolveSync(`${name}/package.json`, from)
  } catch {
    entry = Bun.resolveSync(name, from)
  }
  let directory = dirname(entry)
  for (;;) {
    const manifest = (await readFile(
      join(directory, 'package.json'),
      'utf8',
    ).then(
      (text) => JSON.parse(text),
      () => undefined,
    )) as
      | { name: string; version: string; dependencies?: Record<string, string> }
      | undefined
    if (manifest?.name === name) {
      const identity = `${name} ${manifest.version}`
      if (visited.has(identity)) return
      visited.add(identity)
      const files = (await readdir(directory))
        .filter((file) => /^(licen[cs]e|copying|notice)(\.|-|$)/i.test(file))
        .sort()
      let text = (
        await Promise.all(
          files.map((file) => readFile(join(directory, file), 'utf8')),
        )
      ).join('\n\n')
      if (!text) {
        const readme = await readFile(
          join(directory, 'README.md'),
          'utf8',
        ).catch(() => '')
        const license = /^#+ .*licen[cs].*$/im.exec(readme)
        if (license) text = readme.slice(license.index)
      }
      if (!text) throw new Error(`Missing npm attribution: ${identity}`)
      notices.set(identity, text)
      for (const dependency of Object.keys(manifest.dependencies ?? {}))
        await collect(dependency, directory)
      return
    }
    const parent = dirname(directory)
    if (parent === directory)
      throw new Error(`Cannot locate npm manifest: ${name}`)
    directory = parent
  }
}
for (const name of npmRoots) await collect(name, resolve('bldr/dist/deps'))
await writeFile(
  join(output, 'THIRD_PARTY_NOTICES.txt'),
  [...notices]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(
      ([name, text]) =>
        `${name}\n${'='.repeat(name.length)}\n\n${text.trim()}\n`,
    )
    .join('\n'),
)
const revision = await run(['git', 'rev-parse', 'HEAD'])
const dirty = Boolean(
  await run(['git', 'status', '--porcelain', '--untracked-files=normal']),
)
await writeFile(
  join(output, 'build.json'),
  JSON.stringify(
    {
      revision,
      dirty,
      nodeTarget: '>=24.15.0 <25',
      bundledNotices: notices.size,
    },
    null,
    2,
  ) + '\n',
)
