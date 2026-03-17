import { describe, it, expect, beforeAll, afterAll } from 'vitest'
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { goTsResolver } from './go-ts-resolver.js'

describe('goTsResolver', () => {
  let tmpDir: string
  const vendorTsFile = 'github.com/example/pkg/types.ts'
  const vendorJsFile = 'github.com/example/pkg/hasjs.js'
  const vendorJsTsFile = 'github.com/example/pkg/hasjs.ts'

  beforeAll(async () => {
    tmpDir = await mkdtemp(join(tmpdir(), 'go-ts-resolver-'))
    const pkgDir = join(tmpDir, 'vendor', 'github.com/example/pkg')
    await mkdir(pkgDir, { recursive: true })
    await writeFile(join(pkgDir, 'types.ts'), 'export const x = 1')
    await writeFile(join(pkgDir, 'hasjs.js'), 'export const y = 2')
    await writeFile(join(pkgDir, 'hasjs.ts'), 'export const y: number = 2')
  })

  afterAll(async () => {
    if (tmpDir) {
      await rm(tmpDir, { recursive: true, force: true })
    }
  })

  function createPlugin() {
    const plugin = goTsResolver(tmpDir)
    // resolveId is the only hook we need; bind it to a no-op context
    const resolveId = plugin.resolveId as (source: string) => Promise<string | null>
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
})
