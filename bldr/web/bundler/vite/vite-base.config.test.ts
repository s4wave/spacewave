import { describe, it, expect, afterAll, vi } from 'vitest'
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises'
import { realpathSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { createServer, type ViteDevServer } from 'vite'

// The isolated Bldr harness ships the SDK inside its own dist root. The app
// project roots here have no node_modules install of @aptre/bldr-sdk, no
// tsconfig paths, and no vendored Spacewave sources, so only the base-config
// alias can resolve these imports.
type HarnessLayout = 'flat' | 'nested' | 'none'

interface ResolveResult {
  id?: string
  meta?: Record<string, Record<string, unknown>>
}

async function startIsolatedHarness(layout: HarnessLayout): Promise<{
  baseDir: string
  distRoot: string
  server: ViteDevServer
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
    await writeFile(
      join(sdkRoot, 'hooks', 'useStreamingResource.ts'),
      'export function useStreamingResource() {}\n',
    )
  }
  await mkdir(appRoot, { recursive: true })
  // vite-base.config reads these at import time; reset the module registry so
  // each harness picks up its own roots.
  process.env['BLDR_PROJECT_ROOT'] = appRoot
  process.env['BLDR_DIST_ROOT'] = distRoot
  vi.resetModules()
  const { default: baseConfig } = await import('./vite-base.config.js')
  const server = await createServer({
    ...baseConfig,
    root: appRoot,
    configFile: false,
    logLevel: 'error',
    server: { middlewareMode: true },
  })
  return { baseDir, distRoot, server }
}

describe('vite-base.config bldr-sdk aliases in an isolated harness', () => {
  const harnesses: Awaited<ReturnType<typeof startIsolatedHarness>>[] = []
  afterAll(async () => {
    delete process.env['BLDR_PROJECT_ROOT']
    delete process.env['BLDR_DIST_ROOT']
    for (const harness of harnesses) {
      await harness.server.close()
      await rm(harness.baseDir, { recursive: true, force: true })
    }
  })

  // resolveId returns { id, meta }; the Vite alias plugin reports an unmatched
  // replacement through the vite:alias noResolved marker instead of null.
  type Harness = Awaited<ReturnType<typeof startIsolatedHarness>>
  async function resolveImport(
    harness: Harness,
    source: string,
  ): Promise<string | null> {
    const result = (await harness.server.pluginContainer.resolveId(
      source,
    )) as ResolveResult | string | null
    if (typeof result === 'string') {
      return result
    }
    if (result?.meta?.['vite:alias']?.['noResolved']) {
      return null
    }
    return result?.id ?? null
  }

  function existingPath(path: string): string {
    // macOS reports temp dirs through /var symlinks; compare real paths.
    return realpathSync(path)
  }

  it.each([
    ['flat', (distRoot: string) => join(distRoot, 'sdk')],
    ['nested', (distRoot: string) => join(distRoot, 'bldr', 'sdk')],
  ] as const)(
    'resolves bldr-sdk imports against the %s dist layout',
    async (layout, sdkRootOf) => {
      const harness = await startIsolatedHarness(layout)
      harnesses.push(harness)
      const sdkRoot = sdkRootOf(harness.distRoot)
      expect(
        existingPath(
          (await resolveImport(harness, '@aptre/bldr-sdk')) as string,
        ),
      ).toBe(existingPath(join(sdkRoot, 'plugin.ts')))
      expect(
        existingPath(
          (await resolveImport(
            harness,
            '@aptre/bldr-sdk/hooks/useStreamingResource.js',
          )) as string,
        ),
      ).toBe(existingPath(join(sdkRoot, 'hooks', 'useStreamingResource.ts')))
    },
  )

  it('does not resolve a .js hook import with no packaged .ts file', async () => {
    const harness = await startIsolatedHarness('flat')
    harnesses.push(harness)
    expect(
      await resolveImport(harness, '@aptre/bldr-sdk/hooks/doesNotExist.js'),
    ).toBeNull()
  })

  it('leaves bldr-sdk imports unresolved when the dist root has no SDK', async () => {
    const harness = await startIsolatedHarness('none')
    harnesses.push(harness)
    expect(await resolveImport(harness, '@aptre/bldr-sdk')).toBeNull()
    expect(
      await resolveImport(
        harness,
        '@aptre/bldr-sdk/hooks/useStreamingResource.js',
      ),
    ).toBeNull()
  })
})
