import { describe, expect, test } from 'bun:test'
import {
  existsSync,
  mkdirSync,
  rmSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { deflateRawSync } from 'node:zlib'

import {
  type Fetch,
  type ZipFile,
  escapePath,
  goImports,
  hash1,
  isSafeRelativePath,
  modulesImported,
  parseGoMod,
  parseGoSum,
  parseModulesTxt,
  readZip,
  recordName,
  sourceKey,
  sourceOf,
  staleModules,
  syncVendor,
  verifiedFiles,
} from './vendor-ts.js'

const testdata = join(import.meta.dir, 'testdata', 'vendor-ts')

// makeZip builds a zip archive, deflating each file when deflate is set.
// declared overrides the uncompressed size the central directory states.
function makeZip(
  files: ZipFile[],
  deflate = false,
  declared: (file: ZipFile) => number = (f) => f.data.length,
): Uint8Array {
  const encoder = new TextEncoder()
  const locals: Uint8Array[] = []
  const centrals: Uint8Array[] = []
  let offset = 0
  for (const file of files) {
    const name = encoder.encode(file.name)
    const body = deflate ? new Uint8Array(deflateRawSync(file.data)) : file.data
    const local = new Uint8Array(30 + name.length + body.length)
    const lv = new DataView(local.buffer)
    lv.setUint32(0, 0x04034b50, true)
    lv.setUint16(8, deflate ? 8 : 0, true)
    lv.setUint32(18, body.length, true)
    lv.setUint32(22, file.data.length, true)
    lv.setUint16(26, name.length, true)
    local.set(name, 30)
    local.set(body, 30 + name.length)
    const central = new Uint8Array(46 + name.length)
    const cv = new DataView(central.buffer)
    cv.setUint32(0, 0x02014b50, true)
    cv.setUint16(10, deflate ? 8 : 0, true)
    cv.setUint32(20, body.length, true)
    cv.setUint32(24, declared(file), true)
    cv.setUint16(28, name.length, true)
    cv.setUint32(42, offset, true)
    central.set(name, 46)
    locals.push(local)
    centrals.push(central)
    offset += local.length
  }
  const centralSize = centrals.reduce((n, c) => n + c.length, 0)
  const end = new Uint8Array(22)
  const ev = new DataView(end.buffer)
  ev.setUint32(0, 0x06054b50, true)
  ev.setUint16(8, files.length, true)
  ev.setUint16(10, files.length, true)
  ev.setUint32(12, centralSize, true)
  ev.setUint32(16, offset, true)
  const out = new Uint8Array(offset + centralSize + 22)
  let at = 0
  for (const part of [...locals, ...centrals, end]) {
    out.set(part, at)
    at += part.length
  }
  return out
}

function file(name: string, text: string): ZipFile {
  return { name, data: new TextEncoder().encode(text) }
}

const goMod = `module example.com/main

go 1.25 // toolchain comment

require example.com/single v1.0.0

require (
	example.com/a v1.2.0
	example.com/a/nested v0.1.0 // indirect
	example.com/b v0.3.0
	example.com/local v0.0.0
)

replace example.com/b v0.3.0 => example.com/b-fork v0.3.1

replace (
	example.com/b v0.2.0 => example.com/b-old v0.2.9
	example.com/local => ../local-checkout
)
`

describe('parseGoMod', () => {
  const mod = parseGoMod(goMod, '/work/main')

  test('reads the module, requirements and comments', () => {
    expect(mod.module).toBe('example.com/main')
    expect([...mod.require]).toEqual([
      ['example.com/single', 'v1.0.0'],
      ['example.com/a', 'v1.2.0'],
      ['example.com/a/nested', 'v0.1.0'],
      ['example.com/b', 'v0.3.0'],
      ['example.com/local', 'v0.0.0'],
    ])
  })

  test('follows replace directives', () => {
    expect(sourceOf(mod, 'example.com/b')).toEqual({
      kind: 'proxy',
      path: 'example.com/b-fork',
      version: 'v0.3.1',
    })
    expect(sourceOf(mod, 'example.com/local')).toEqual({
      kind: 'local',
      dir: '/work/local-checkout',
    })
    expect(sourceOf(mod, 'example.com/a')).toEqual({
      kind: 'proxy',
      path: 'example.com/a',
      version: 'v1.2.0',
    })
  })

  test('applies a versioned replace only to that version', () => {
    const other = parseGoMod(
      'module m\nrequire example.com/b v0.4.0\nreplace example.com/b v0.3.0 => example.com/b-fork v0.3.1\n',
      '/',
    )
    expect(sourceKey(sourceOf(other, 'example.com/b'))).toBe(
      'example.com/b@v0.4.0',
    )
  })

  test('refuses a module go.mod does not require', () => {
    expect(() => sourceOf(mod, 'example.com/missing')).toThrow(
      'example.com/missing',
    )
  })
})

describe('imports', () => {
  const mod = parseGoMod(goMod, '/work/main')

  test('maps @go and @goscript specifiers to Go paths', () => {
    expect(
      goImports(
        [
          `import { A } from '@go/example.com/a/pkg/a.pb.js'`,
          `export * from "@go/example.com/a/nested/x.js"`,
          `const m = await import('@goscript/builtin/index.js')`,
          `import './local.js'`,
        ].join('\n'),
      ),
    ).toEqual([
      'example.com/a/pkg/a.pb.js',
      'example.com/a/nested/x.js',
      'github.com/s4wave/goscript/gs/builtin/index.js',
    ])
  })

  test('picks the longest module and skips the main module', () => {
    const source = [
      `import '@go/example.com/a/nested/x.js'`,
      `import '@go/example.com/a/y.js'`,
      `import '@go/example.com/main/sdk/z.js'`,
      `import '@go/example.com/unknown/w.js'`,
    ].join('\n')
    expect([...modulesImported(mod, [source])].sort()).toEqual([
      'example.com/a',
      'example.com/a/nested',
    ])
  })
})

describe('go.sum and hashing', () => {
  const sums = parseGoSum(
    readFileSync(join(import.meta.dir, '..', 'go.sum'), 'utf8'),
  )

  test('parseGoSum keeps zip hashes and skips go.mod hashes', () => {
    const parsed = parseGoSum(
      'example.com/a v1.0.0 h1:zip=\nexample.com/a v1.0.0/go.mod h1:mod=\n',
    )
    expect([...parsed]).toEqual([['example.com/a v1.0.0', 'h1:zip=']])
  })

  // A go.sum /go.mod hash is Hash1 over one file named go.mod.
  test.each([
    ['util.mod', 'h1:NCySShOBZEGJl31Q2z3xSeDOWxRZ9wTcDTGAHvNi9bo='],
    [
      'prometheus-client-lite.mod',
      'h1:qRtWyi1AK6J7eZ1E5dfZEBZtBOIHjL0Nq9BejU+9UIM=',
    ],
  ])('hash1 of %s matches go.sum', (name, want) => {
    const data = new Uint8Array(readFileSync(join(testdata, name)))
    expect(hash1([{ name: 'go.mod', data }])).toBe(want)
  })

  test('the fixtures match the go.sum of this checkout', () => {
    const text = readFileSync(join(import.meta.dir, '..', 'go.sum'), 'utf8')
    expect(text).toContain('h1:NCySShOBZEGJl31Q2z3xSeDOWxRZ9wTcDTGAHvNi9bo=')
    expect(text).toContain('h1:qRtWyi1AK6J7eZ1E5dfZEBZtBOIHjL0Nq9BejU+9UIM=')
    expect(sums.size).toBeGreaterThan(0)
  })

  test('escapePath escapes upper case', () => {
    expect(escapePath('github.com/BurntSushi/toml')).toBe(
      'github.com/!burnt!sushi/toml',
    )
  })
})

describe('verifiedFiles', () => {
  const files = [
    file('example.com/a@v1.2.0/go.mod', 'module example.com/a\n'),
    file('example.com/a@v1.2.0/pkg/a.go', 'package pkg\n'),
    file('example.com/a@v1.2.0/pkg/a.pb.ts', 'export const a = 1\n'),
    file('example.com/a@v1.2.0/pkg/a.d.ts', 'export declare const b: 1\n'),
  ]

  test('reads stored and deflated zips alike', () => {
    for (const deflate of [false, true]) {
      const read = readZip(makeZip(files, deflate))
      expect(read.map((f) => f.name)).toEqual(files.map((f) => f.name))
      expect(hash1(read)).toBe(hash1(files))
    }
  })

  test('returns only the .ts files of a matching zip', () => {
    const sums = new Map([['example.com/a v1.2.0', hash1(files)]])
    const got = verifiedFiles(
      makeZip(files, true),
      'example.com/a',
      'v1.2.0',
      sums,
    )
    expect([...got.keys()].sort()).toEqual(['pkg/a.d.ts', 'pkg/a.pb.ts'])
  })

  test('refuses a zip whose hash differs from go.sum', () => {
    const sums = new Map([['example.com/a v1.2.0', hash1(files)]])
    const tampered = [...files]
    tampered[2] = file(
      'example.com/a@v1.2.0/pkg/a.pb.ts',
      'export const a = 2\n',
    )
    expect(() =>
      verifiedFiles(makeZip(tampered), 'example.com/a', 'v1.2.0', sums),
    ).toThrow('example.com/a v1.2.0: zip hash')
  })

  test('refuses a module go.sum lacks', () => {
    expect(() =>
      verifiedFiles(makeZip(files), 'example.com/a', 'v1.2.0', new Map()),
    ).toThrow('go.sum has no hash for example.com/a v1.2.0')
  })
})

describe('zip bounds and entry paths', () => {
  const prefix = 'example.com/a@v1.2.0/'
  const files = [
    file(prefix + 'go.mod', 'module example.com/a\n'),
    file(prefix + 'x.ts', 'export const x = "' + 'x'.repeat(4096) + '"\n'),
  ]

  test.each([
    '../../escaped.ts',
    '/abs.ts',
    'a/../../escaped.ts',
    './x.ts',
    'a//x.ts',
    'a\\x.ts',
  ])('refuses the whole module for entry %s', (rest) => {
    const evil = [...files, file(prefix + rest, 'export {}\n')]
    const sums = new Map([['example.com/a v1.2.0', hash1(evil)]])
    expect(() =>
      verifiedFiles(makeZip(evil, true), 'example.com/a', 'v1.2.0', sums),
    ).toThrow('is unsafe')
  })

  test('isSafeRelativePath accepts plain paths only', () => {
    expect(isSafeRelativePath('pkg/a.pb.ts')).toBe(true)
    expect(isSafeRelativePath('')).toBe(false)
    expect(isSafeRelativePath('pkg/..')).toBe(false)
  })

  test('refuses an entry that inflates past its declared size', () => {
    const zip = makeZip(files, true, (f) =>
      f.name.endsWith('x.ts') ? 16 : f.data.length,
    )
    expect(() => readZip(zip)).toThrow('does not inflate to its declared')
  })

  test('refuses a module whose declared total exceeds the bound', () => {
    expect(() => readZip(makeZip(files, true), 1024)).toThrow(
      'uncompressed size exceeds 1024 bytes',
    )
  })

  test('refuses zip64 sizes and duplicate names', () => {
    expect(() => readZip(makeZip(files, false, () => 0xffffffff))).toThrow(
      'zip64 archives are not supported',
    )
    expect(() => readZip(makeZip([files[0], files[0]]))).toThrow(
      'duplicate entry ' + prefix + 'go.mod',
    )
  })
})

describe('staleness', () => {
  const mod = parseGoMod(goMod, '/work/main')

  test('compares modules.txt with go.mod', () => {
    const vendored = parseModulesTxt(
      [
        '# example.com/a v1.2.0',
        '## explicit; go 1.21',
        'example.com/a/pkg',
        '# example.com/b v0.3.0 => example.com/b-fork v0.3.0',
        '# example.com/local v0.0.0 => ../local-checkout',
      ].join('\n'),
      '/work/main',
    )
    expect(
      staleModules(
        mod,
        [
          'example.com/a',
          'example.com/b',
          'example.com/local',
          'example.com/single',
        ],
        vendored,
      ),
    ).toEqual(['example.com/b', 'example.com/single'])
  })
})

describe('syncVendor', () => {
  const mainMod = (version: string) =>
    parseGoMod(
      `module example.com/main\nrequire (\n\texample.com/a ${version}\n\texample.com/dep v0.1.0\n)\n`,
      '/',
    )
  const zips = new Map<string, ZipFile[]>()
  const addModule = (path: string, version: string, ts: string) => {
    zips.set(`https://proxy.golang.org/${path}/@v/${version}.zip`, [
      file(`${path}@${version}/go.mod`, `module ${path}\n`),
      file(`${path}@${version}/x.ts`, ts),
    ])
  }
  addModule('example.com/a', 'v1.0.0', `import '@go/example.com/dep/d.js'\n`)
  addModule('example.com/a', 'v1.1.0', `import '@go/example.com/dep/d.js'\n`)
  addModule('example.com/dep', 'v0.1.0', 'export const d = 1\n')
  const sums = new Map<string, string>()
  for (const [url, files] of zips) {
    const [, path, version] = /\.org\/(.+)\/@v\/(.+)\.zip$/.exec(url) ?? []
    sums.set(`${path} ${version}`, hash1(files))
  }
  const fetched: string[] = []
  const get: Fetch = async (url) => {
    fetched.push(url)
    const files = zips.get(url)
    if (!files) throw new Error('offline')
    return makeZip(files, true)
  }
  const sources = [`import { a } from '@go/example.com/a/x.js'\n`]

  test('fetches the closure, then only what go.mod changed', async () => {
    const root = mkdtempSync(join(tmpdir(), 'vendor-ts-'))
    const first = await syncVendor({
      root,
      mod: mainMod('v1.0.0'),
      sums,
      sources,
      get,
    })
    expect(first.fetched).toEqual(['example.com/a', 'example.com/dep'])
    expect(existsSync(join(root, 'vendor/example.com/dep/x.ts'))).toBe(true)
    expect(existsSync(join(root, 'vendor/example.com/a/go.mod'))).toBe(false)

    const again = await syncVendor({
      root,
      mod: mainMod('v1.0.0'),
      sums,
      sources,
      get,
    })
    expect(again.fetched).toEqual([])

    const bumped = await syncVendor({
      root,
      mod: mainMod('v1.1.0'),
      sums,
      sources,
      get,
    })
    expect(bumped.fetched).toEqual(['example.com/a'])
    const record = JSON.parse(
      readFileSync(join(root, 'vendor', recordName), 'utf8'),
    )
    expect(record['example.com/a'].source).toBe('example.com/a@v1.1.0')
    expect(readdirSync(join(root, 'vendor')).sort()).toEqual([
      recordName,
      'example.com',
    ])
  })

  test('fails offline with a stale vendor and keeps what it had', async () => {
    const root = mkdtempSync(join(tmpdir(), 'vendor-ts-'))
    await syncVendor({ root, mod: mainMod('v1.0.0'), sums, sources, get })
    await expect(
      syncVendor({ root, mod: mainMod('v1.9.0'), sums, sources, get }),
    ).rejects.toThrow('run bun run typecheck once online')
    const record = JSON.parse(
      readFileSync(join(root, 'vendor', recordName), 'utf8'),
    )
    expect(record['example.com/a'].source).toBe('example.com/a@v1.0.0')
    expect(existsSync(join(root, 'vendor/example.com/a/x.ts'))).toBe(true)
  })

  test('refuses a module with an escaping entry and writes nothing', async () => {
    const root = mkdtempSync(join(tmpdir(), 'vendor-ts-'))
    const evil = [
      file('example.com/a@v1.0.0/go.mod', 'module example.com/a\n'),
      file('example.com/a@v1.0.0/../../escaped.ts', 'export {}\n'),
    ]
    const evilSums = new Map([['example.com/a v1.0.0', hash1(evil)]])
    await expect(
      syncVendor({
        root,
        mod: mainMod('v1.0.0'),
        sums: evilSums,
        sources,
        get: () => Promise.resolve(makeZip(evil, true)),
      }),
    ).rejects.toThrow('example.com/a v1.0.0: zip entry')
    expect(existsSync(join(root, 'escaped.ts'))).toBe(false)
    expect(existsSync(join(root, 'vendor', 'escaped.ts'))).toBe(false)
    expect(existsSync(join(root, 'vendor', 'example.com'))).toBe(false)
  })

  test('fetches a recorded module whose directory is gone', async () => {
    const root = mkdtempSync(join(tmpdir(), 'vendor-ts-'))
    await syncVendor({ root, mod: mainMod('v1.0.0'), sums, sources, get })
    rmSync(join(root, 'vendor/example.com/a'), { recursive: true })
    const again = await syncVendor({
      root,
      mod: mainMod('v1.0.0'),
      sums,
      sources,
      get,
    })
    expect(again.fetched).toEqual(['example.com/a'])
    expect(existsSync(join(root, 'vendor/example.com/a/x.ts'))).toBe(true)
  })

  test('warns on a stale go mod vendor tree and leaves it alone', async () => {
    const root = mkdtempSync(join(tmpdir(), 'vendor-ts-'))
    const vendorDir = join(root, 'vendor')
    mkdirSync(join(vendorDir, 'example.com/a'), { recursive: true })
    writeFileSync(join(vendorDir, 'modules.txt'), '# example.com/a v1.0.0\n')
    writeFileSync(join(vendorDir, 'example.com/a/x.ts'), 'export {}\n')
    const snapshot = () =>
      new Map(
        readdirSync(vendorDir, { recursive: true, encoding: 'utf8' }).map(
          (name) => {
            const file = join(vendorDir, name)
            return [
              name,
              statSync(file).isDirectory() ? null : readFileSync(file),
            ] as const
          },
        ),
      )
    const before = snapshot()
    fetched.length = 0
    const result = await syncVendor({
      root,
      mod: mainMod('v1.1.0'),
      sums,
      sources,
      get,
    })
    expect(result.fetched).toEqual([])
    expect(result.warning).toBe(
      `${vendorDir} came from go mod vendor and is stale for example.com/a; run bun run prepare where Go runs, or remove vendor/ to fetch TypeScript only`,
    )
    expect(fetched).toEqual([])
    expect(snapshot()).toEqual(before)

    const current = await syncVendor({
      root,
      mod: mainMod('v1.0.0'),
      sums,
      sources: [`export {}\n`],
      get,
    })
    expect(current.warning).toBeUndefined()
    expect(fetched).toEqual([])
  })
})
