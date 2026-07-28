import { afterEach, describe, expect, it } from 'vitest'

import {
  BROWSER_SHELL_TABS_STORAGE_KEY,
  BrowserShellTabsStore,
  resetBrowserShellTabsStoreForTests,
} from './BrowserShellTabsStore.js'

class SerialLocks {
  private tail = Promise.resolve()

  request<T>(
    _name: string,
    _options: LockOptions,
    callback: (lock: Lock) => T | PromiseLike<T>,
  ): Promise<Awaited<T>> {
    const run = this.tail.then(() => callback({} as Lock))
    this.tail = run.then(
      () => undefined,
      () => undefined,
    )
    return run as Promise<Awaited<T>>
  }
}

class ContentionLocks extends SerialLocks {
  firstEntered = Promise.withResolvers<void>()
  private releaseFirst = Promise.withResolvers<void>()
  private first = true
  override request<T>(
    name: string,
    options: LockOptions,
    callback: (lock: Lock) => T | PromiseLike<T>,
  ): Promise<Awaited<T>> {
    return super.request(name, options, async (lock) => {
      if (this.first) {
        this.first = false
        this.firstEntered.resolve()
        await this.releaseFirst.promise
      }
      return callback(lock)
    })
  }

  release(): void {
    this.releaseFirst.resolve()
  }
}

function makeStore() {
  return new BrowserShellTabsStore({
    locks: new SerialLocks() as unknown as Pick<LockManager, 'request'>,
  })
}

function expectStoreError(promise: Promise<unknown>, code: string) {
  return expect(promise).rejects.toMatchObject({ code })
}

