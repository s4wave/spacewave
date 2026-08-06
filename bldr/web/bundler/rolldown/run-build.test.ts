import { promises as fs } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, it } from 'vitest'
import { runBuild, validateBuildRequest } from './run-build.js'
import type { BuildRequest } from './rolldown.pb.js'

const dependencyRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../dist/deps',
)
const temporaryDirectories: string[] = []

async function makeProject(): Promise<{
  root: string
  output: string
  request: (overrides?: Partial<BuildRequest>) => BuildRequest
}> {
  const root = await fs.mkdtemp(join(tmpdir(), 'bldr-rolldown-test-'))
  temporaryDirectories.push(root)
  const output = join(root, 'dist')
  return {
    root,
    output,
    request(overrides = {}) {
      return {
        workingDir: root,
        sourceRoot: root,
        outputRoot: output,
        entrypoints: [{ name: 'main', inputPath: join(root, 'main.ts') }],
        format: 'es',
        platform: 'browser',
        sourcemap: 'none',
        treeShaking: true,
        ...overrides,
      }
    },
  }
}

afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => fs.rm(directory, { recursive: true, force: true })),
  )
})

describe('direct Rolldown/Oxc owner', () => {
  it('tree-shakes unused exports while retaining side effects and virtual modules', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'side.ts'),
      `export const unused = 1\nconsole.log('side effect')\n`,
    )
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `import './side.js'\nimport { virtualValue } from 'virtual:test'\nconsole.log(virtualValue)\n`,
    )
    const result = await runBuild(
      project.request({
        virtualModules: { 'virtual:test': 'export const virtualValue = 2' },
      }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const output = await fs.readFile(join(project.output, 'main.js'), 'utf8')
    expect(output).toContain('side effect')
    expect(output).not.toContain('unused')
    expect(output).toContain('2')
  })

  it('builds CommonJS output with Oxc/Rolldown', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `module.exports = { value: 3 }\n`,
    )
    const result = await runBuild(
      project.request({ format: 'cjs' }),
      dependencyRoot,
    )
    expect(result.diagnostics).toEqual([])
    expect(
      (result.outputs ?? []).some((output) => output.type === 'javascript'),
    ).toBe(true)
    expect(
      await fs.readFile(join(project.output, 'main.js'), 'utf8'),
    ).toContain('module.exports')
  })

  it('writes dynamic chunks, assets, and external sourcemaps', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'lazy.ts'),
      `export const lazy = 'loaded'\n`,
    )
    await fs.writeFile(
      join(project.root, 'data.bin'),
      new Uint8Array([1, 2, 3, 4]),
    )
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `import dataURL from './data.bin'\nexport { dataURL }\nexport const load = () => import('./lazy.js')\n`,
    )
    const result = await runBuild(
      project.request({
        codeSplitting: true,
        sourcemap: 'both',
        loaders: { '.bin': 'asset' },
        entryFileNames: 'entries/[name].js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name][extname]',
        publicPath: '/static/',
      }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const outputs = result.outputs ?? []
    expect(
      outputs.filter((output) => output.type === 'javascript').length,
    ).toBeGreaterThan(1)
    expect(outputs.some((output) => output.type === 'asset')).toBe(true)
    expect(outputs.some((output) => output.type === 'map')).toBe(true)
    expect((result.entrypointOutputs ?? {}).main).toBe('entries/main.js')
    for (const output of outputs) {
      expect(output.path).not.toMatch(/(^|\/|\\)\.\.($|\/|\\)/)
      expect(output.bytes).toBeGreaterThan(0n)
      expect(output.gzipBytes).toBeGreaterThan(0n)
      expect(output.sha256).toMatch(/^[0-9a-f]{64}$/)
    }
    const main = await fs.readFile(
      join(project.output, 'entries/main.js'),
      'utf8',
    )
    expect(main).toContain('sourceMappingURL=data:application/json;base64,')
  })

  it('reports structured missing-import diagnostics', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `import './does-not-exist.js'\n`,
    )
    const result = await runBuild(project.request(), dependencyRoot)
    const diagnostics = result.diagnostics ?? []
    expect(diagnostics.length).toBeGreaterThan(0)
    expect(
      diagnostics.some((diagnostic) => diagnostic.severity === 'error'),
    ).toBe(true)
    expect(
      diagnostics.some((diagnostic) => (diagnostic.message ?? '').length > 0),
    ).toBe(true)
  })

  it('bundles injected overrides and leaves bare packages external', async () => {
    const project = await makeProject()
    const prelude = join(project.root, 'prelude.js')
    await fs.writeFile(prelude, `console.log('disk')\n`)
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `import value from 'runtime-package/subpath'\nconsole.log(value)\n`,
    )
    const result = await runBuild(
      project.request({
        externalPackages: true,
        external: ['runtime-package'],
        inject: [prelude],
        sourceOverrides: {
          [prelude]: `console.log('override')\n`,
        },
      }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const output = await fs.readFile(join(project.output, 'main.js'), 'utf8')
    expect(output).toContain('override')
    expect(output).not.toContain('disk')
    expect(output).toContain('runtime-package/subpath')
    expect(result.entrypointOutputs?.main).toBe('main.js')
    expect(result.inputs).toContain(await fs.realpath(prelude))
  })

  it('requires a global name for IIFE output', () => {
    expect(() => validateBuildRequest({ format: 'iife' })).toThrow(
      /requires global_name/,
    )
  })

  it('resolves generated imports that escape the flattened Bldr source root', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'go.mod'),
      'module github.com/example/app\n',
    )
    const bldrDistRoot = join(project.root, 'bldr-dist')
    const hostRoot = join(bldrDistRoot, 'sdk', 'plugin', 'host')
    const registryRoot = join(
      bldrDistRoot,
      'vendor',
      'github.com',
      's4wave',
      'spacewave',
      'sdk',
      'objecttype',
      'registry',
    )
    await fs.mkdir(hostRoot, { recursive: true })
    await fs.mkdir(registryRoot, { recursive: true })
    await fs.writeFile(
      join(hostRoot, 'host.pb.ts'),
      `import { registry } from '../../../../sdk/objecttype/registry/registry.pb.js'\nexport { registry }\n`,
    )
    await fs.writeFile(
      join(registryRoot, 'registry.pb.ts'),
      `export const registry = 'escaped-relative-vendor'\n`,
    )
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `import { registry } from 'sdk/plugin/host/host.pb.js'\nconsole.log(registry)\n`,
    )
    const result = await runBuild(
      project.request({ bldrDistRoot }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const output = await fs.readFile(join(project.output, 'main.js'), 'utf8')
    expect(output).toContain('escaped-relative-vendor')
  })

  it('reports CSS-bearing graphs for Vite routing without emitting output', async () => {
    const project = await makeProject()
    await fs.writeFile(join(project.root, 'style.css'), `body { color: red }\n`)
    await fs.writeFile(join(project.root, 'main.ts'), `import './style.css'\n`)
    const result = await runBuild(
      project.request({ routeCssImports: true }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    expect(result.hasCssImports).toBe(true)
    expect(result.outputs ?? []).toEqual([])
  })

  it('rejects CSS loaders before starting Rolldown', () => {
    expect(() => validateBuildRequest({ loaders: { '.css': 'text' } })).toThrow(
      /CSS loader configuration/,
    )
    expect(() => validateBuildRequest({ loaders: { '.txt': 'css' } })).toThrow(
      /CSS loader configuration/,
    )
  })
})
