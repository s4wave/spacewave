import { describe, it, expect, beforeAll, afterAll, vi } from 'vitest'
import { promises as fs } from 'node:fs'
import path from 'node:path'
import os from 'node:os'
import type { Rollup } from 'vite'

import { analyzeManifest, buildAndAnalyze, buildConfig } from './build.js'
import { makeModulePreloadHelperWorkerSafe } from './module-preload.js'

describe('Vite Build - Transitive Dependency Tracking', () => {
  let testDir: string
  let distDir: string

  beforeAll(async () => {
    // Create a temporary test directory
    testDir = await fs.mkdtemp(path.join(os.tmpdir(), 'vite-test-'))
    distDir = path.join(testDir, 'dist')
    await fs.mkdir(distDir, { recursive: true })
    await fs.mkdir(path.join(distDir, '.vite'), { recursive: true })

    // Create test files: A.tsx -> B.tsx -> C.tsx
    await fs.writeFile(
      path.join(testDir, 'A.tsx'),
      `import { b } from './B.js'\nexport function a() { return b() }`,
    )
    await fs.writeFile(
      path.join(testDir, 'B.tsx'),
      `import { c } from './C.js'\nexport function b() { return c() }`,
    )
    await fs.writeFile(
      path.join(testDir, 'C.tsx'),
      `export function c() { return 'hello' }`,
    )
  })

  afterAll(async () => {
    // Clean up test directory
    await fs.rm(testDir, { recursive: true, force: true })
  })

  describe('analyzeManifest', () => {
    it('does not write to stderr for missing optional config files', async () => {
      const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
      try {
        await buildConfig(
          { mode: 'development', command: 'build' },
          path.join(testDir, 'missing-vite.config.ts'),
        )
        expect(warn).not.toHaveBeenCalled()
      } finally {
        warn.mockRestore()
      }
    })

    it('should track all transitive dependencies in chunk moduleIds', async () => {
      // Create a mock manifest
      const manifest = {
        'A.tsx': {
          file: 'assets/A-hash123.mjs',
          isEntry: true,
          src: 'A.tsx',
        },
      }

      // Write the manifest file
      await fs.writeFile(
        path.join(distDir, '.vite/manifest.json'),
        JSON.stringify(manifest, null, 2),
      )

      // Create mock output chunks simulating what Rollup would produce
      // The key test: moduleIds should include A.tsx, B.tsx, AND C.tsx
      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          // This is the critical part: Rollup should include all transitively imported files
          moduleIds: [
            path.join(testDir, 'A.tsx'),
            path.join(testDir, 'B.tsx'),
            path.join(testDir, 'C.tsx'), // Transitive dependency
          ],
          // Minimal required fields for OutputChunk
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
        } as unknown as Rollup.OutputChunk,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)

      // Verify that all files are tracked
      expect(analysis.entrypointOutputs).toHaveLength(1)
      const entrypoint = analysis.entrypointOutputs[0]
      expect(entrypoint.entrypoint).toBe('A.tsx')

      // The critical assertion: all three files should be in the inputs array
      expect(entrypoint.inputs).toContain('A.tsx')
      expect(entrypoint.inputs).toContain('B.tsx')
      expect(entrypoint.inputs).toContain('C.tsx')
      expect(entrypoint.inputs.length).toBe(3)
    })

    it('should handle multiple entrypoints with shared dependencies', async () => {
      // Create an additional entry D.tsx that also imports C.tsx
      await fs.writeFile(
        path.join(testDir, 'D.tsx'),
        `import { c } from './C.js'\nexport function d() { return c() }`,
      )

      const manifest = {
        'A.tsx': {
          file: 'assets/A-hash123.mjs',
          isEntry: true,
          src: 'A.tsx',
        },
        'D.tsx': {
          file: 'assets/D-hash456.mjs',
          isEntry: true,
          src: 'D.tsx',
        },
      }

      await fs.writeFile(
        path.join(distDir, '.vite/manifest.json'),
        JSON.stringify(manifest, null, 2),
      )

      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          moduleIds: [
            path.join(testDir, 'A.tsx'),
            path.join(testDir, 'B.tsx'),
            path.join(testDir, 'C.tsx'),
          ],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
        } as unknown as Rollup.OutputChunk,
        {
          type: 'chunk',
          fileName: 'assets/D-hash456.mjs',
          name: 'D',
          facadeModuleId: path.join(testDir, 'D.tsx'),
          moduleIds: [
            path.join(testDir, 'D.tsx'),
            path.join(testDir, 'C.tsx'), // Shared dependency
          ],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/D-hash456.mjs',
        } as unknown as Rollup.OutputChunk,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)

      expect(analysis.entrypointOutputs).toHaveLength(2)

      const entryA = analysis.entrypointOutputs.find(
        (e) => e.entrypoint === 'A.tsx',
      )
      const entryD = analysis.entrypointOutputs.find(
        (e) => e.entrypoint === 'D.tsx',
      )

      expect(entryA).toBeDefined()
      expect(entryD).toBeDefined()

      // Both should track C.tsx
      expect(entryA!.inputs).toContain('C.tsx')
      expect(entryD!.inputs).toContain('C.tsx')
    })

    it('should track CSS files from transitive imports', async () => {
      const manifest = {
        'A.tsx': {
          file: 'assets/A-hash123.mjs',
          isEntry: true,
          src: 'A.tsx',
          css: ['assets/A-hash123.css'],
          imports: ['B.tsx'],
        },
        'B.tsx': {
          file: 'assets/B-hash456.mjs',
          css: ['assets/B-hash456.css'], // CSS from transitive import
        },
      }

      await fs.writeFile(
        path.join(distDir, '.vite/manifest.json'),
        JSON.stringify(manifest, null, 2),
      )

      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          moduleIds: [path.join(testDir, 'A.tsx'), path.join(testDir, 'B.tsx')],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
        } as unknown as Rollup.OutputChunk,
        {
          type: 'asset',
          fileName: 'assets/A-hash123.css',
          name: 'A.css',
          source: '',
          needsCodeReference: false,
        } as unknown as Rollup.OutputAsset,
        {
          type: 'asset',
          fileName: 'assets/B-hash456.css',
          name: 'B.css',
          source: '',
          needsCodeReference: false,
        } as unknown as Rollup.OutputAsset,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)

      const entryA = analysis.entrypointOutputs.find(
        (e) => e.entrypoint === 'A.tsx',
      )

      expect(entryA).toBeDefined()
      // Should include both direct and transitive CSS
      expect(entryA!.outputs.css).toContain('assets/A-hash123.css')
      expect(entryA!.outputs.css).toContain('assets/B-hash456.css')
    })

    it('should not classify Vite-emitted JS assets as entrypoint CSS', async () => {
      const manifest = {
        'A.tsx': {
          file: 'assets/A-hash123.mjs',
          isEntry: true,
          src: 'A.tsx',
          imports: ['B.tsx'],
        },
        'B.tsx': {
          file: 'assets/B-hash456.mjs',
          assets: ['assets/opfs-worker-hash.js'],
        },
      }

      await fs.writeFile(
        path.join(distDir, '.vite/manifest.json'),
        JSON.stringify(manifest, null, 2),
      )

      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          moduleIds: [path.join(testDir, 'A.tsx'), path.join(testDir, 'B.tsx')],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
        } as unknown as Rollup.OutputChunk,
        {
          type: 'asset',
          fileName: 'assets/opfs-worker-hash.js',
          name: 'opfs-worker.js',
          source: '',
          needsCodeReference: false,
        } as unknown as Rollup.OutputAsset,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)

      const entryA = analysis.entrypointOutputs.find(
        (e) => e.entrypoint === 'A.tsx',
      )

      expect(entryA).toBeDefined()
      expect(entryA!.outputs.css).not.toContain('assets/opfs-worker-hash.js')
    })

    it('should ignore synthetic vite and rolldown module ids', async () => {
      const manifest = {
        'A.tsx': {
          file: 'assets/A-hash123.mjs',
          isEntry: true,
          src: 'A.tsx',
        },
      }

      await fs.writeFile(
        path.join(distDir, '.vite/manifest.json'),
        JSON.stringify(manifest, null, 2),
      )

      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          moduleIds: [
            path.join(testDir, 'A.tsx'),
            '__vite-browser-external',
            '__vite-browser-external?commonjs-proxy',
            'builtin:esm-external-require-react',
          ],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
        } as unknown as Rollup.OutputChunk,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)
      const entryA = analysis.entrypointOutputs[0]

      expect(entryA.inputs).toContain('A.tsx')
      expect(entryA.inputs).toEqual(['A.tsx'])
    })

    it('should normalize escaped relative module ids back into the repo root', async () => {
      const nodeModulePath = path.join(
        testDir,
        'node_modules',
        '@aptre',
        'it-ws',
        'dist',
        'src',
      )
      await fs.mkdir(nodeModulePath, { recursive: true })
      await fs.writeFile(
        path.join(nodeModulePath, 'duplex.js'),
        `export function duplex() { return null }`,
      )

      const manifest = {
        'A.tsx': {
          file: 'assets/A-hash123.mjs',
          isEntry: true,
          src: 'A.tsx',
        },
      }

      await fs.writeFile(
        path.join(distDir, '.vite/manifest.json'),
        JSON.stringify(manifest, null, 2),
      )

      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          moduleIds: [
            path.join(testDir, 'A.tsx'),
            '../../../../../../../../node_modules/@aptre/it-ws/dist/src/duplex.js',
          ],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: [],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
        } as unknown as Rollup.OutputChunk,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)
      const entryA = analysis.entrypointOutputs[0]

      expect(entryA.inputs).toContain('A.tsx')
      expect(entryA.inputs).toContain(
        path.join(
          'node_modules',
          '@aptre',
          'it-ws',
          'dist',
          'src',
          'duplex.js',
        ),
      )
      expect(entryA.inputs).not.toContain(
        '../../../../../../../../node_modules/@aptre/it-ws/dist/src/duplex.js',
      )
    })

    it('should synthesize entry analysis when the Vite manifest is missing', async () => {
      await fs.rm(path.join(distDir, '.vite'), { recursive: true, force: true })

      const outputChunks: (Rollup.OutputChunk | Rollup.OutputAsset)[] = [
        {
          type: 'chunk',
          fileName: 'assets/A-hash123.mjs',
          name: 'A',
          facadeModuleId: path.join(testDir, 'A.tsx'),
          moduleIds: [
            path.join(testDir, 'A.tsx'),
            path.join(testDir, 'B.tsx'),
            path.join(testDir, 'C.tsx'),
          ],
          code: '',
          dynamicImports: [],
          exports: [],
          implicitlyLoadedBefore: [],
          importedBindings: {},
          imports: [],
          isDynamicEntry: false,
          isEntry: true,
          isImplicitEntry: false,
          map: null,
          modules: {},
          referencedFiles: ['assets/A-hash123.css'],
          sourcemapFileName: null,
          preliminaryFileName: 'assets/A-hash123.mjs',
          viteMetadata: {
            importedCss: new Set(['assets/A-hash123.css']),
          },
        } as unknown as Rollup.OutputChunk,
      ]

      const analysis = await analyzeManifest(distDir, outputChunks, testDir)

      expect(analysis.entrypointOutputs).toHaveLength(1)
      const entryA = analysis.entrypointOutputs[0]
      expect(entryA.entrypoint).toBe('A.tsx')
      expect(entryA.inputs).toContain('A.tsx')
      expect(entryA.inputs).toContain('B.tsx')
      expect(entryA.inputs).toContain('C.tsx')
      expect(entryA.outputs.css).toContain('assets/A-hash123.css')
    })
  })
})