describe('BrowserShellTabsStore', () => {
  afterEach(() => {
    localStorage.clear()
    resetBrowserShellTabsStoreForTests()
  })

  it('serializes concurrent additions without losing records', async () => {
    const store = makeStore()
    await Promise.all(
      ['a', 'b', 'c'].map((id) => store.create({ id, path: `/${id}` })),
    )
    expect(store.getSnapshot().records.map((record) => record.id)).toEqual([
      'a',
      'b',
      'c',
    ])
    expect(store.getSnapshot().revision).toBe(3)
  })

  it('waits for a held Web Lock before committing the next mutation', async () => {
    const locks = new ContentionLocks()
    const store = new BrowserShellTabsStore({
      locks: locks as unknown as Pick<LockManager, 'request'>,
    })
    const first = store.create({ id: 'a', path: '/a' })
    await locks.firstEntered.promise
    const second = store.create({ id: 'b', path: '/b' })

    await Promise.resolve()
    expect(store.getSnapshot().records).toEqual([])

    locks.release()
    await Promise.all([first, second])
    expect(store.getSnapshot().records.map((record) => record.id)).toEqual([
      'a',
      'b',
    ])
  })

  it('rejects stale revision, epoch, and schema writers', async () => {
    const store = makeStore()
    await store.create({ id: 'a', path: '/' })
    await expectStoreError(
      store.create({ id: 'b', path: '/b' }, { revision: 0 }),
      'stale-version',
    )
    await expectStoreError(
      store.create({ id: 'c', path: '/c' }, { epoch: 1 }),
      'stale-version',
    )
    await expectStoreError(
      store.create({ id: 'd', path: '/d' }, { schemaVersion: 99 }),
      'stale-version',
    )
  })

  it('initializes a clean first schema and reset advances epoch', async () => {
    const store = makeStore()
    expect(store.getSnapshot().records).toHaveLength(0)
    const first = await store.create({ id: 'fresh', path: '/' })
    expect(first.name).toBe('Home')
    const reset = await store.reset({ id: 'reset', path: '/docs' })
    expect(reset.name).toBe('Docs')
    expect(store.getSnapshot()).toMatchObject({
      schemaVersion: 1,
      epoch: 1,
      revision: 1,
    })
    expect(store.getSnapshot().records).toEqual([
      expect.objectContaining({ id: 'reset', creationSequence: 1 }),
    ])
  })

  it('replaces the final close with one fresh Home record atomically', async () => {
    const store = makeStore()
    await store.create({ id: 'only-tab', path: '/docs' })
    const before = store.read()

    await store.close('only-tab')

    const after = store.read()
    expect(after.records).toHaveLength(1)
    expect(after.records[0]).toMatchObject({
      path: '/',
      name: 'Home',
      creationSequence: 1,
    })
    expect(after.records[0]?.id).not.toBe('only-tab')
    expect(after.epoch).toBe(before.epoch)
    expect(after.revision).toBe(before.revision + 1)
  })

  it('keeps concurrent closes from publishing an empty snapshot', async () => {
    const store = makeStore()
    await store.create({ id: 'tab-a', path: '/a' })
    await store.create({ id: 'tab-b', path: '/b' })

    await Promise.all([store.close('tab-a'), store.close('tab-b')])

    const snapshot = store.read()
    expect(snapshot.records).toHaveLength(1)
    expect(snapshot.records[0]).toMatchObject({
      path: '/',
      name: 'Home',
      creationSequence: 1,
    })
    expect(snapshot.records[0]?.id).not.toBe('tab-a')
    expect(snapshot.records[0]?.id).not.toBe('tab-b')
    expect(snapshot.epoch).toBe(0)
  })

  it('retains inventory across store reconstruction until explicit close or reset', async () => {
    const locks = new SerialLocks() as unknown as Pick<LockManager, 'request'>
    const first = new BrowserShellTabsStore({ locks })
    await first.create({ id: 'tab-1', path: '/' })
    await first.create({ id: 'tab-2', path: '/docs' })

    const restarted = new BrowserShellTabsStore({ locks })
    expect(restarted.read().records.map((record) => record.id)).toEqual([
      'tab-1',
      'tab-2',
    ])

    await restarted.close('tab-1')
    const afterClose = new BrowserShellTabsStore({ locks })
    expect(afterClose.read().records.map((record) => record.id)).toEqual([
      'tab-2',
    ])

    await afterClose.reset({ id: 'reset-tab', path: '/' })
    const afterReset = new BrowserShellTabsStore({ locks })
    expect(afterReset.read().records).toEqual([
      expect.objectContaining({
        id: 'reset-tab',
        path: '/',
        creationSequence: 1,
      }),
    ])
  })

  it('rejects absent Web Locks without a local fallback', async () => {
    const store = new BrowserShellTabsStore({ locks: undefined })
    await expectStoreError(
      store.create({ id: 'a', path: '/' }),
      'web-lock-unavailable',
    )
    expect(store.getSnapshot().records).toHaveLength(0)
  })

  it('rejects parse and quota/write failures without publishing', async () => {
    localStorage.setItem(BROWSER_SHELL_TABS_STORAGE_KEY, '{bad json')
    const parsedStore = makeStore()
    await expectStoreError(
      parsedStore.create({ id: 'a', path: '/' }),
      'invalid-snapshot',
    )

    const storage: Storage = {
      getItem: () => null,
      setItem: () => {
        throw Object.assign(new Error('full'), { name: 'QuotaExceededError' })
      },
      removeItem: () => {},
      clear: () => {},
      key: () => null,
      length: 0,
    }
    const writeStore = new BrowserShellTabsStore({
      storage,
      locks: new SerialLocks() as unknown as Pick<LockManager, 'request'>,
    })
    await expectStoreError(writeStore.create({ id: 'a', path: '/' }), 'quota')
    expect(writeStore.getSnapshot().revision).toBe(0)
  })

  it('publishes local mutations and accepts newer validated storage events', async () => {
    const store = makeStore()
    let notifications = 0
    const unsubscribe = store.subscribe(() => notifications++)
    await store.create({ id: 'a', path: '/' })
    expect(notifications).toBe(1)
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: BROWSER_SHELL_TABS_STORAGE_KEY,
        newValue: JSON.stringify({
          schemaVersion: 1,
          epoch: 0,
          revision: 2,
          records: [
            { id: 'a', path: '/', name: 'Home', creationSequence: 1 },
            { id: 'b', path: '/docs', name: 'Docs', creationSequence: 2 },
          ],
        }),
      }),
    )
    expect(store.getSnapshot().records).toHaveLength(2)
    unsubscribe()
  })

  it('preserves shared path/name/customName and rejects stale writers', async () => {
    const store = makeStore()
    await store.create({ id: 'a', path: '/docs' })
    await store.rename('a', 'Reference')
    await store.updatePath('a', '/changelog')
    expect(store.getRecord('a')).toMatchObject({
      path: '/changelog',
      name: 'Changelog',
      customName: 'Reference',
    })
    await expectStoreError(store.close('a', { revision: 1 }), 'stale-version')
  })
})
