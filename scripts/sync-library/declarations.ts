import { mkdir, readFile, copyFile, writeFile } from 'node:fs/promises'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { parseSync, Visitor } from 'oxc-parser'

// TypeScript owns declaration generation. Assembly copies only public reachable
// modules.
const output = resolve(process.argv[2] ?? 'packages/spacewave/dist')
const source = resolve('.tmp/sync-library/types')
const compiler = Bun.spawn(
  ['bunx', 'tsrs', '-p', 'scripts/sync-library/tsconfig.json'],
  { stdout: 'inherit', stderr: 'inherit' },
)
if (await compiler.exited)
  throw new Error('Public declaration generation failed')

const visited = new Set<string>()
const external = new Set<string>()
async function copy(path: string): Promise<void> {
  if (visited.has(path)) return
  if (!path.startsWith(source + sep))
    throw new Error('Declaration escapes package')
  visited.add(path)
  const text = await readFile(path, 'utf8')
  const parsed = parseSync(path, text)
  if (parsed.errors.length)
    throw new Error(`Invalid generated declaration: ${path}`)
  const imports = new Set<string>()
  new Visitor({
    ImportDeclaration: (node) => {
      imports.add(node.source.value)
    },
    ExportNamedDeclaration: (node) => {
      if (node.source) imports.add(node.source.value)
    },
    ExportAllDeclaration: (node) => {
      imports.add(node.source.value)
    },
    TSImportType: (node) => {
      imports.add(node.source.value)
    },
  }).visit(parsed.program)
  for (const specifier of imports) {
    if (specifier.startsWith('.')) {
      await copy(resolve(dirname(path), specifier.replace(/\.js$/, '.d.ts')))
    } else {
      if (
        ![
          '@standard-schema/spec',
          'react',
          'react/jsx-runtime',
          'node:http',
        ].includes(specifier)
      )
        throw new Error(`Undeclared public type dependency: ${specifier}`)
      external.add(specifier)
    }
  }
  const target = join(output, 'types', relative(source, path))
  await mkdir(dirname(target), { recursive: true })
  await copyFile(path, target)
}
for (const name of ['index', 'server', 'react'])
  await copy(join(source, `packages/spacewave/${name}.d.ts`))
await writeFile(
  '.tmp/sync-library/type-report.json',
  JSON.stringify(
    {
      files: [...visited].map((path) => relative(source, path)).sort(),
      external: [...external].sort(),
    },
    null,
    2,
  ) + '\n',
)