describe('makeModulePreloadHelperWorkerSafe', () => {
  it('guards the generated preload helper for worker imports', () => {
    const code = `//#region \\0vite/preload-helper.js
var __vitePreload = function preload(baseModule, deps) {
  if (deps && deps.length > 0) {
    const links = document.getElementsByTagName("link");
  }
  function handlePreloadError(err) {
    window.dispatchEvent(err)
  }
};`

    const got = makeModulePreloadHelperWorkerSafe(code)

    expect(got).toContain(
      'typeof document !== "undefined" && deps && deps.length > 0',
    )
    expect(got).toContain(
      'typeof window !== "undefined" && window.dispatchEvent(err)',
    )
  })

  it('guards minified preload helpers too', () => {
    const code =
      '//#region \\0vite/preload-helper.js\nvar r=function(r,i){if(i&&i.length>0){const s=document.getElementsByTagName("link")}function s(e){window.dispatchEvent(e)}};'

    const got = makeModulePreloadHelperWorkerSafe(code)

    expect(got).toContain(
      'typeof document !== "undefined" && i && i.length > 0',
    )
    expect(got).toContain(
      'typeof window !== "undefined" && window.dispatchEvent(e)',
    )
  })
})

describe('Vite build input tracking', () => {
  it('includes config dependencies and lazy chunks in the cache inputs', async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), 'vite-lazy-inputs-'))
    try {
      const entry = path.join(root, 'entry.ts')
      await fs.writeFile(entry, "export const load = () => import('./lazy')")
      await fs.writeFile(
        path.join(root, 'lazy.ts'),
        "export { value } from './shared'",
      )
      await fs.writeFile(
        path.join(root, 'shared.ts'),
        'export const value = 42',
      )

      const configPath = path.join(root, 'vite.config.mjs')
      await fs.writeFile(
        path.join(root, 'build-policy.mjs'),
        'export const minify = false',
      )
      await fs.writeFile(
        configPath,
        "import { minify } from './build-policy.mjs'; export default { build: { minify } }",
      )
      const config = await buildConfig(
        { mode: 'production', command: 'build' },
        configPath,
      )

      const { result } = await buildAndAnalyze(
        {
          ...config,
          root,
          build: {
            ...config.build,
            outDir: path.join(root, 'dist'),
            lib: { entry, formats: ['es'] },
          },
        },
        root,
        new Map(),
      )

      expect(result.inputFiles.sort()).toEqual([
        'build-policy.mjs',
        'entry.ts',
        'lazy.ts',
        'shared.ts',
        'vite.config.mjs',
      ])
    } finally {
      await fs.rm(root, { recursive: true, force: true })
    }
  })
})
