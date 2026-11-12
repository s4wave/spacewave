import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

import {
  fillReactDevGeneration,
  parseReactDevPath,
  ReactDevPicks,
  readReactDevGeneration,
  readReactDevSetting,
  reactDevCacheName,
  reactDevStorageKey,
  serveReactDev,
  type ReactDevFile,
} from './react-dev.js'

const origin = 'https://app.test'

class FakeCache {
  public readonly entries = new Map<string, Response>()

  public async match(key: string | Request): Promise<Response | undefined> {
    return this.entries.get(toURL(key))?.clone()
  }

  public async put(key: string | Request, response: Response): Promise<void> {
    this.entries.set(toURL(key), response.clone())
  }

  public async delete(key: string | Request): Promise<boolean> {
    return this.entries.delete(toURL(key))
  }

  public async keys(): Promise<Request[]> {
    return [...this.entries.keys()].map((url) => new Request(url))
  }
}

class FakeCacheStorage {
  public readonly caches = new Map<string, FakeCache>()

  public async open(name: string): Promise<FakeCache> {
    let cache = this.caches.get(name)
    if (!cache) {
      cache = new FakeCache()
      this.caches.set(name, cache)
    }
    return cache
  }

  public async delete(name: string): Promise<boolean> {
    return this.caches.delete(name)
  }
}

function toURL(key: string | Request): string {
  return typeof key === 'string' ? key : key.url
}

const files: ReactDevFile[] = [
  { file: 'react/index.mjs', url: '/dev/react/index.mjs' },
  { file: 'react-dom/client.mjs', url: '/dev/react-dom/client.mjs' },
]

function devSource(failAt = ''): (url: string) => Promise<Response> {
  return async (url) =>
    url.endsWith(failAt) && failAt
      ? new Response('gone', { status: 503 })
      : new Response('dev ' + url)
}

function newPicks(storage: FakeCacheStorage): ReactDevPicks {
  return new ReactDevPicks((version) =>
    readReactDevGeneration(storage as unknown as CacheStorage, origin, version),
  )
}

async function load(
  picks: ReactDevPicks,
  clientId: string,
  prodPath: string,
): Promise<string> {
  const path = parseReactDevPath('/b/react-dev/19.2.0' + prodPath)
  if (!path) {
    throw new Error('unparsed ' + prodPath)
  }
  const response = await serveReactDev(picks, clientId, path, async (p) => {
    return new Response('prod ' + p)
  })
  return response.text()
}

const reactPath = '/entrypoint/abc/pkgs/react/index.mjs'
const domPath = '/entrypoint/abc/pkgs/react-dom/client.mjs'

describe('parseReactDevPath', () => {
  it('splits the version, production path and generation file', () => {
    expect(parseReactDevPath('/b/react-dev/19.2.0' + domPath)).toEqual({
      version: '19.2.0',
      prodPath: domPath,
      file: 'react-dom/client.mjs',
    })
    expect(parseReactDevPath('/b/react-dev/19.2.0/react.mjs')).toBeNull()
    expect(parseReactDevPath('/b/pkg/react/index.mjs')).toBeNull()
  })
})

describe('Development React generations', () => {
  it('serves both files from a complete generation', async () => {
    const storage = new FakeCacheStorage()
    await fillReactDevGeneration(
      storage as unknown as CacheStorage,
      origin,
      '19.2.0',
      files,
      devSource(),
    )
    const picks = newPicks(storage)

    expect(await load(picks, 'doc', reactPath)).toBe('dev /dev/react/index.mjs')
    expect(await load(picks, 'doc', domPath)).toBe(
      'dev /dev/react-dom/client.mjs',
    )
  })

  it('serves the production files when no generation is complete', async () => {
    const storage = new FakeCacheStorage()
    await expect(
      fillReactDevGeneration(
        storage as unknown as CacheStorage,
        origin,
        '19.2.0',
        files,
        devSource('client.mjs'),
      ),
    ).rejects.toThrow('HTTP 503')
    const picks = newPicks(storage)

    expect(await load(picks, 'doc', reactPath)).toBe('prod ' + reactPath)
    expect(await load(picks, 'doc', domPath)).toBe('prod ' + domPath)
  })

  it('never mixes builds when the generation goes mid-boot', async () => {
    const storage = new FakeCacheStorage()
    await fillReactDevGeneration(
      storage as unknown as CacheStorage,
      origin,
      '19.2.0',
      files,
      devSource(),
    )
    const picks = newPicks(storage)

    // The document's first file picks the generation; eviction follows.
    expect(await load(picks, 'doc', reactPath)).toMatch(/^dev /)
    await storage.delete(reactDevCacheName)
    expect(await load(picks, 'doc', domPath)).toMatch(/^dev /)

    // The next document finds no generation and runs production throughout.
    expect(await load(picks, 'next', reactPath)).toMatch(/^prod /)
    expect(await load(picks, 'next', domPath)).toMatch(/^prod /)
  })

  it('treats a generation missing a listed file as absent', async () => {
    const storage = new FakeCacheStorage()
    await fillReactDevGeneration(
      storage as unknown as CacheStorage,
      origin,
      '19.2.0',
      files,
      devSource(),
    )
    const cache = await storage.open(reactDevCacheName)
    await cache.delete(origin + '/b/react-dev/19.2.0/react-dom/client.mjs')
    const picks = newPicks(storage)

    expect(await load(picks, 'doc', reactPath)).toMatch(/^prod /)
    expect(await load(picks, 'doc', domPath)).toMatch(/^prod /)
  })

  it('keeps only the latest version and drops closed documents', async () => {
    const storage = new FakeCacheStorage()
    for (const version of ['19.1.0', '19.2.0']) {
      await fillReactDevGeneration(
        storage as unknown as CacheStorage,
        origin,
        version,
        files,
        devSource(),
      )
    }
    const cache = await storage.open(reactDevCacheName)
    expect(
      [...cache.entries.keys()].every((url) => url.includes('/19.2.0/')),
    ).toBe(true)

    const picks = newPicks(storage)
    await load(picks, 'a', reactPath)
    await load(picks, 'b', reactPath)
    picks.retain(new Set(['b']))
    expect(picks.size).toBe(1)
  })
})

