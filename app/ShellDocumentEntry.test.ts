import { describe, expect, it } from 'vitest'

import {
  classifyShellDocumentEntry,
  isShellDocumentContinuation,
  isShellDocumentHandoff,
} from './ShellDocumentEntry.js'

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
      const entry = classifyShellDocumentEntry({
        path: '/docs',
        params: { spaceId: 'space-1' },
        navigationType,
        incarnation: navigationType,
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
      const entry = classifyShellDocumentEntry({
        path: '/docs',
        navigationType: 'navigate',
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
})
