import { describe, it, expect, afterAll, vi } from 'vitest'
import {
  mkdtemp,
  mkdir,
  writeFile,
  readdir,
  readFile,
  rm,
} from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { build } from 'vite'

// The isolated Bldr harness ships the SDK inside its own dist root. The app
// project roots here have no node_modules install of @aptre/bldr-sdk, no
// tsconfig paths, and no vendored Spacewave sources, so only the base-config
// alias can resolve these imports. The regression runs a real Vite/Rolldown
// bundle against a flat dist root containing both a .ts and a .tsx packaged
// SDK module.
type HarnessLayout = 'flat' | 'nested' | 'none'

async function startIsolatedHarness(layout: HarnessLayout): Promise<{
  baseDir: string
  distRoot: string
  appRoot: string
}> {
  const baseDir = await mkdtemp(join(tmpdir(), 'bldr-sdk-alias-'))
  const appRoot = join(baseDir, 'app')
  const distRoot = join(baseDir, 'dist')
  if (layout !== 'none') {
    // Flat harness dist layout is <dist>/sdk; the monorepo dist tree nests it
    // under <dist>/bldr/sdk. Both must resolve.
    const sdkRoot =
      layout === 'flat' ? join(distRoot, 'sdk') : join(distRoot, 'bldr', 'sdk')
    await mkdir(join(sdkRoot, 'hooks'), { recursive: true })
    await writeFile(
      join(sdkRoot, 'plugin.ts'),
      'export const bldrSdkPlugin = true\n',
    )
    // Packaged SDK modules ship as either .ts or .tsx; the alias must not
    // assume one extension.
    await writeFile(
      join(sdkRoot, 'hooks', 'useStreamingResource.ts'),
      "export function useStreamingResource() { return 'stream' }\n",
    )
    await writeFile(
      join(sdkRoot, 'hooks', 'ResourcesContext.tsx'),
      "export function ResourcesContext() { return 'resources' }\n",
    )
  }
  await mkdir(appRoot, { recursive: true })
  // vite-base.config reads these at import time; reset the module registry so
  // each harness picks up its own roots.
  process.env['BLDR_PROJECT_ROOT'] = appRoot
  process.env['BLDR_DIST_ROOT'] = distRoot
  vi.resetModules()
  return { baseDir, distRoot, appRoot }
}

async function loadBaseConfig(appRoot: string) {
  vi.resetModules()
  const { default: baseConfig } = await import('./vite-base.config.js')
  return {
    ...baseConfig,
    root: appRoot,
    configFile: false,
    logLevel: 'error' as const,
  }
}

describe('vite-base.config bldr-sdk aliases in an isolated harness', () => {
  const baseDirs: string[] = []
  afterAll(async () => {
    delete process.env['BLDR_PROJECT_ROOT']
    delete process.env['BLDR_DIST_ROOT']
    for (const baseDir of baseDirs) {
      await rm(baseDir, { recursive: true, force: true })
    }
  })

  it.each(['flat', 'nested'] as const)(
    'bundles .ts and .tsx bldr-sdk hooks from the %s dist layout',
    async (layout) => {
      const harness = await startIsolatedHarness(layout)
      baseDirs.push(harness.baseDir)
      const appEntry = join(harness.appRoot, 'index.ts')
      await writeFile(
        appEntry,
        [
          "import { bldrSdkPlugin } from '@aptre/bldr-sdk'",
          "import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'",
          "import { ResourcesContext } from '@aptre/bldr-sdk/hooks/ResourcesContext.js'",
          'console.log(bldrSdkPlugin, useStreamingResource(), ResourcesContext())',
          '',
        ].join('\n'),
      )

      const outDir = join(harness.appRoot, 'vite-dist')
      await build({
        ...(await loadBaseConfig(harness.appRoot)),
        build: {
          outDir,
          emptyOutDir: true,
          write: true,
          minify: false,
          rolldownOptions: { input: appEntry },
        },
      })

      // Vite writes the bundle under outDir/assets/.
      const chunkPaths: string[] = []
      async function walk(dir: string) {
        for (const entry of await readdir(dir, { withFileTypes: true })) {
          const path = join(dir, entry.name)
          if (entry.isDirectory()) {
            await walk(path)
          } else if (path.endsWith('.js')) {
            chunkPaths.push(path)
          }
        }
      }
      await walk(outDir)
      expect(chunkPaths.length).toBeGreaterThan(0)
      const bundled = await Promise.all(
        chunkPaths.map((chunk) => readFile(chunk, 'utf-8')),
      ).then((parts) => parts.join('\n'))
      expect(bundled).toContain('stream')
      expect(bundled).toContain('resources')
    },
  )

  it('fails the bundle when a .js hook import has no packaged target', async () => {
    const harness = await startIsolatedHarness('flat')
    baseDirs.push(harness.baseDir)
    const appEntry = join(harness.appRoot, 'index.ts')
    await writeFile(
      appEntry,
      [
        "import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'",
        'console.log(useResource())',
        '',
      ].join('\n'),
    )

    await expect(
      build({
        ...(await loadBaseConfig(harness.appRoot)),
        build: {
          outDir: join(harness.appRoot, 'vite-dist'),
          emptyOutDir: true,
          write: false,
          minify: false,
          rolldownOptions: { input: appEntry },
        },
      }),
    ).rejects.toThrow(/useResource|UNLOADABLE_DEPENDENCY|Could not load/i)
  })

  it('leaves bldr-sdk imports unresolved when the dist root has no SDK', async () => {
    const harness = await startIsolatedHarness('none')
    baseDirs.push(harness.baseDir)
    const appEntry = join(harness.appRoot, 'index.ts')
    await writeFile(
      appEntry,
      [
        "import { bldrSdkPlugin } from '@aptre/bldr-sdk'",
        'console.log(bldrSdkPlugin)',
        '',
      ].join('\n'),
    )

    await expect(
      build({
        ...(await loadBaseConfig(harness.appRoot)),
        build: {
          outDir: join(harness.appRoot, 'vite-dist'),
          emptyOutDir: true,
          write: false,
          minify: false,
          rolldownOptions: { input: appEntry },
        },
      }),
    ).rejects.toThrow()
  })
})
