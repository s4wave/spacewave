import { describe, expect, it } from 'vitest'
import { promises as fs } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { build as viteBuild } from 'vite'

import { withDependencyRoot } from './dependency-root.js'

import { buildStableEntryFileName } from './output-naming.js'
import { buildWebPkgImportSpecifier } from './web-pkg-naming.js'

describe('buildStableEntryFileName', () => {
  it('keeps plugin root entry module paths stable across platform builds', () => {
    const rootDir = path.join(path.sep, 'repo', 'game')

    expect(
      buildStableEntryFileName(
        rootDir,
        path.join(rootDir, 'web', 'entry.tsx'),
        'entry',
      ),
    ).toBe('web/entry.mjs')
  })

  it('uses the chunk name for synthetic entries without content hashes', () => {
    expect(buildStableEntryFileName('/repo/game', null, 'frontend')).toBe(
      'frontend.mjs',
    )
  })
})

describe('buildWebPkgImportSpecifier', () => {
  it('publishes a non-index package root as the bare package specifier', () => {
    expect(
      buildWebPkgImportSpecifier(
        'non-index-root',
        'build/foo.module',
        'build/foo.module',
      ),
    ).toBe('non-index-root')
    expect(
      buildWebPkgImportSpecifier(
        'non-index-root',
        'examples/extra',
        'build/foo.module',
      ),
    ).toBe('non-index-root/examples/extra')
  })
})

describe('withDependencyRoot', () => {
  it('resolves package imports from a separate installed dependency root', async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), 'bldr-vite-deps-'))
    const packageRoot = path.join(root, 'package')
    const dependencyRoot = path.join(root, 'dependencies')
    const outputRoot = path.join(root, 'out')
    try {
      await fs.mkdir(packageRoot, { recursive: true })
      for (const [name, source] of [
        ['it-pushable', 'export const source = "separate-it-pushable"'],
        ['ulid', 'export const source = "separate-ulid"'],
      ]) {
        const moduleRoot = path.join(dependencyRoot, 'node_modules', name)
        await fs.mkdir(moduleRoot, { recursive: true })
        await fs.writeFile(
          path.join(moduleRoot, 'package.json'),
          JSON.stringify({ name, type: 'module', exports: './index.js' }),
        )
        await fs.writeFile(path.join(moduleRoot, 'index.js'), source)
      }
      const entry = path.join(packageRoot, 'entry.js')
      await fs.writeFile(
        entry,
        `import { source as pushable } from 'it-pushable'
import { source as id } from 'ulid'
export { pushable, id }
`,
      )

      await viteBuild(
        withDependencyRoot({
          root: packageRoot,
          configFile: false,
          build: {
            outDir: outputRoot,
            lib: { entry, formats: ['es'] },
            emptyOutDir: true,
            minify: false,
            rolldownOptions: {
              input: entry,
              output: {
                format: 'es',
                entryFileNames: 'entry.js',
                preserveModules: true,
              },
            },
          },
        }, dependencyRoot),
      )

      const output = (
        await Promise.all(
          ['entry.js', 'entry2.js', 'entry3.js'].map((name) =>
            fs.readFile(path.join(outputRoot, name), 'utf8'),
          ),
        )
      ).join('\n')
      expect(output).toContain('separate-it-pushable')
      expect(output).toContain('separate-ulid')
    } finally {
      await fs.rm(root, { recursive: true, force: true })
    }
  })
})
