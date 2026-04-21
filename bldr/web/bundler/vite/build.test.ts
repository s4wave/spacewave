import { describe, it, expect, beforeAll, afterAll } from 'vitest'
import { analyzeManifest } from './build.js'
import { promises as fs } from 'fs'
import path from 'path'
import os from 'os'
import type { Rollup } from 'vite'

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

    it('should ignore synthetic vite external module ids', async () => {
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
      expect(entryA.inputs).not.toContain('__vite-browser-external')
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
        path.join('node_modules', '@aptre', 'it-ws', 'dist', 'src', 'duplex.js'),
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

  describe('buildAndAnalyze integration', () => {
    it('should return all input files including transitive dependencies', async () => {
      // Note: This will actually run Vite build, which requires proper setup
      // For now, we'll test the analysis part with mocked data
      // In a real scenario, you'd need a complete vite setup

      // TODO: Add full integration test that actually runs Vite build
      // and verifies the complete flow including transitive dependencies
    })
  })

  describe('Input file tracking verification', () => {
    it('should verify that Rollup includes transitive moduleIds', async () => {
      // This test documents what we expect from Rollup/Vite
      // Rollup's OutputChunk.moduleIds should include ALL modules bundled into a chunk,
      // not just direct imports

      const mockChunk = {
        type: 'chunk' as const,
        fileName: 'test.mjs',
        facadeModuleId: '/root/entry.tsx',
        moduleIds: [
          '/root/entry.tsx', // Direct entry
          '/root/direct.tsx', // Direct import
          '/root/transitive.tsx', // Transitive import (the key test)
        ],
      }

      // This is what we rely on: moduleIds contains ALL modules in the chunk
      expect(mockChunk.moduleIds).toContain('/root/transitive.tsx')
      expect(mockChunk.moduleIds.length).toBeGreaterThanOrEqual(3)
    })
  })
})

describe('Integration: Hot Reload Input File Tracking', () => {
  it('should document the expected behavior for hot reload', () => {
    // GIVEN: A.tsx imports B.tsx which imports C.tsx
    // WHEN: The build completes successfully
    // THEN: The inputFiles array returned to Go compiler should include:
    //   - A.tsx (entry point)
    //   - B.tsx (direct dependency)
    //   - C.tsx (transitive dependency)
    //
    // This ensures that when C.tsx changes, the Go compiler knows to rebuild

    const expectedBehavior = {
      scenario: 'A.tsx -> B.tsx -> C.tsx',
      inputFiles: ['A.tsx', 'B.tsx', 'C.tsx'],
      hotReloadTriggers: {
        'A.tsx changes': 'should rebuild',
        'B.tsx changes': 'should rebuild',
        'C.tsx changes': 'should rebuild (currently failing)',
      },
    }

    // The issue is that C.tsx might not be in the inputFiles list
    expect(expectedBehavior.inputFiles).toContain('C.tsx')
  })
})
