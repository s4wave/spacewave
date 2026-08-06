import { afterEach, expect, test } from 'bun:test'
import { mkdtemp, readdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import type { OutputAsset, OutputChunk } from 'rolldown'
import { build } from 'rolldown'
import { build as viteBuild } from 'vite'

const fixtureRoots: string[] = []

afterEach(async () => {
  await Promise.all(fixtureRoots.splice(0).map((root) => rm(root, { recursive: true })))
})

async function fixtureRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'bldr-rolldown-capabilities-'))
  fixtureRoots.push(root)
  return root
}

function chunkCode(output: (OutputAsset | OutputChunk)[]): string {
  return output
    .filter((item) => item.type === 'chunk')
    .map((item) => item.code)
    .join('\n')
}

test('tree shakes unused exports and retains side effects from virtual modules', async () => {
  const result = await build({
    write: false,
    input: 'bldr:entry',
    output: { format: 'es' },
    plugins: [
      {
        name: 'bldr-capability-virtual-modules',
        resolveId(id) {
          return id.startsWith('bldr:') ? id : null
        },
        load(id) {
          switch (id) {
            case 'bldr:entry':
              return "import { used } from 'bldr:values'; import 'bldr:side-effect'; console.log(used)"
            case 'bldr:values':
              return "export const used = 'used-sentinel'; export const unused = 'unused-sentinel'"
            case 'bldr:side-effect':
              return "globalThis.__bldrSideEffect = 'side-effect-sentinel'"
            default:
              return null
          }
        },
      },
    ],
  })

  const code = chunkCode(result.output)
  expect(code).toContain('used-sentinel')
  expect(code).toContain('side-effect-sentinel')
  expect(code).not.toContain('unused-sentinel')
})

test('converts CommonJS imports into executable ESM', async () => {
  const root = await fixtureRoot()
  const entry = join(root, 'entry.ts')
  await writeFile(
    entry,
    "import value, { named } from './dependency.cjs'; globalThis.__bldrCjs = [value, named]",
  )
  await writeFile(
    join(root, 'dependency.cjs'),
    "module.exports = { default: 'default-sentinel', named: 'named-sentinel' }",
  )

  const result = await build({ input: entry, write: false, output: { format: 'es' } })
  const code = chunkCode(result.output)
  expect(code).toContain('default-sentinel')
  expect(code).toContain('named-sentinel')
  expect(code).toContain('__bldrCjs')
})

test('splits dynamic imports with source maps and normalized output names', async () => {
  const root = await fixtureRoot()
  const entry = join(root, 'entry.ts')
  await writeFile(entry, "export const load = () => import('./lazy')")
  await writeFile(join(root, 'lazy.ts'), "export const value = 'lazy-sentinel'")

  const result = await build({
    write: false,
    input: entry,
    output: {
      format: 'es',
      entryFileNames: 'entry/[name]-[hash].mjs',
      chunkFileNames: 'chunks/[name]-[hash].mjs',
      sourcemap: true,
    },
  })
  const chunks = result.output.filter((item) => item.type === 'chunk')
  expect(chunks).toHaveLength(2)
  expect(chunks.some((chunk) => chunk.isDynamicEntry)).toBe(true)
  for (const chunk of chunks) {
    expect(chunk.fileName.startsWith('/')).toBe(false)
    expect(chunk.fileName.split('/')).not.toContain('..')
    expect(chunk.map).not.toBeNull()
  }
})

test('emits file assets with normalized output paths', async () => {
  const root = await fixtureRoot()
  const entry = join(root, 'entry.ts')
  await writeFile(entry, "import icon from './icon.svg'; console.log(icon)")
  await writeFile(join(root, 'icon.svg'), '<svg xmlns="http://www.w3.org/2000/svg"></svg>')

  const result = await build({
    write: false,
    input: entry,
    moduleTypes: {
      '.svg': 'asset',
    },
    output: {
      format: 'es',
      assetFileNames: 'assets/[name]-[hash][extname]',
    },
  })
  const fileNames = result.output.map((item) => item.fileName)
  expect(fileNames.some((name) => name.startsWith('assets/') && name.endsWith('.svg'))).toBe(true)
  for (const fileName of fileNames) {
    expect(fileName.startsWith('/')).toBe(false)
    expect(fileName.split('/')).not.toContain('..')
  }
})

test('builds CSS and related assets through config-free Vite', async () => {
  const root = await fixtureRoot()
  const entry = join(root, 'entry.ts')
  const outDir = join(root, 'dist')
  await writeFile(entry, "import './style.css'; import icon from './icon.svg'; console.log(icon)")
  await writeFile(join(root, 'style.css'), '.probe { color: rgb(1 2 3); }')
  await writeFile(join(root, 'icon.svg'), '<svg xmlns="http://www.w3.org/2000/svg"></svg>')

  await viteBuild({
    configFile: false,
    root,
    publicDir: false,
    logLevel: 'silent',
    build: {
      outDir,
      emptyOutDir: true,
      assetsInlineLimit: 0,
      manifest: true,
      minify: 'oxc',
      rolldownOptions: { input: entry },
    },
  })
  const fileNames = await readdir(outDir, { recursive: true })
  expect(fileNames.some((name) => name.endsWith('.css'))).toBe(true)
  expect(fileNames.some((name) => name.endsWith('.svg'))).toBe(true)
  expect(fileNames).toContain('.vite/manifest.json')
  for (const fileName of fileNames) {
    expect(fileName.startsWith('/')).toBe(false)
    expect(fileName.split('/')).not.toContain('..')
  }
})
