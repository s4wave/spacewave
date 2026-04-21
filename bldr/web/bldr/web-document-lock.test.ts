import { afterEach, describe, expect, it, vi } from 'vitest'

import { shouldUseWebDocumentLivenessLock } from './web-document-lock.js'

describe('shouldUseWebDocumentLivenessLock', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns false when navigator.locks is unavailable', () => {
    vi.stubGlobal('navigator', {})

    expect(shouldUseWebDocumentLivenessLock()).toBe(false)
  })

  it('returns true when navigator.locks exists', () => {
    vi.stubGlobal('navigator', { locks: {} })

    expect(shouldUseWebDocumentLivenessLock()).toBe(true)
  })
})