// The shell's inline script, run against a stand-in document.
const shell = readFileSync(
  join(__dirname, '../entrypoint/index/index.html'),
  'utf8',
)
const mapScript =
  /<script>\s*(\/\/ Emit the only import map:[\s\S]*?)<\/script>/.exec(
    shell,
  )?.[1]

interface ShellEnv {
  setting?: string | null
  controlled?: boolean
  storageThrows?: boolean
  reactVersion?: string
}

function runShell(env: ShellEnv): Record<string, string> {
  if (!mapScript) {
    throw new Error('import map script not found in index.html')
  }
  let emitted = ''
  const mapJSON = JSON.stringify({
    imports: {
      react: './entrypoint/abc/pkgs/react/index.mjs',
      'react/jsx-runtime': './entrypoint/abc/pkgs/react/jsx-runtime.mjs',
      'react-dom/client': './entrypoint/abc/pkgs/react-dom/client.mjs',
      'react-icons': './entrypoint/abc/pkgs/react-icons/index.mjs',
      '@aptre/bldr': './entrypoint/abc/pkgs/@aptre/bldr/index.mjs',
    },
  })
  const data = {
    content: { firstElementChild: { textContent: mapJSON } },
    getAttribute: () => env.reactVersion ?? '19.2.0',
    after: (script: { type: string; textContent: string }) => {
      expect(script.type).toBe('importmap')
      emitted = script.textContent
    },
  }
  const window: Record<string, unknown> = {}
  const localStorage = {
    getItem: (key: string) => {
      if (env.storageThrows) {
        throw new Error('denied')
      }
      expect(key).toBe(reactDevStorageKey)
      return env.setting ?? null
    },
  }
  const document = {
    baseURI: origin + '/',
    getElementById: (id: string) => (id === 'bldr-import-map' ? data : null),
    createElement: () => ({ type: '', textContent: '' }),
  }
  const navigator = {
    serviceWorker: { controller: env.controlled ? {} : null },
  }
  new Function('window', 'document', 'localStorage', 'navigator', mapScript)(
    window,
    document,
    localStorage,
    navigator,
  )
  return (JSON.parse(emitted) as { imports: Record<string, string> }).imports
}

const on = JSON.stringify({ enabled: true, version: '19.2.0' })

describe('shell import map script', () => {
  it('binds development React only when every condition holds', () => {
    const imports = runShell({ setting: on, controlled: true })
    expect(imports['react']).toBe(
      '/b/react-dev/19.2.0/entrypoint/abc/pkgs/react/index.mjs',
    )
    expect(imports['react/jsx-runtime']).toBe(
      '/b/react-dev/19.2.0/entrypoint/abc/pkgs/react/jsx-runtime.mjs',
    )
    expect(imports['react-dom/client']).toBe(
      '/b/react-dev/19.2.0/entrypoint/abc/pkgs/react-dom/client.mjs',
    )
    expect(imports['react-icons']).toBe(
      './entrypoint/abc/pkgs/react-icons/index.mjs',
    )
    expect(imports['@aptre/bldr']).toBe(
      './entrypoint/abc/pkgs/@aptre/bldr/index.mjs',
    )
  })

  it('binds production React in every other state', () => {
    const prod = './entrypoint/abc/pkgs/react/index.mjs'
    const cases: ShellEnv[] = [
      { setting: null, controlled: true },
      { setting: JSON.stringify({ enabled: false, version: '19.2.0' }) },
      { setting: on, controlled: false },
      {
        setting: JSON.stringify({ enabled: true, version: '19.1.0' }),
        controlled: true,
      },
      { setting: JSON.stringify({ enabled: true, version: '' }) },
      { setting: '{not json', controlled: true },
      { setting: on, controlled: true, storageThrows: true },
      { setting: on, controlled: true, reactVersion: '' },
    ]
    for (const env of cases) {
      expect(runShell(env)['react']).toBe(prod)
    }
  })
})

describe('readReactDevSetting', () => {
  it('reads an unreadable setting as off', () => {
    const storage = { getItem: () => '[1' } as unknown as Storage
    expect(readReactDevSetting(storage)).toEqual({
      enabled: false,
      version: '',
    })
  })
})
