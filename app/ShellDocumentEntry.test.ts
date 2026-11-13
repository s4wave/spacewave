import { describe, expect, it } from 'vitest'

import {
  classifyShellDocumentEntry,
  isShellDocumentContinuation,
  isShellDocumentHandoff,
  resolveShellDocumentEntry,
} from './ShellDocumentEntry.js'
import { writeShellDocumentState } from './ShellDocumentState.js'

describe('ShellDocumentEntry', () => {
  it('classifies an explicit shell tab handoff from hash navigation', () => {
    const entry = classifyShellDocumentEntry({
      hash: '#/docs?shellTabId=tab-2&spaceId=space-1',
      navigationType: 'navigate',
      incarnation: 'handoff-entry',
    })

    expect(entry).toMatchObject({
      kind: 'handoff',
      path: '/docs',
      params: { shellTabId: 'tab-2', spaceId: 'space-1' },
      tabId: 'tab-2',
      incarnation: 'handoff-entry',
    })
    expect(isShellDocumentHandoff(entry)).toBe(true)
    expect(isShellDocumentContinuation(entry)).toBe(false)
  })

  it('classifies reload and history restoration as continuation entries', () => {
    for (const navigationType of ['reload', 'back_forward'] as const) {
      const storage = memoryStorage()
      writeShellDocumentState(
        { incarnation: navigationType, activeTabId: 'tab-1' },
        storage,
      )
      const entry = classifyShellDocumentEntry({
        path: '/docs',
        params: { spaceId: 'space-1' },
        navigationType,
        storage,
      })

      expect(entry).toMatchObject({
        kind: 'continuation',
        path: '/docs',
        params: { spaceId: 'space-1' },
        incarnation: navigationType,
      })
      expect(isShellDocumentContinuation(entry)).toBe(true)
    }
  })

  it('classifies same-entry and persisted BFCache restores as continuation', () => {
    for (const options of [
      { sameEntry: true, incarnation: 'same-entry' },
      { persisted: true, incarnation: 'persisted-entry' },
    ]) {
      const storage = memoryStorage()
      writeShellDocumentState(
        { incarnation: options.incarnation, activeTabId: 'tab-1' },
        storage,
      )
      const entry = classifyShellDocumentEntry({
        path: '/docs',
        navigationType: 'navigate',
        storage,
        ...options,
      })
      expect(entry).toMatchObject({
        kind: 'continuation',
        path: '/docs',
        incarnation: options.incarnation,
      })
      expect(isShellDocumentContinuation(entry)).toBe(true)
    }
  })

  it('classifies a restore that saved no Shell state as fresh', () => {
    for (const navigationType of ['reload', 'back_forward'] as const) {
      const entry = classifyShellDocumentEntry({
        path: '/quickstart/local',
        navigationType,
        storage: memoryStorage(),
      })
      expect(entry).toMatchObject({ kind: 'fresh', path: '/quickstart/local' })
    }
  })

  it('classifies ordinary navigation as fresh and rejects malformed handoff IDs', () => {
    expect(
      classifyShellDocumentEntry({
        path: '/docs',
        navigationType: 'navigate',
        incarnation: 'fresh-entry',
      }),
    ).toMatchObject({ kind: 'fresh', path: '/docs' })

    expect(
      classifyShellDocumentEntry({
        hash: '#/docs?shellTabId=bad%20id',
        navigationType: 'navigate',
        incarnation: 'invalid-handoff',
      }),
    ).toMatchObject({ kind: 'fresh', path: '/docs' })
  })

  it('continues the first entry when the Shell remounts in one document', () => {
    const storage = memoryStorage()
    const classify = () =>
      classifyShellDocumentEntry({
        hash: '#/u/1/so/space-1/-/files',
        navigationType: 'navigate',
      })

    const first = resolveShellDocumentEntry(storage, classify)
    expect(first.kind).toBe('fresh')

    // Before the first mount writes its state, a remount classifies again.
    const early = resolveShellDocumentEntry(storage, classify)
    expect(early.kind).toBe('fresh')
    expect(early.incarnation).not.toBe(first.incarnation)

    // Once the document holds the entry's state, a remount continues it.
    writeShellDocumentState(
      { incarnation: early.incarnation, activeTabId: 'tab-1' },
      storage,
    )
    const remounted = resolveShellDocumentEntry(storage, classify)
    expect(remounted).toEqual({
      kind: 'continuation',
      path: early.path,
      params: early.params,
      incarnation: early.incarnation,
    })
    expect(isShellDocumentContinuation(remounted)).toBe(true)

    // Another document's storage classifies its own entry.
    const other = resolveShellDocumentEntry(memoryStorage(), classify)
    expect(other.kind).toBe('fresh')

    // Cleared document state starts a new entry.
    storage.clear()
    expect(resolveShellDocumentEntry(storage, classify).kind).toBe('fresh')
  })
})

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => {
      values.delete(key)
    },
    setItem: (key, value) => {
      values.set(key, value)
    },
  }
}
