import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  PersistenceStatus,
  StorageDurabilityOwner,
  StorageManagerLike,
} from './storage-durability-owner.js'

// These tests drive the document-side durability owner directly. The owner is
// the single seam that turns a "first meaningful write" signal into at most one
// navigator.storage.persisted()/persist() sequence, records the observed status,
// and isolates persistence errors from the user write.

function mockStorage(
  persisted: boolean,
  persistResult: boolean,
): StorageManagerLike & {
  persisted: ReturnType<typeof vi.fn>
  persist: ReturnType<typeof vi.fn>
} {
  return {
    persisted: vi.fn(async () => persisted),
    persist: vi.fn(async () => persistResult),
  }
}

function record(): {
  statuses: PersistenceStatus[]
  listener: (s: PersistenceStatus) => void
} {
  const statuses: PersistenceStatus[] = []
  return { statuses, listener: (s) => statuses.push(s) }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('StorageDurabilityOwner', () => {
  test('does not query or request persistence before a meaningful write', async () => {
    const storage = mockStorage(false, true)
    const { statuses } = record()
    const owner = new StorageDurabilityOwner(storage)
    await owner.whenSettled()
    expect(storage.persisted).not.toHaveBeenCalled()
    expect(storage.persist).not.toHaveBeenCalled()
    expect(owner.getStatus()).toBe('unknown')
    expect(statuses).toEqual([])
  })
  test('reads current protection without requesting it', async () => {
    const storage = mockStorage(true, true)
    const owner = new StorageDurabilityOwner(storage)

    await expect(owner.readStatus()).resolves.toBe('persisted')

    expect(storage.persisted).toHaveBeenCalledTimes(1)
    expect(storage.persist).not.toHaveBeenCalled()
  })
  test('explicit protection requests use the same owner sequence', async () => {
    const storage = mockStorage(false, true)
    const owner = new StorageDurabilityOwner(storage)

    await owner.requestProtection()

    expect(storage.persisted).toHaveBeenCalledTimes(1)
    expect(storage.persist).toHaveBeenCalledTimes(1)
    expect(owner.getStatus()).toBe('persisted')
  })

  test('first meaningful write triggers exactly one persisted()/persist() sequence, deduped across concurrent writes', async () => {
    const storage = mockStorage(false, true)
    const { statuses, listener } = record()
    const owner = new StorageDurabilityOwner(storage, listener)
    // Concurrent first writes all fire before any await resolves.
    owner.noteMeaningfulWrite()
    owner.noteMeaningfulWrite()
    owner.noteMeaningfulWrite()
    await owner.whenSettled()
    expect(storage.persisted).toHaveBeenCalledTimes(1)
    expect(storage.persist).toHaveBeenCalledTimes(1)
    expect(owner.getStatus()).toBe('persisted')
    expect(statuses).toEqual(['persisted'])
    // A later write does not re-request within the same document.
    owner.noteMeaningfulWrite()
    await owner.whenSettled()
    expect(storage.persisted).toHaveBeenCalledTimes(1)
    expect(storage.persist).toHaveBeenCalledTimes(1)
  })

  test('already-persisted origin records status without requesting persist()', async () => {
    const storage = mockStorage(true, true)
    const owner = new StorageDurabilityOwner(storage)
    owner.noteMeaningfulWrite()
    await owner.whenSettled()
    expect(storage.persisted).toHaveBeenCalledTimes(1)
    expect(storage.persist).not.toHaveBeenCalled()
    expect(owner.getStatus()).toBe('persisted')
  })

  test('denial changes only the recorded status, silently', async () => {
    const storage = mockStorage(false, false)
    const { statuses, listener } = record()
    const owner = new StorageDurabilityOwner(storage, listener)
    owner.noteMeaningfulWrite()
    await owner.whenSettled()
    expect(storage.persist).toHaveBeenCalledTimes(1)
    expect(owner.getStatus()).toBe('not-persisted')
    expect(statuses).toEqual(['not-persisted'])
  })

  test('a persistence query failure never fails the user write', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const storage: StorageManagerLike = {
      persisted: vi.fn(async () => {
        throw new Error('persisted rejected')
      }),
      persist: vi.fn(async () => true),
    }
    const { statuses, listener } = record()
    const owner = new StorageDurabilityOwner(storage, listener)
    // noteMeaningfulWrite must return synchronously without throwing.
    expect(() => owner.noteMeaningfulWrite()).not.toThrow()
    await expect(owner.whenSettled()).resolves.toBeUndefined()
    expect(owner.getStatus()).toBe('unknown')
    expect(statuses).toEqual([])
    expect(warn).toHaveBeenCalledTimes(1)
  })

  test('a persist request failure never fails the user write', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const storage: StorageManagerLike = {
      persisted: vi.fn(async () => false),
      persist: vi.fn(async () => {
        throw new Error('persist rejected')
      }),
    }
    const owner = new StorageDurabilityOwner(storage)
    expect(() => owner.noteMeaningfulWrite()).not.toThrow()
    await expect(owner.whenSettled()).resolves.toBeUndefined()
    expect(owner.getStatus()).toBe('unknown')
    expect(warn).toHaveBeenCalledTimes(1)
  })

  test('a null storage manager is a safe no-op', async () => {
    const owner = new StorageDurabilityOwner(null)
    expect(() => owner.noteMeaningfulWrite()).not.toThrow()
    await owner.whenSettled()
    expect(owner.getStatus()).toBe('unknown')
  })
})
