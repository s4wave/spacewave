// @vitest-environment node

import { promises as fs } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { runBuild, validateBuildRequest } from './run-build.js'
import type { BuildRequest } from './rolldown.pb.js'

const dependencyRoot = resolve('bldr/dist/deps')
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
  it('resolves source dependencies when the working directory is elsewhere', async () => {
    const project = await makeProject()
    const packageRoot = join(project.root, 'node_modules', 'project-value')
    await fs.mkdir(packageRoot, { recursive: true })
    await fs.writeFile(
      join(packageRoot, 'package.json'),
      JSON.stringify({
        name: 'project-value',
        type: 'module',
        exports: './index.js',
      }),
    )
    await fs.writeFile(
      join(packageRoot, 'index.js'),
      "export const value = 'source-owned-dependency'\n",
    )
    await fs.writeFile(
      join(project.root, 'main.ts'),
      "import { value } from 'project-value'\nimport { z } from 'zod'\nconsole.log(z.string().parse(value))\n",
    )
    const result = await runBuild(
      project.request({ workingDir: project.output }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    expect(
      await fs.readFile(join(project.output, 'main.js'), 'utf8'),
    ).toContain('source-owned-dependency')
    expect(result.inputs).toContain(
      await fs.realpath(join(packageRoot, 'index.js')),
    )
  })

  it('shares SDK module identity with local imports', async () => {
    const project = await makeProject()
    const distRoot = join(project.root, 'packaged')
    for (const root of [join(project.root, 'bldr'), distRoot]) {
      await fs.mkdir(join(root, 'sdk'), { recursive: true })
      await fs.writeFile(
        join(root, 'sdk', 'plugin.ts'),
        'export const context = {}\n',
      )
    }
    await fs.writeFile(
      join(project.root, 'main.ts'),
      [
        "import { context as publicContext } from '@aptre/bldr-sdk'",
        "import { context as localContext } from './bldr/sdk/plugin.js'",
        'export const same = publicContext === localContext',
      ].join('\n'),
    )
    const result = await runBuild(
      project.request({ bldrDistRoot: distRoot, format: 'cjs' }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const loaded = { exports: {} as { same: boolean } }
    const code = await fs.readFile(join(project.output, 'main.js'), 'utf8')
    new Function('module', 'exports', code)(loaded, loaded.exports)
    expect(loaded.exports.same).toBe(true)
    expect(result.inputs).toContain(
      await fs.realpath(join(project.root, 'bldr', 'sdk', 'plugin.ts')),
    )
    expect(result.inputs).not.toContain(join(distRoot, 'sdk', 'plugin.ts'))
  })

  it('keeps the default export of an injected entry', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'inject.js'),
      `console.log('inject side effect')\n`,
    )
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `export default async function main() {}\n`,
    )
    const result = await runBuild(
      project.request({
        inject: [join(project.root, 'inject.js')],
      }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const output = await fs.readFile(join(project.output, 'main.js'), 'utf8')
    expect(output).toContain('inject side effect')
    // The emitted module carries a default export in either accepted shape.
    const hasDefault =
      /\bexport\s*\{[^}]*\bas\s+default\b[^}]*\}/.test(output) ||
      /\bexport\s+default\b/.test(output)
    expect(hasDefault).toBe(true)
  })

  it('builds an injected self-executing entry without exports', async () => {
    const project = await makeProject()
    await fs.writeFile(
      join(project.root, 'inject.js'),
      `console.log('inject side effect')\n`,
    )
    await fs.writeFile(
      join(project.root, 'main.ts'),
      `console.log('self executing entry')\n`,
    )
    const result = await runBuild(
      project.request({
        inject: [join(project.root, 'inject.js')],
      }),
      dependencyRoot,
    )
    expect(result.diagnostics ?? []).toEqual([])
    const output = await fs.readFile(join(project.output, 'main.js'), 'utf8')
    expect(output).toContain('inject side effect')
    expect(output).toContain('self executing entry')
  })

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

  it.each(['./does-not-exist.js', 'missing-codec-package/stream.js'])(
    'rejects an unresolved import: %s',
    async (specifier) => {
      const project = await makeProject()
      await fs.writeFile(
        join(project.root, 'main.ts'),
        `import '${specifier}'\n`,
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
    },
  )

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
