import { describe, it, expect, beforeAll, afterAll } from 'vitest'
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { buildGoAliases, goTsResolver } from './go-ts-resolver.js'

describe('goTsResolver', () => {
  let tmpDir: string
  const localPbTsFile = 'db/volume/volume.pb.ts'

  beforeAll(async () => {
    tmpDir = await mkdtemp(join(tmpdir(), 'go-ts-resolver-'))
    const pkgDir = join(tmpDir, 'vendor', 'github.com/example/pkg')
    const localDir = join(tmpDir, 'db/volume')
    await mkdir(pkgDir, { recursive: true })
    await mkdir(localDir, { recursive: true })
    await writeFile(join(pkgDir, 'types.ts'), 'export const x = 1')
    await writeFile(join(pkgDir, 'hasjs.js'), 'export const y = 2')
    await writeFile(join(pkgDir, 'hasjs.ts'), 'export const y: number = 2')
    await writeFile(join(localDir, 'volume.pb.ts'), 'export const v = 1')
    // Resolver reads the local module path from go.mod so it can map
    // @go/<module>/... imports to project sources instead of vendor.
    await writeFile(
      join(tmpDir, 'go.mod'),
      'module github.com/s4wave/spacewave\n\ngo 1.26\n',
    )
  })

  afterAll(async () => {
    if (tmpDir) {
      await rm(tmpDir, { recursive: true, force: true })
    }
  })

  function createPlugin() {
    const plugin = goTsResolver(tmpDir)
    // resolveId is the only hook we need; bind it to a no-op context
    const resolveId = plugin.resolveId as (
      source: string,
      importer?: string,
    ) => Promise<string | null>
    return resolveId
  }

  it('resolves @go/ .js import to .ts when only .ts exists', async () => {
    const resolveId = createPlugin()
    const result = await resolveId('@go/github.com/example/pkg/types.js')
    expect(result).toBe(
      join(tmpDir, 'vendor', 'github.com/example/pkg/types.ts'),
    )
  })

  it('returns null for non-@go/ imports', async () => {
    const resolveId = createPlugin()
    const result = await resolveId('react')
    expect(result).toBeNull()
  })

  it('returns null for @go/ imports not ending in .js', async () => {
    const resolveId = createPlugin()
    const result = await resolveId('@go/github.com/example/pkg/types.ts')
    expect(result).toBeNull()
  })

  it('returns null when neither .ts nor .js exists in vendor', async () => {
    const resolveId = createPlugin()
    const result = await resolveId('@go/github.com/example/pkg/missing.js')
    expect(result).toBeNull()
  })

  it('resolves to .ts even when .js also exists', async () => {
    const resolveId = createPlugin()
    const result = await resolveId('@go/github.com/example/pkg/hasjs.js')
    // Plugin checks for .ts file existence unconditionally, returns .ts path if it exists
    expect(result).toBe(
      join(tmpDir, 'vendor', 'github.com/example/pkg/hasjs.ts'),
    )
  })

  it('resolves monorepo-local @go imports from the repo root', async () => {
    const resolveId = createPlugin()
    const result = await resolveId(
      '@go/github.com/s4wave/spacewave/db/volume/volume.pb.js',
    )
    expect(result).toBe(join(tmpDir, localPbTsFile))
  })

  it('resolves aliased relative .js imports to sibling .ts files', async () => {
    const resolveId = createPlugin()
    const result = await resolveId(
      '../../vendor/github.com/example/pkg/types.js',
      join(tmpDir, 'bldr/example/example-class.tsx'),
    )
    expect(result).toBe(
      join(tmpDir, 'vendor', 'github.com/example/pkg/types.ts'),
    )
  })

  it('resolves project-root-relative vendor .js imports to .ts files', async () => {
    const resolveId = createPlugin()
    const result = await resolveId('vendor/github.com/example/pkg/types.js')
    expect(result).toBe(
      join(tmpDir, 'vendor', 'github.com/example/pkg/types.ts'),
    )
  })

  it('prefers app root vendor and falls back to the Bldr dist vendor tree', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'go-ts-source-'))
    const distRoot = await mkdtemp(join(tmpdir(), 'go-ts-dist-'))
    try {
      await writeFile(
        join(sourceRoot, 'go.mod'),
        'module github.com/example/app\n\ngo 1.26\n',
      )
      const appVendorDir = join(
        sourceRoot,
        'vendor',
        'github.com/aperturerobotics/glados/sdk/projection',
      )
      await mkdir(appVendorDir, { recursive: true })
      await writeFile(
        join(appVendorDir, 'projection.pb.ts'),
        'export const projection = 1',
      )
      const vendorDir = join(
        distRoot,
        'vendor',
        'github.com/aperturerobotics/util/pipesock',
      )
      await mkdir(vendorDir, { recursive: true })
      await writeFile(join(vendorDir, 'pipesock.ts'), 'export const pipe = 1')

      const plugin = goTsResolver(sourceRoot, distRoot)
      const resolveId = plugin.resolveId as (
        source: string,
        importer?: string,
      ) => Promise<string | null>
      const appResult = await resolveId(
        '@go/github.com/aperturerobotics/glados/sdk/projection/projection.pb.js',
      )
      expect(appResult).toBe(join(appVendorDir, 'projection.pb.ts'))

      const fallbackResult = await resolveId(
        '@go/github.com/aperturerobotics/util/pipesock/pipesock.js',
      )
      expect(fallbackResult).toBe(join(vendorDir, 'pipesock.ts'))

      const vendorRelativeResult = await resolveId(
        'vendor/github.com/aperturerobotics/glados/sdk/projection/projection.pb.js',
      )
      expect(vendorRelativeResult).toBe(join(appVendorDir, 'projection.pb.ts'))

      const aliases = buildGoAliases(sourceRoot, distRoot)
      expect(aliases).toHaveLength(1)
      expect(String(aliases[0].find)).toBe(
        String(/^@go\/github\.com\/example\/app\/(.*)$/),
      )
      expect(aliases[0].replacement).toBe(join(sourceRoot, '$1'))
    } finally {
      await rm(sourceRoot, { recursive: true, force: true })
      await rm(distRoot, { recursive: true, force: true })
    }
  })

  it('resolves materialized scoped package imports from a .s4wave/sdk root', async () => {
    const projectRoot = await mkdtemp(join(tmpdir(), 'go-ts-materialized-'))
    try {
      const sdkDir = join(projectRoot, '.s4wave', 'sdk')
      await mkdir(sdkDir, { recursive: true })
      await writeFile(join(sdkDir, 'foo.ts'), 'export const foo = 1')

      const plugin = goTsResolver(projectRoot)
      const resolveId = plugin.resolveId as (
        source: string,
        importer?: string,
      ) => Promise<string | null>

      const result = await resolveId('@s4wave/sdk/foo.js')

      expect(result).toBe(join(sdkDir, 'foo.ts'))
    } finally {
      await rm(projectRoot, { recursive: true, force: true })
    }
  })

  it('builds aliases for materialized scoped package roots', async () => {
    const projectRoot = await mkdtemp(join(tmpdir(), 'go-ts-aliases-'))
    try {
      await mkdir(join(projectRoot, '.s4wave', 'web'), { recursive: true })
      await mkdir(join(projectRoot, '.aptre', 'bldr-sdk'), { recursive: true })

      const aliases = buildGoAliases(projectRoot)
      const replacementsByFind = new Map(
        aliases.map((alias) => [String(alias.find), alias.replacement]),
      )

      expect(aliases).toHaveLength(2)
      expect(replacementsByFind.get('@s4wave/web')).toBe(
        join(projectRoot, '.s4wave', 'web'),
      )
      expect(replacementsByFind.get('@aptre/bldr-sdk')).toBe(
        join(projectRoot, '.aptre', 'bldr-sdk'),
      )
    } finally {
      await rm(projectRoot, { recursive: true, force: true })
    }
  })
})
